#!/usr/bin/env python3
"""Explicitly execute an authorized reboot drill and measure public recovery."""
import argparse
import json
from pathlib import Path
import subprocess
import time
import urllib.request


def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--host',required=True);p.add_argument('--ssh-key',required=True);p.add_argument('--url',required=True)
 p.add_argument('--execute',action='store_true');args=p.parse_args()
 if not args.execute:p.error('reboot requires explicit --execute')
 ssh=['ssh','-i',args.ssh_key,'-o','BatchMode=yes','-o','ConnectTimeout=3',args.host]
 before=subprocess.check_output(ssh+['cat /proc/sys/kernel/random/boot_id'],text=True).strip()
 started=time.monotonic()
 subprocess.run(ssh+['sudo systemctl reboot'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=10)
 ssh_ready=None;after=None;healthy=None
 for _ in range(200):
  try:
   result=subprocess.run(ssh+['cat /proc/sys/kernel/random/boot_id'],capture_output=True,text=True,timeout=5)
   if result.returncode==0 and result.stdout.strip()!=before:
    after=result.stdout.strip();ssh_ready=ssh_ready or time.monotonic()-started
    with urllib.request.urlopen(args.url.rstrip('/')+'/health',timeout=5) as response:
     state=json.load(response)
    if state['status']=='healthy' and not state['snapshot']['stale']:
     healthy=time.monotonic()-started;break
  except Exception:pass
  time.sleep(2)
 if healthy is None:raise RuntimeError('recovery not observed within drill timeout')
 checks=subprocess.check_output(ssh+['sudo docker ps --format "{{.Names}} {{.Status}}"; mountpoint /mnt/beon-scratch; systemctl is-active beon-avastel-source.service beon-avastel-update.timer beon-glances.service beon-backup.timer beon-geoipupdate.timer beon-alert-check.timer'],text=True)
 print(json.dumps({'boot_id_before':before,'boot_id_after':after,'ssh_ready_seconds':ssh_ready,
                   'https_healthy_seconds':healthy,'version':state['version'],'snapshot_stale':state['snapshot']['stale'],
                   'checks':checks,'scope':'Warm guest reboot; not Azure deallocation/reimage or disaster restore'},indent=2))


if __name__=='__main__':main()
