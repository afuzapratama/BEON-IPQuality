#!/usr/bin/env python3
"""Restore a decrypted backup into a new private Docker stack; never target production."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tarfile
import time
import urllib.error
import urllib.request


def run(*args, input=None):
    return subprocess.check_output(list(args), input=input, stderr=subprocess.STDOUT).decode().strip()


def extract(archive, target):
    with tarfile.open(archive) as bundle:
        # data filter rejects path traversal, unsafe links, devices and ownership.
        bundle.extractall(target, filter='data')


def verify(backup):
    if not (backup / 'COMPLETE').is_file():
        raise ValueError('incomplete backup')
    checked = set()
    for line in (backup / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split(maxsplit=1)
        name = name.lstrip('*')
        if Path(name).name != name or len(digest) != 64:
            raise ValueError('invalid checksum record')
        checked.add(name)
        if hashlib.sha256((backup / name).read_bytes()).hexdigest() != digest:
            raise ValueError('backup checksum mismatch')
    required = ('postgres.dump', 'runtime.env', 'deployment-lock.env', 'config.tar.gz', 'mmdb.tar.gz')
    if not set(required).issubset(checked) or not all((backup / name).is_file() for name in required):
        raise ValueError('required backup component missing')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--backup', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    os.umask(0o077)
    backup = Path(args.backup).resolve()
    output = Path(args.output).resolve()
    output.mkdir(mode=0o700, parents=True, exist_ok=False)
    started = time.monotonic()
    verify(backup)
    project = output / 'project'
    project.mkdir()
    extract(backup / 'config.tar.gz', project)
    mmdb = project / 'data/mmdb'
    mmdb.mkdir(parents=True)
    extract(backup / 'mmdb.tar.gz', mmdb)
    # Only mounted config/MMDB subdirectories are readable by app UID 10001.
    for directory in (project / 'configs', mmdb):
        for parent, dirs, files in os.walk(directory):
            Path(parent).chmod(0o755)
            for name in files:
                Path(parent, name).chmod(0o644)
    mmdb.chmod(0o777)  # isolated compiler needs to replace restored snapshot
    lock = output / 'deployment-lock.env'
    shutil.copyfile(backup / 'deployment-lock.env', lock)
    project_name = 'beon-dr-' + secrets.token_hex(6)
    key = secrets.token_hex(32)
    env = dict(COMPOSE_PROJECT_NAME=project_name, PROJECT_ROOT=str(project), MMDB_HOST_DIR=str(mmdb),
        POSTGRES_PASSWORD=secrets.token_hex(32), REDIS_PASSWORD=secrets.token_hex(32),
        API_KEY=key, ADMIN_API_KEY=secrets.token_hex(32), API_PORT='0', REDIS_ENABLED='false',
        DB_MAX_CONNECTIONS='5', DB_MIN_CONNECTIONS='1', INGESTOR_CONCURRENCY='1',
        API_MEMORY='192m', POSTGRES_MEMORY='512m', REDIS_MEMORY='64m',
        REDIS_MAXMEMORY='32mb', INGESTOR_MEMORY='1g', COMPILER_MEMORY='2560m',
        COMPILER_GO_MEMORY_LIMIT='1800MiB')
    envfile = output / 'drill.env'
    envfile.write_text(''.join(f'{k}={v}\n' for k, v in env.items()))
    compose = ['docker', 'compose', '-p', project_name, '--project-directory', str(project),
        '--env-file', str(envfile), '--env-file', str(lock),
        '-f', str(project / 'deployments/compose.production.yml')]
    report = dict(scope='Bunny backup reconstructed on a separate host; isolated DB/Redis/MMDB, synthetic credentials, loopback API. No public DNS/TLS or VPS rebuild.',
                  backup_id=backup.name, project=project_name, phases={})
    def phase(name, *argv, input=None):
        clock = time.monotonic()
        try:
            result = run(*argv, input=input)
        except subprocess.CalledProcessError as error:
            (output / (name + '.log')).write_bytes(error.output)
            raise RuntimeError(name + ' failed; private log saved') from None
        (output / (name + '.log')).write_text(result + '\n')
        report['phases'][name] = dict(seconds=round(time.monotonic() - clock, 3), success=True)
        return result
    def request(base, path, authenticated=True, data=None):
        req = urllib.request.Request(base + path, headers={'X-API-Key': key} if authenticated else {},
            data=json.dumps(data).encode() if data else None)
        if data:
            req.add_header('Content-Type', 'application/json')
        with urllib.request.urlopen(req, timeout=15) as response:
            return json.load(response)
    try:
        phase('start-storage', *compose, 'up', '-d', '--wait', 'postgres', 'redis')
        phase('restore-database', *compose, 'exec', '-T', 'postgres', 'pg_restore',
              '-U', 'beon', '-d', 'ipquality', '--exit-on-error', '--no-owner', '--no-acl',
              input=(backup / 'postgres.dump').read_bytes())
        # Migrations must be idempotent against the restored checksum ledger.
        phase('migrate', *compose, 'run', '--rm', '--no-deps', 'migrate')
        counts = phase('database-check', *compose, 'exec', '-T', 'postgres', 'psql', '-U', 'beon',
              '-d', 'ipquality', '-At', '-c',
              'SELECT count(*) FROM ip_reputation; SELECT version FROM schema_migrations ORDER BY version;')
        report['database_rows'] = int(counts.splitlines()[0])
        if report['database_rows'] < 1:
            raise ValueError('empty restored database')
        # Restored feeds need fresh successful fetches, especially five-minute feeds.
        # externally_managed account stays in the restored DB; no production URL/key exposed.
        phase('refresh-feeds', *compose, 'run', '--rm', '--no-deps', 'ingestor-init')
        phase('recompile', *compose, 'run', '--rm', '--no-deps', 'compiler-init')
        phase('verify-mmdb', *compose, 'run', '--rm', '--no-deps', 'compiler-init',
              '-verify-mmdb', '/app/data/mmdb')
        phase('start-api', *compose, 'up', '-d', '--no-deps', 'api')
        base = 'http://' + run(*compose, 'port', 'api', '8080')
        for _ in range(60):
            try:
                health = request(base, '/health', authenticated=False)
                if health['status'] == 'healthy' and not health['snapshot']['stale']:
                    break
            except (OSError, ValueError):
                pass
            time.sleep(1)
        else:
            raise RuntimeError('restored API failed freshness health gate')
        ips = phase('sample-restored-account', *compose, 'exec', '-T', 'postgres', 'psql',
            '-U', 'beon', '-d', 'ipquality', '-At', '-c',
            "SELECT host(ip_start) FROM ip_reputation WHERE source IN ('avastel_account/normalized_account', 'Avastel Account/normalized_account') AND expires_at > now() ORDER BY ip_start LIMIT 25;").splitlines()
        if len(ips) != 25:
            raise ValueError('restored account sample incomplete')
        predictions = request(base, '/api/v1/check/batch', data={'ips': ips})['results']
        if len(predictions) != 25 or not all(row['proxy'] or row.get('whitelisted') for row in predictions):
            raise ValueError('restored proxy records not served')
        dns = request(base, '/api/v1/check/8.8.8.8')
        ipv6 = request(base, '/api/v1/check/2606:4700:4700::1111')
        whitelist = request(base, '/api/v1/check/66.249.64.1')
        if ipv6['ip'] != '2606:4700:4700::1111' or not whitelist.get('whitelisted') or whitelist['score'] != 0:
            raise ValueError('IPv6/whitelist failed')
        for path, authenticated in [('/api/v1/check/8.8.8.8', False), ('/api/v1/cache/stats', True)]:
            try:
                request(base, path, authenticated=authenticated)
            except urllib.error.HTTPError as error:
                if error.code != 401:
                    raise
            else:
                raise ValueError('unauthorized request accepted')
        report.update(success=True, health=health['status'], account_samples=25,
            ipv6_passed=True, whitelist_passed=True, auth_passed=True,
            dns_score=dns['score'], provider_key_restored=(backup / 'avastel.key').is_file(),
            geoip_credentials_restored=(backup / 'GeoIP.conf').is_file(),
            operations_credentials_restored=(backup / 'operations.env').is_file(),
            data_version=health['snapshot']['data_version'] if 'data_version' in health['snapshot'] else predictions[0]['data_version'],
            duration_seconds=round(time.monotonic() - started, 3))
    finally:
        # Only the random project created above is removed. Source backup is retained privately.
        try:
            phase('cleanup', *compose, 'down', '-v', '--remove-orphans')
            report['cleanup_passed'] = True
        finally:
            envfile.unlink(missing_ok=True)
            (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
