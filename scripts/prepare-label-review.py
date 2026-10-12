#!/usr/bin/env python3
"""Prepare a private, blinded diagnostic cohort; source membership is never a label."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import random
import urllib.parse
import urllib.request


def candidates(path, count, seed):
    rng = random.Random(seed)
    sample = []
    for index, line in enumerate(Path(path).read_text().splitlines()):
        ip = str(ipaddress.ip_address(line.strip()))
        if not ipaddress.ip_address(ip).is_global:
            raise ValueError('source contains nonpublic IP')
        if index < count:
            sample.append(ip)
        else:
            slot = rng.randrange(index + 1)
            if slot < count:
                sample[slot] = ip
    return sorted(set(sample))


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--source', required=True)
    p.add_argument('--client-env', required=True)
    p.add_argument('--output', required=True)
    p.add_argument('--count', type=int, default=250)
    args = p.parse_args()
    if not 20 <= args.count <= 500:
        p.error('count must be 20..500')
    os.umask(0o077)
    output = Path(args.output)
    output.mkdir(mode=0o700, parents=True, exist_ok=False)
    env = {}
    for line in Path(args.client_env).read_text().splitlines():
        if '=' in line and not line.startswith('#'):
            k, v = line.split('=', 1)
            env[k] = v.strip().strip('\"\'')
    base = env['API_BASE_URL'].rstrip('/')
    if urllib.parse.urlparse(base).scheme != 'https':
        raise ValueError('HTTPS required')
    ips = candidates(args.source, args.count, 20261013)
    # Diagnostic controls only: DNS addresses are not assumed negative labels.
    ips = sorted(set(ips + ['8.8.8.8', '8.8.4.4', '1.1.1.1', '1.0.0.1',
                            '9.9.9.9', '208.67.222.222', '2606:4700:4700::1111',
                            '2001:4860:4860::8888']))
    predictions = []
    for offset in range(0, len(ips), 50):
        req = urllib.request.Request(base + '/api/v1/check/batch',
            data=json.dumps({'ips': ips[offset:offset + 50]}).encode(),
            headers={'Content-Type': 'application/json', 'X-API-Key': env['API_KEY']})
        with urllib.request.urlopen(req, timeout=30) as response:
            predictions.extend(json.load(response)['results'])
    if {row['ip'] for row in predictions} != set(ips) or len(predictions) != len(ips):
        raise ValueError('incomplete predictions')
    versions = {row['data_version'] for row in predictions}
    if len(versions) != 1:
        raise ValueError('snapshot changed; restart cohort collection')
    # Store predictions separately so reviewers do not see scores or flags.
    (output / 'predictions-diagnostic.jsonl').write_text(''.join(json.dumps(row) + '\n' for row in predictions))
    rows = [dict(ip=ip, labels={}, label_source='', observed_at='', reviewed_by='',
                 evidence_uri='', split_group='', independent=False, training_overlap=None,
                 review_status='unreviewed') for ip in ips]
    (output / 'reviews-blinded.jsonl').write_text(''.join(json.dumps(row) + '\n' for row in rows))
    summary = dict(samples=len(ips), approved_independent=0, data_version=next(iter(versions)),
        source_sha256=hashlib.sha256(Path(args.source).read_bytes()).hexdigest(),
        predicted_counts={name: sum(row.get(name) is True for row in predictions)
                          for name in ('proxy', 'vpn', 'tor', 'datacenter', 'risky')},
        interpretation='Diagnostic feed-derived cohort; no ground truth, precision or recall. Reviewers receive only reviews-blinded.jsonl; overlap review required. Independent positive and negative observations still needed.')
    (output / 'cohort-summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    print(json.dumps(summary, indent=2))


if __name__ == '__main__':
    main()
