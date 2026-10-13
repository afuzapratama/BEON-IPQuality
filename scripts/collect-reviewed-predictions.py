#!/usr/bin/env python3
"""Collect one-version API predictions only after independent labels have been frozen."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import urllib.parse
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--labels', required=True)
    parser.add_argument('--client-env', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    spec = importlib.util.spec_from_file_location('review', Path(__file__).with_name('label-review.py'))
    review = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(review)
    rows = [json.loads(line) for line in Path(args.labels).read_text().splitlines() if line.strip()]
    accepted, unknown = review.reviewed(rows)
    if unknown or not accepted:
        raise ValueError('requires frozen approved independent labels only')
    env = {}
    for line in Path(args.client_env).read_text().splitlines():
        if '=' in line and not line.startswith('#'):
            k, v = line.split('=', 1)
            env[k] = v.strip().strip('\"\'')
    base = env['API_BASE_URL'].rstrip('/')
    if urllib.parse.urlparse(base).scheme != 'https':
        raise ValueError('HTTPS required')
    predictions = []
    for offset in range(0, len(accepted), 50):
        ips = [row['ip'] for row in accepted[offset:offset + 50]]
        req = urllib.request.Request(base + '/api/v1/check/batch', data=json.dumps({'ips': ips}).encode(),
            headers={'Content-Type': 'application/json', 'X-API-Key': env['API_KEY']})
        with urllib.request.urlopen(req, timeout=30) as response:
            predictions.extend(json.load(response)['results'])
    if len(predictions) != len(accepted) or {row['ip'] for row in predictions} != {row['ip'] for row in accepted}:
        raise ValueError('incomplete predictions')
    versions = {row['data_version'] for row in predictions}
    if len(versions) != 1:
        raise ValueError('snapshot changed; recollect before evaluation')
    os.umask(0o077)
    with Path(args.output).open('x') as output:
        output.write(''.join(json.dumps(row) + '\n' for row in predictions))
    print(json.dumps({'samples': len(predictions), 'data_version': next(iter(versions))}))


if __name__ == '__main__':
    main()
