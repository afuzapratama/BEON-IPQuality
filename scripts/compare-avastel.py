#!/usr/bin/env python3
"""Pinned, read-only positive proxy agreement study; never changes production feeds."""
import argparse
import csv
from datetime import datetime, timezone
import hashlib
import ipaddress
import json
import math
import os
from pathlib import Path
import random
import urllib.error
import urllib.request

REPO = 'antoinevastel/avastel-bot-ips-lists'
FILE = 'avastel-proxy-bot-ips-1day.txt'


def fetch(url, headers=None, data=None):
    request = urllib.request.Request(url, headers=headers or {'User-Agent': 'BEON-IPQuality-evaluation'}, data=data)
    with urllib.request.urlopen(request, timeout=60) as response:
        content = response.read(32 * 1024 * 1024 + 1)
        if len(content) > 32 * 1024 * 1024:
            raise ValueError('response exceeds 32 MiB bound')
        return content


def parse_ips(content):
    addresses = set()
    for line in content.decode().splitlines():
        value = line.split('#', 1)[0].strip()
        if not value:
            continue
        fields = next(csv.reader([value], delimiter=';'))
        if fields == ['ip_address', 'autonomous_system']:
            continue
        address = ipaddress.ip_address(fields[0])
        if getattr(address, 'ipv4_mapped', None):
            address = address.ipv4_mapped
        if not address.is_global:
            raise ValueError('source contains non-global IP')
        addresses.add(str(address))
    if not addresses:
        raise ValueError('empty source')
    return sorted(addresses)


def wilson(success, total):
    if not total:
        return None
    z = 1.959963984540054
    p = success / total
    denominator = 1 + z*z/total
    center = (p + z*z/(2*total)) / denominator
    margin = z * math.sqrt(p*(1-p)/total + z*z/(4*total*total)) / denominator
    return [max(0.0, center-margin), min(1.0, center+margin)]


def save(directory, name, value):
    (directory / name).write_text(json.dumps(value, indent=2) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--client-env', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--samples', type=int, default=500)
    parser.add_argument('--seed', type=int, default=20261013)
    args = parser.parse_args()
    if not 1 <= args.samples <= 1000:
        parser.error('samples must be 1..1000')
    os.umask(0o077)
    directory = Path(args.output)
    directory.mkdir(mode=0o700, parents=True, exist_ok=False)
    credentials = {}
    for line in Path(args.client_env).read_text().splitlines():
        if '=' in line and not line.lstrip().startswith('#'):
            key, value = line.split('=', 1)
            credentials[key.strip()] = value.strip().strip('\"\'')
    base = credentials['API_BASE_URL'].rstrip('/')
    if not base.startswith('https://'):
        raise ValueError('API endpoint must use HTTPS')
    commit = json.loads(fetch(f'https://api.github.com/repos/{REPO}/commits?path={FILE}&per_page=1'))[0]
    revision = commit['sha']
    url = f'https://raw.githubusercontent.com/{REPO}/{revision}/{FILE}'
    content = fetch(url)
    updated = next((line.removeprefix('# Last update:').strip()
                    for line in content.decode().splitlines()[:10]
                    if line.startswith('# Last update:')), None)
    if not updated:
        raise ValueError('source lacks update timestamp')
    update_time = datetime.fromisoformat(updated.replace('Z', '+00:00'))
    if update_time.tzinfo is None:
        raise ValueError('source timestamp must include timezone')
    age = (datetime.now(timezone.utc) - update_time).total_seconds()
    if not 0 <= age <= 36*3600:
        raise ValueError('source is future-dated or older than 36 hours')
    (directory / 'source.txt').write_bytes(content)
    addresses = parse_ips(content)
    sample = random.Random(args.seed).sample(addresses, min(args.samples, len(addresses)))
    fetched_at = datetime.now(timezone.utc).isoformat()
    manifest = dict(source_url=url, source_revision=revision,
                    source_commit_time=commit['commit']['committer']['date'], fetched_at=fetched_at,
                    source_sha256=hashlib.sha256(content).hexdigest(), unique_ips=len(addresses),
                    source_age_seconds_at_fetch=age,
                    source_update_header=next((line for line in content.decode().splitlines()[:10]
                                               if line.startswith('# Last update:')), None),
                    samples=len(sample), seed=args.seed, label='provider-reported proxy positives',
                    independence='No direct Avastel feed configured; upstream independence not fully established',
                    interpretation='Positive-only agreement; not overall accuracy, precision or false-positive rate')
    save(directory, 'manifest.json', manifest)
    (directory / 'sample.jsonl').write_text(''.join(json.dumps({'ip': ip})+'\n' for ip in sample))
    predictions = []
    version = None
    for offset in range(0, len(sample), 50):
        ips = sample[offset:offset+50]
        payload = json.dumps({'ips': ips}).encode()
        response = json.loads(fetch(base+'/api/v1/check/batch',
                                   {'X-API-Key': credentials['API_KEY'], 'Content-Type': 'application/json'}, payload))
        rows = response['results']
        if len(rows) != len(ips) or {row.get('ip') for row in rows} != set(ips):
            raise ValueError('batch coverage mismatch')
        for row in rows:
            current = row.get('data_version')
            if not current or type(row.get('proxy')) is not bool:
                raise ValueError('missing version or valid proxy prediction')
            if version is None:
                version = current
            if current != version:
                raise ValueError('snapshot changed; repeat collection on consistent snapshot')
        predictions.extend(rows)
    (directory / 'predictions.jsonl').write_text(''.join(json.dumps(row)+'\n' for row in predictions))
    matched = sum(row['proxy'] for row in predictions)
    report = dict(manifest, data_version=version, collected_at=datetime.now(timezone.utc).isoformat(),
                  proxy_detected=matched, proxy_not_detected=len(sample)-matched,
                  positive_agreement=matched/len(sample), positive_agreement_wilson95=wilson(matched,len(sample)),
                  precision=None, false_positive_rate=None,
                  ipv4=sum(ipaddress.ip_address(ip).version == 4 for ip in sample),
                  ipv6=sum(ipaddress.ip_address(ip).version == 6 for ip in sample),
                  category_counts={key: sum(row.get(key) is True for row in predictions)
                                   for key in ('risky','vpn','tor','datacenter','botnet','whitelisted')},
                  feed_sources_seen=sorted({source for row in predictions for source in row.get('sources', [])}),
                  limitations=['Provider labels not independently revalidated', 'Positive-only corpus',
                               'One daily snapshot', 'Possible upstream correlation',
                               'Shared IPs and time-varying proxy activity',
                               'Confidence interval excludes label uncertainty and ASN/subnet dependence'])
    save(directory, 'report.json', report)
    print(json.dumps({k: report[k] for k in ('samples','proxy_detected','proxy_not_detected','positive_agreement','data_version')}, indent=2))


if __name__ == '__main__':
    try:
        main()
    except urllib.error.HTTPError as error:
        raise SystemExit(f'Collection failed: HTTP {error.code}; no completed report') from None
    except Exception as error:
        # Never print request URLs, headers, credentials or response payloads.
        raise SystemExit(f'Collection failed: {type(error).__name__}; inspect private inputs; no completed report') from None
