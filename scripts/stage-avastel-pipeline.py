#!/usr/bin/env python3
"""Run private normalized Avastel data through temporary local PostgreSQL and Go workers."""
import argparse
import functools
import hashlib
import http.server
import json
import os
from pathlib import Path
import platform
import socket
import urllib.request
import subprocess
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[1]


def smoke_api(directory, stage, env):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    key = 'isolated-staging-only-' + 'x'*48
    env = dict(env, SERVER_HOST='127.0.0.1', SERVER_PORT=str(port), API_AUTH_ENABLED='true',
               API_KEY=key, ADMIN_API_KEY='', REDIS_ENABLED='false', API_CORS_ENABLED='false')
    env.pop('DATABASE_URL', None)
    sample = []
    manifest = json.loads((directory/'manifest.json').read_text())
    step = max(1, manifest['unique_ips']//100)
    with (directory/'proxy-ips.txt').open() as source:
        for number, line in enumerate(source):
            if number % step == 0 and len(sample) < 100:
                sample.append(line.strip())
    base = f'http://127.0.0.1:{port}'
    with (stage/'api-smoke.log').open('w') as log:
        process = subprocess.Popen([str(directory/'bin/api'), '-config',str(ROOT/'configs/config.yaml')],
                                   env=env,stdout=log,stderr=subprocess.STDOUT)
        try:
            for _ in range(50):
                if process.poll() is not None:
                    raise RuntimeError('staging API exited early')
                try:
                    with urllib.request.urlopen(base+'/health',timeout=2) as response:
                        health = json.load(response)
                    break
                except OSError:
                    time.sleep(0.1)
            else:
                raise RuntimeError('staging API not ready')
            results=[]
            for offset in range(0,len(sample),50):
                request = urllib.request.Request(base+'/api/v1/check/batch',
                    data=json.dumps({'ips':sample[offset:offset+50]}).encode(),
                    headers={'X-API-Key':key,'Content-Type':'application/json'})
                with urllib.request.urlopen(request,timeout=10) as response:
                    results.extend(json.load(response)['results'])
            if {row['ip'] for row in results} != set(sample) or len(results) != len(sample):
                raise ValueError('staging API coverage mismatch')
            if not all(row['proxy'] is True for row in results):
                raise ValueError('staging lookup lost proxy flag')
            return dict(samples=len(sample), proxy_true=sum(row['proxy'] for row in results),
                        scores=sorted({row['score'] for row in results}),
                        risky_true=sum(row['risky'] for row in results),
                        data_versions=sorted({row['data_version'] for row in results}),
                        health_status=health['status'],
                        interpretation='Integration self-check on ingested records; not independent accuracy')
        finally:
            process.terminate()
            try: process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dataset-dir', required=True)
    parser.add_argument('--run-name', default='pipeline')
    args = parser.parse_args()
    if not sys.platform.startswith("linux"):
        parser.error("pipeline measurements require Linux wait4 RSS units")
    os.umask(0o077)
    directory = Path(args.dataset_dir).resolve()
    manifest = json.loads((directory/'manifest.json').read_text())
    normalized = directory/'proxy-ips.txt'
    if hashlib.sha256(normalized.read_bytes()).hexdigest() != manifest['normalized_sha256']:
        raise ValueError('normalized checksum mismatch')
    if not args.run_name.replace('-', '').replace('_', '').isalnum():
        parser.error('run-name must be a simple directory name')
    stage = directory/args.run_name
    stage.mkdir(mode=0o700, exist_ok=False)
    container = f'beon-avastel-staging-{os.getpid()}'
    server = None
    metrics = {}
    try:
        subprocess.run(['docker','run','-d','--name',container,'--memory','1g',
                        '-e','POSTGRES_PASSWORD=staging-only','-e','POSTGRES_DB=avastel_stage',
                        '-p','127.0.0.1::5432','postgres:17-alpine'],check=True,capture_output=True)
        for attempt in range(30):
            if subprocess.run(['docker','exec',container,'pg_isready','-U','postgres','-d','avastel_stage'],capture_output=True).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError('temporary database not ready')
        port = subprocess.check_output(['docker','port',container,'5432/tcp'],text=True).strip().split(':')[-1]
        env = dict(os.environ, DB_HOST='127.0.0.1',DB_PORT=port,DB_USER='postgres',DB_PASSWORD='staging-only',
                   DB_NAME='avastel_stage',DB_SSL_MODE='disable',DB_MAX_CONNECTIONS='5',DB_MIN_CONNECTIONS='1',
                   REDIS_ENABLED='false',INGESTOR_CONCURRENCY='1',METRICS_HOST='127.0.0.1',
                   MMDB_PATH=str(stage/'reputation.mmdb'),MMDB_OUTPUT=str(stage/'reputation.mmdb'),
                   GEOIP_CITY_PATH=str(stage/'absent-city.mmdb'),GEOIP_ASN_PATH=str(stage/'absent-asn.mmdb'),
                   LOG_LEVEL='info',FEEDS_PATH=str(stage/'feeds.yaml'),GOMEMLIMIT='700MiB',GOGC='50')
        env.pop('DATABASE_URL', None)
        env.pop('REDIS_URL', None)
        migration_env = dict(env, PGHOST='127.0.0.1', PGPORT=port, PGUSER='postgres',
                             PGPASSWORD='staging-only', PGDATABASE='avastel_stage')
        migration_env.pop('DATABASE_URL', None)
        with (stage/'migration.log').open('w') as log:
            subprocess.run([str(ROOT/'scripts/migrate.sh')],env=migration_env,stdout=log,stderr=subprocess.STDOUT,check=True)

        class Handler(http.server.SimpleHTTPRequestHandler):
            def log_message(self, *args): pass
            def do_GET(self):
                if self.path != '/proxy-ips.txt':
                    self.send_error(404)
                    return
                return super().do_GET()

        server = http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(directory)))
        threading.Thread(target=server.serve_forever,daemon=True).start()
        url = f'http://127.0.0.1:{server.server_port}/proxy-ips.txt'
        (stage/'feeds.yaml').write_text(f'''feeds:
  avastel_account_staging:
    enabled: true
    name: Avastel Account Staging Only
    threat_type: proxy
    confidence: 0.8
    weight: 40
    schedule: "0 */6 * * *"
    sources:
    - url: {url}
      format: plain
      name: normalized_account
formats:
  plain:
    description: Normalized proxy-positive IPs
whitelist:
  enabled: false
''')
        for name, arguments, memory in [('ingestor',['-once','-feeds',str(stage/'feeds.yaml')],'700MiB'),
                                         ('compiler',['-oneshot'],'1100MiB')]:
            env['GOMEMLIMIT'] = memory
            env['METRICS_PORT'] = '0'
            command = [str(directory/'bin'/name), '-config',str(ROOT/'configs/config.yaml'),*arguments]
            started = time.monotonic()
            with (stage/f'{name}.log').open('w') as log:
                process = subprocess.Popen(command,env=env,stdout=log,stderr=subprocess.STDOUT)
                timer = threading.Timer(1200, process.kill)
                timer.start()
                try:
                    _, status, usage = os.wait4(process.pid, 0)
                    process.returncode = os.waitstatus_to_exitcode(status)
                except BaseException:
                    process.kill()
                    process.wait()
                    raise
                finally:
                    timer.cancel()
                metrics[name] = dict(elapsed_seconds=time.monotonic()-started,
                                     peak_rss_kib=usage.ru_maxrss, exit_code=process.returncode)
                (stage/f'{name}.metrics.json').write_text(json.dumps(metrics[name])+'\n')
                if process.returncode:
                    raise RuntimeError(f'{name} failed; inspect private staging log')
            print(name, json.dumps(metrics[name]),flush=True)
        verified = subprocess.check_output([str(directory/'bin/compiler'),'-verify-mmdb',str(stage)],text=True)
        metrics['verification']=json.loads(verified)
        metrics['api_smoke']=smoke_api(directory,stage,env)
        query = 'SELECT json_build_object(\'active_rows\',COUNT(*) FILTER (WHERE expires_at > NOW()),\'total_rows\',COUNT(*)) FROM ip_reputation;'
        counts = subprocess.check_output(['docker','exec',container,'psql','-U','postgres','-d','avastel_stage','-At','-c',query],text=True)
        metrics['database']=json.loads(counts)
        db_size = subprocess.check_output(['docker','exec',container,'psql','-U','postgres','-d','avastel_stage','-At','-c','SELECT pg_database_size(current_database());'],text=True)
        metrics['database_bytes']=int(db_size.strip())
        metrics.update(architecture=platform.machine(),dataset_unique_ips=manifest['unique_ips'],
                       mmdb_bytes=(stage/'reputation.mmdb').stat().st_size,
                       source_zip_sha256=manifest['zip_sha256'],
                       measurement_scope='Local isolated normalized source only; no live pilot feeds, no GeoLite2; not ARM production capacity',
                       production_enabled=False,redistribution_authorized=False)
        (stage/'report.json').write_text(json.dumps(metrics,indent=2)+'\n')
        print('Staging pipeline complete; report saved privately.',flush=True)
    finally:
        if server:
            server.shutdown()
            server.server_close()
        subprocess.run(['docker','rm','-fv',container],capture_output=True)


if __name__ == '__main__': main()
