#!/usr/bin/env python3
"""Bounded private ARM staging with a read-only production dump and separate workers."""
import argparse
from datetime import datetime, timezone
import functools
import hashlib
import http.server
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import threading
import time
import yaml


def docker(*args):
    return subprocess.check_output(['docker', *args], text=True).strip()


def available():
    return int(next(line.split()[1] for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemAvailable:'))) * 1024


def abort_reason(memory_available, elapsed):
    if memory_available < 600*1024*1024:
        return "host memory below 600 MiB"
    if elapsed > 300:
        return "worker timeout exceeded 300 seconds"
    return None


def source_config(url):
    return dict(enabled=True, name='Avastel Account Staging Only', threat_type='proxy', confidence=0.8,
                weight=40, schedule='0 */6 * * *', sources=[dict(url=url,format='plain',name='normalized_account')])


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--root',required=True)
    p.add_argument('--source',required=True)
    p.add_argument('--sha256',required=True)
    args=p.parse_args()
    if platform.machine() not in ('aarch64','arm64'):
        p.error('requires ARM64')
    if available() < 2000*1024*1024:
        p.error('requires at least 2000 MiB available memory before staging')
    os.umask(0o077)
    root=Path(args.root).resolve()
    root.mkdir(mode=0o700,parents=True,exist_ok=False)
    production=Path('/opt/beon-ipquality')
    source=Path(args.source).resolve()
    if hashlib.sha256(source.read_bytes()).hexdigest()!=args.sha256:
        raise ValueError('source checksum mismatch')
    report=dict(started_at=datetime.now(timezone.utc).isoformat(),architecture=platform.machine(),source_sha256=args.sha256,production_enabled=False,
                scope='Combined active pilot snapshot + account proxy-positive IPs on ARM; isolated DB/network/MMDB',
                memory_limits=dict(postgres='384MiB',ingestor='896MiB',compiler='1536MiB'),
                cpu_limits=dict(postgres=0.5,worker=1.0),pressure_abort_mib=600, stages={})
    prefix='beon-avastel-arm-'+str(os.getpid())
    network=prefix+'-network'
    postgres=prefix+'-postgres'
    active=[]
    server=None
    images={}
    for role in ('postgres','ingestor','compiler','api'):
        images[role]=docker('inspect',f'beon-ipquality-{role}-1','--format','{{.Config.Image}}')
    report['images']=images
    try:
        docker('network','create','--internal',network)
        gateway=json.loads(docker('network','inspect',network))[0]['IPAM']['Config'][0]['Gateway']
        for name in ('configs','mmdb','source'):
            (root/name).mkdir(mode=0o750)
            os.chown(root/name,10001,10001)
        for name in ('config.yaml','feeds.yaml'):
            shutil.copyfile(production/'configs'/name,root/'configs'/name)
            os.chown(root/'configs'/name,10001,10001)
            (root/'configs'/name).chmod(0o640)
        for file in (production/'data/mmdb').glob('GeoLite2-*.mmdb'):
            shutil.copyfile(file,root/'mmdb'/file.name)
            os.chown(root/'mmdb'/file.name,10001,10001)
            (root/'mmdb'/file.name).chmod(0o640)
        shutil.copyfile(source,root/'source/proxy-ips.txt')
        (root/'source/proxy-ips.txt').chmod(0o640)
        os.chown(root/'source/proxy-ips.txt',10001,10001)
        class Handler(http.server.SimpleHTTPRequestHandler):
            def log_message(self,*args): pass
            def do_GET(self):
                if self.path!='/proxy-ips.txt':
                    self.send_error(404)
                    return
                return super().do_GET()
        server=http.server.ThreadingHTTPServer((gateway,0),functools.partial(Handler,directory=str(root/'source')))
        threading.Thread(target=server.serve_forever,daemon=True).start()
        url=f'http://{gateway}:{server.server_port}/proxy-ips.txt'
        original=yaml.safe_load((root/'configs/feeds.yaml').read_text())
        original['feeds']['avastel_account_staging']=source_config(url)
        original.setdefault('formats',{})['plain']={'description':'Normalized proxy-positive IPs'}
        (root/'configs/combined.yaml').write_text(yaml.safe_dump(original))
        ingest=dict(feeds={'avastel_account_staging':source_config(url)},formats={'plain':{'description':'Normalized proxy-positive IPs'}},whitelist={'enabled':False})
        (root/'configs/ingest.yaml').write_text(yaml.safe_dump(ingest))
        for name in ('combined.yaml','ingest.yaml'):
            os.chown(root/'configs'/name,10001,10001)
            (root/'configs'/name).chmod(0o640)
        with (root/'baseline.dump').open('wb') as dump:
            subprocess.run(['docker','exec','beon-ipquality-postgres-1','sh','-c',
                            'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc --no-owner --no-acl'],
                           stdout=dump,check=True,timeout=180)
        docker('run','-d','--name',postgres,'--network',network,'--memory','384m','--cpus','0.5',
               '-e','POSTGRES_PASSWORD=private-staging-only','-e','POSTGRES_DB=avastel_stage',images['postgres'])
        active.append(postgres)
        for _ in range(60):
            if subprocess.run(['docker','exec',postgres,'pg_isready','-U','postgres','-d','avastel_stage'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:
                break
            time.sleep(0.5)
        else: raise RuntimeError('staging database not ready')
        with (root/'baseline.dump').open('rb') as dump,(root/'restore.log').open('w') as log:
            subprocess.run(['docker','exec','-i',postgres,'pg_restore','-U','postgres','-d','avastel_stage',
                            '--no-owner','--no-acl','--exit-on-error'],stdin=dump,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=180)
        def sql(query):
            return docker('exec',postgres,'psql','-U','postgres','-d','avastel_stage','-At','-c',query)
        report['baseline_active_rows']=int(sql('SELECT count(*) FROM ip_reputation WHERE expires_at > NOW();'))
        base_env=dict(DB_HOST=postgres,DB_PORT='5432',DB_USER='postgres',DB_PASSWORD='private-staging-only',
                      DB_NAME='avastel_stage',DB_SSL_MODE='disable',DB_MAX_CONNECTIONS='5',DB_MIN_CONNECTIONS='1',
                      REDIS_ENABLED='false',MMDB_PATH='/app/data/mmdb/reputation.mmdb',MMDB_OUTPUT='/app/data/mmdb/reputation.mmdb',
                      GEOIP_CITY_PATH='/app/data/mmdb/GeoLite2-City.mmdb',GEOIP_ASN_PATH='/app/data/mmdb/GeoLite2-ASN.mmdb',
                      INGESTOR_CONCURRENCY='1',METRICS_HOST='127.0.0.1',GOGC='50',LOG_LEVEL='info')
        for role,limit,gomemory,arguments in [('ingestor','896m','700MiB',['-once','-feeds','/app/configs/ingest.yaml']),
                                            ('compiler','1536m','1100MiB',['-oneshot'])]:
            if available()<1800*1024*1024:
                report['stages'][role]={'aborted':'insufficient memory headroom before worker'}
                break
            name=prefix+'-'+role
            env=dict(base_env,GOMEMLIMIT=gomemory,FEEDS_PATH='/app/configs/combined.yaml')
            command=['run','-d','--name',name,'--network',network,'--memory',limit,'--cpus','1',
                     '--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp',
                     '-v',str(root/'configs')+':/app/configs:ro','-v',str(root/'mmdb')+':/app/data/mmdb']
            for k,v in env.items():command.extend(['-e',k+'='+v])
            command.extend([images[role],'-config','/app/configs/config.yaml',*arguments])
            started=time.monotonic();docker(*command);active.append(name)
            pid=int(docker('inspect',name,'--format','{{.State.Pid}}'))
            peak_hwm=peak_cgroup=0;aborted=None
            while True:
                state=json.loads(docker('inspect',name))[0]['State']
                if not state['Running']:break
                try:
                    for line in Path(f'/proc/{pid}/status').read_text().splitlines():
                        if line.startswith('VmHWM:'):peak_hwm=max(peak_hwm,int(line.split()[1])*1024)
                    cgroup=Path(f'/proc/{pid}/cgroup').read_text().strip().split('::')[-1]
                    peak_file=Path('/sys/fs/cgroup')/cgroup.lstrip('/')/'memory.peak'
                    peak_cgroup=max(peak_cgroup,int(peak_file.read_text().strip()))
                except (FileNotFoundError,ProcessLookupError):pass
                aborted=abort_reason(available(),time.monotonic()-started)
                if aborted:docker('kill',name)
                if aborted:break
                time.sleep(0.5)
            state=json.loads(docker('inspect',name))[0]['State']
            (root/f'{role}.log').write_text(docker('logs',name))
            outcome=dict(elapsed_seconds=time.monotonic()-started,exit_code=state['ExitCode'],oom_killed=state['OOMKilled'],
                         process_hwm_bytes_sampled=peak_hwm,cgroup_peak_bytes_sampled=peak_cgroup,aborted=aborted)
            report['stages'][role]=outcome
            (root/'report.json').write_text(json.dumps(report,indent=2)+'\n')
            print(role,json.dumps(outcome),flush=True)
            if outcome['exit_code'] or outcome['oom_killed'] or aborted:break
        report['combined_active_rows']=int(sql('SELECT count(*) FROM ip_reputation WHERE expires_at > NOW();'))
        report['staging_database_bytes']=int(sql('SELECT pg_database_size(current_database());'))
        if (root/'mmdb/reputation.mmdb').exists():
            verification=docker('run','--rm','--network','none','--read-only','--memory','192m',
                                '-v',str(root/'mmdb')+':/snapshot:ro',images['compiler'],'-verify-mmdb','/snapshot')
            report['verification']=json.loads(verification)
            report['mmdb_bytes']=(root/'mmdb/reputation.mmdb').stat().st_size
        report['completed_at']=datetime.now(timezone.utc).isoformat()
        (root/'report.json').write_text(json.dumps(report,indent=2)+'\n')
    finally:
        if server:server.shutdown();server.server_close()
        for name in reversed(active):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        subprocess.run(['docker','network','rm',network],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    print('ARM staging completed; temporary containers/volumes/network removed.',flush=True)


if __name__=='__main__':main()
