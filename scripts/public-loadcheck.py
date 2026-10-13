#!/usr/bin/env python3
"""Rate-bounded public HTTPS stability probe; secrets read privately, aggregate output only."""
import argparse
import concurrent.futures
import json
from pathlib import Path
import random
import time
import urllib.error
import urllib.parse
import urllib.request


def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--client-env',required=True);p.add_argument('--ips',required=True)
 p.add_argument('--duration',type=int,default=300);p.add_argument('--rate',type=int,default=5)
 args=p.parse_args()
 if not 30<=args.duration<=600 or not 1<=args.rate<=10:p.error('duration 30..600 seconds; rate 1..10 HTTP req/s')
 env={}
 for line in Path(args.client_env).read_text().splitlines():
  if '=' in line and not line.startswith('#'):
   k,v=line.split('=',1);env[k]=v.strip().strip('\"\'')
 base=env['API_BASE_URL'].rstrip('/')
 if urllib.parse.urlparse(base).scheme!='https':p.error('HTTPS required')
 ips=Path(args.ips).read_text().splitlines()
 if len(ips)<100:p.error('at least 100 IPs required')
 rng=random.Random(20261013);cohort=rng.sample(ips,min(2000,len(ips)))
 cohort+=['8.8.8.8','1.1.1.1','9.9.9.9','2606:4700:4700::1111','2001:4860:4860::8888']
 def fetch(index):
  batch=index%2==1
  selected=[cohort[(index*7+j)%len(cohort)] for j in range(4 if batch else 1)]
  path='/api/v1/check/batch' if batch else '/api/v1/check/'+urllib.parse.quote(selected[0],safe=':')
  data=json.dumps({'ips':selected}).encode() if batch else None
  req=urllib.request.Request(base+path,data=data,headers={'X-API-Key':env['API_KEY'],'Content-Type':'application/json'})
  started=time.monotonic();status=0;valid=False
  try:
   with urllib.request.urlopen(req,timeout=10) as response:
    status=response.status;body=json.load(response)
    valid=sorted([row.get('ip') for row in body.get('results',[])])==sorted(selected) if batch else body.get('ip')==selected[0]
  except urllib.error.HTTPError as error:status=error.code
  except Exception:pass
  return {'status':status,'valid':valid,'ms':(time.monotonic()-started)*1000,'batch':batch}
 started=time.monotonic();futures=[]
 with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
  for i in range(args.duration*args.rate):
   delay=started+i/args.rate-time.monotonic()
   if delay>0:time.sleep(delay)
   futures.append(pool.submit(fetch,i))
  rows=[f.result() for f in futures]
 elapsed=time.monotonic()-started;report={'duration_seconds':elapsed,'target_rate':args.rate,'requests':len(rows),
   'varied_ip_cohort':len(cohort),'statuses':{},'invalid_or_failed':sum(row['status']!=200 or not row['valid'] for row in rows),
   'scope':'Client outside VPS, public TLS, rate bounded below key quota; stability probe, not throughput ceiling'}
 for row in rows:
  key=str(row['status']);report['statuses'][key]=report['statuses'].get(key,0)+1
 for batch,name in [(False,'single'),(True,'batch4')]:
  latencies=sorted(row['ms'] for row in rows if row['batch']==batch and row['status']==200 and row['valid'])
  report[name]={k:latencies[int((len(latencies)-1)*q)] if latencies else None for k,q in [('p50_ms',.5),('p95_ms',.95),('p99_ms',.99)]}
 print(json.dumps(report,indent=2))
 if report['invalid_or_failed']:raise SystemExit(1)


if __name__=='__main__':main()
