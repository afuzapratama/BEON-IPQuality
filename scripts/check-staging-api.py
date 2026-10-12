#!/usr/bin/env python3
"""Smoke-check private ARM staging MMDB using an authenticated loopback API container."""
import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import urllib.error
import urllib.request


def command(*args):
    return subprocess.check_output(['docker',*args],text=True).strip()


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root',required=True)
    args=parser.parse_args()
    os.umask(0o077)
    root=Path(args.root).resolve()
    previous=json.loads((root/'report.json').read_text())
    if previous.get('stages',{}).get('compiler',{}).get('exit_code') != 0 or not previous.get('verification'):
        raise ValueError('requires verified successful staging compilation')
    image=command('inspect','beon-ipquality-api-1','--format','{{.Config.Image}}')
    name='beon-staging-api-check-'+str(os.getpid())
    key=secrets.token_hex(32)
    env=dict(API_AUTH_ENABLED='true',API_KEY=key,ADMIN_API_KEY=secrets.token_hex(32),API_CORS_ENABLED='false',
             REDIS_ENABLED='false',SERVER_HOST='0.0.0.0',SERVER_PORT='8080',
             MMDB_PATH='/app/data/mmdb/reputation.mmdb',GEOIP_CITY_PATH='/app/data/mmdb/GeoLite2-City.mmdb',
             GEOIP_ASN_PATH='/app/data/mmdb/GeoLite2-ASN.mmdb',FEEDS_PATH='/app/configs/combined.yaml',
             GOMEMLIMIT='128MiB',LOG_LEVEL='info')
    args=['run','-d','--name',name,'--memory','192m','--read-only','--cap-drop','ALL',
          '--security-opt','no-new-privileges:true','--tmpfs','/tmp','-p','127.0.0.1::8080',
          '-v',str(root/'configs')+':/app/configs:ro','-v',str(root/'mmdb')+':/app/data/mmdb:ro']
    for field,value in env.items():args.extend(['-e',field+'='+value])
    args.extend([image,'-config','/app/configs/config.yaml'])
    try:
        command(*args)
        port=command('port',name,'8080/tcp').split(':')[-1]
        base='http://127.0.0.1:'+port
        def request(path,data=None,authenticated=True):
            headers={'Content-Type':'application/json'}
            if authenticated:headers['X-API-Key']=key
            req=urllib.request.Request(base+path,headers=headers,data=json.dumps(data).encode() if data else None)
            with urllib.request.urlopen(req,timeout=10) as response:return json.load(response)
        for _ in range(60):
            try:
                health=request('/health',authenticated=False)
                break
            except OSError:time.sleep(0.5)
        else:raise RuntimeError('staging health did not become ready')
        size=sum(1 for _ in (root/'source/proxy-ips.txt').open())
        step=max(1,size//100)
        sample=[]
        with (root/'source/proxy-ips.txt').open() as source:
            for index,line in enumerate(source):
                if index%step==0 and len(sample)<100:sample.append(line.strip())
        rows=[]
        for offset in range(0,len(sample),50):
            rows.extend(request('/api/v1/check/batch',{'ips':sample[offset:offset+50]})['results'])
        if len(rows)!=len(sample) or {row['ip'] for row in rows}!=set(sample):
            raise ValueError('incomplete sample coverage')
        if not all(row['proxy'] or row.get('whitelisted') for row in rows):
            raise ValueError('proxy flag missing from ingested records')
        for path, authenticated in (('/api/v1/check/8.8.8.8',False),('/api/v1/cache/stats',False),('/api/v1/cache/stats',True)):
            try:request(path,authenticated=authenticated)
            except urllib.error.HTTPError as error:
                if error.code!=401:raise
            else:raise ValueError('unauthenticated route was accepted')
        whitelist=request('/api/v1/check/66.249.64.1')
        if not whitelist.get('whitelisted') or whitelist['score']!=0:
            raise ValueError('whitelist regression')
        ipv6=request('/api/v1/check/2606:4700:4700::1111')
        report=dict(health=health['status'],snapshot_stale=health['snapshot']['stale'],samples=len(rows),
                    proxy_true=sum(row['proxy'] for row in rows),whitelisted=sum(row.get('whitelisted',False) for row in rows),
                    risky_true=sum(row['risky'] for row in rows),score_min=min(row['score'] for row in rows),
                    score_max=max(row['score'] for row in rows),data_versions=sorted({row['data_version'] for row in rows}),
                    whitelist_passed=True,ipv6_passed=ipv6['ip']=='2606:4700:4700::1111',auth_passed=True,
                    interpretation='Integration on ingested data; not independent accuracy',api_image=image)
        if len(report['data_versions'])!=1 or not report['ipv6_passed']:
            raise ValueError('inconsistent versions or IPv6 response')
        (root/'api-check.json').write_text(json.dumps(report,indent=2)+'\n')
        print(json.dumps(report,indent=2))
    finally:
        subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)


if __name__=='__main__':main()
