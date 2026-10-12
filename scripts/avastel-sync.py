#!/usr/bin/env python3
"""Private production account feed publisher and bridge-only normalized source server."""
import argparse
from datetime import datetime, timezone
import fcntl
import hashlib
import http.server
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

ROOT=Path(os.environ.get('PROJECT_ROOT','/opt/beon-ipquality'))
DATA=ROOT/'data/avastel'


def compose(*args):
    subprocess.run(['docker','compose','--env-file',str(ROOT/'.env'),'--env-file',str(ROOT/'deployment-lock.env'),
                    '-f',str(ROOT/'deployments/compose.production.yml'),*args],cwd=ROOT,check=True,timeout=600)


def refresh():
    DATA.mkdir(mode=0o700,parents=True,exist_ok=True)
    with (DATA/'sync.lock').open('w') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        with tempfile.TemporaryDirectory(prefix='fetch-',dir=DATA) as temp:
            stage=Path(temp)/'dataset'
            subprocess.run([sys.executable,str(ROOT/'scripts/stage-avastel-account.py'),
                            '--key-file',os.environ.get('AVASTEL_KEY_FILE','/etc/beon-ipquality/avastel.key'),
                            '--output',str(stage)],check=True,timeout=180)
            # One atomic directory switch pairs metadata and data from the same download.
            destination=DATA/('snapshot-'+str(time.time_ns()))
            destination.mkdir(mode=0o700)
            for name in ('proxy-ips.txt','manifest.json'):
                shutil.copyfile(stage/name,destination/name)
            link=DATA/'next'
            if link.is_symlink():link.unlink()
            link.symlink_to(destination.name,target_is_directory=True)
            os.replace(link,DATA/'current')
            compose('run','--rm','--no-deps','ingestor','-config','/app/configs/config.yaml',
                    '-feeds','/app/configs/feeds.avastel-publisher.yaml','-once')
            compose('run','--rm','--no-deps','compiler','-config','/app/configs/config.yaml','-oneshot')
            subprocess.run(['docker','compose','--env-file',str(ROOT/'.env'),'--env-file',str(ROOT/'deployment-lock.env'),
                            '-f',str(ROOT/'deployments/compose.production.yml'),'exec','-T','api','sh','-c',
                            'wget -qO- --header="X-API-Key: $ADMIN_API_KEY" --post-data="" http://127.0.0.1:8080/api/v1/reload'],
                           cwd=ROOT,check=True,timeout=30)
            (DATA/'last-published.json').write_text(json.dumps({'published_at':datetime.now(timezone.utc).isoformat(),
                         'source_sha256':json.loads((destination/'manifest.json').read_text())['normalized_sha256']})+'\n')
            for old in DATA.glob('snapshot-*'):
                if old!=destination and time.time()-old.stat().st_mtime>24*3600:shutil.rmtree(old)


def source_snapshot():
    snapshot=(DATA/'current').resolve(strict=True)
    manifest=json.loads((snapshot/'manifest.json').read_text())
    age=(datetime.now(timezone.utc)-datetime.fromisoformat(manifest['fetched_at'])).total_seconds()
    if not 0<=age<=7*3600+20*60:raise ValueError('normalized source expired')
    return snapshot/'proxy-ips.txt'


def serve():
    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self,*args):pass
        def do_GET(self):
            if self.path!='/proxy-ips.txt':self.send_error(404);return
            try:
                file=source_snapshot()
                with file.open('rb') as data:
                    self.send_response(200);self.send_header('Content-Type','text/plain')
                    self.send_header('Content-Length',str(file.stat().st_size));self.end_headers()
                    shutil.copyfileobj(data,self.wfile,1024*1024)
            except (OSError,ValueError,KeyError):self.send_error(503,'source unavailable')
    host=os.environ.get('AVASTEL_SOURCE_HOST','172.18.0.1')
    http.server.ThreadingHTTPServer((host,18090),Handler).serve_forever()


if __name__=='__main__':
    os.umask(0o077)
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('mode',choices=('refresh','serve'))
    args=parser.parse_args()
    if args.mode=='serve':serve()
    else:refresh()
