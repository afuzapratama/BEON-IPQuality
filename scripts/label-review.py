#!/usr/bin/env python3
"""Validate operator-reviewed independent labels and split by reviewer-supplied ASN/group."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path

CATEGORIES={'risky','proxy','vpn','tor','datacenter','botnet','spam','malware','attacker'}


def reviewed(rows):
 accepted=[];unknown=0;seen=set()
 for row in rows:
  address=ipaddress.ip_address(row['ip'])
  if getattr(address,'ipv4_mapped',None):address=address.ipv4_mapped
  if not address.is_global:raise ValueError('only public IPs allowed')
  ip=str(address)
  if ip in seen:raise ValueError('duplicate normalized IP')
  seen.add(ip)
  if row.get('review_status')!='approved':unknown+=1;continue
  required=('label_source','observed_at','reviewed_by','evidence_uri','split_group')
  if any(not row.get(field) for field in required):raise ValueError('approved labels lack provenance/group')
  if row.get('independent') is not True or row.get('training_overlap') is not False:
   raise ValueError('approved holdout requires independent source and no training overlap')
  labels=row.get('labels',{})
  if not labels or set(labels)-CATEGORIES or any(type(value) is not bool for value in labels.values()):
   raise ValueError('known categories require boolean labels; omit unknown categories')
  accepted.append(dict(row,ip=ip))
 return accepted,unknown


def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--reviews',required=True);parser.add_argument('--output',required=True)
 args=parser.parse_args();os.umask(0o077)
 rows=[json.loads(line) for line in Path(args.reviews).read_text().splitlines() if line.strip()]
 accepted,unknown=reviewed(rows)
 if not accepted:raise ValueError('no approved independent labels; no accuracy evaluation possible')
 output=Path(args.output);output.mkdir(mode=0o700,parents=True,exist_ok=False)
 splits={'tuning':[],'holdout':[]}
 for row in accepted:
  # Stable split by whole ASN/group, never by individual IP.
  bucket=int.from_bytes(hashlib.sha256(row['split_group'].encode()).digest()[:8],'big')%5
  splits['holdout' if bucket==0 else 'tuning'].append(row)
 for name,values in splits.items():
  (output/(name+'-labels.jsonl')).write_text(''.join(json.dumps(row)+'\n' for row in values))
 report={'approved':len(accepted),'unreviewed_or_rejected':unknown,'tuning':len(splits['tuning']),'holdout':len(splits['holdout']),
         'note':'Group split does not prove source independence; inspect label balance and provenance before evaluation'}
 (output/'review-summary.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))


if __name__=='__main__':main()
