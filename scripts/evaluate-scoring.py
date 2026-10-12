#!/usr/bin/env python3
"""Offline evaluation of independent JSONL labels against saved API responses."""
import argparse
import ipaddress
import json
from pathlib import Path

CATEGORIES = ('risky', 'proxy', 'vpn', 'tor', 'datacenter', 'botnet', 'spam', 'malware', 'attacker')


def load_records(path):
    records = {}
    for number, line in enumerate(Path(path).read_text().splitlines(), 1):
        if not line.strip():
            continue
        row = json.loads(line)
        addr = ipaddress.ip_address(row['ip'])
        if getattr(addr, 'ipv4_mapped', None):
            addr = addr.ipv4_mapped
        ip = str(addr)
        if ip in records:
            raise ValueError(f'duplicate IP at line {number}')
        records[ip] = row
    if not records:
        raise ValueError('empty input')
    return records


def ratio(numerator, denominator):
    return numerator / denominator if denominator else None


def evaluate(labels, predictions, threshold=None):
    if labels.keys() != predictions.keys():
        raise ValueError('labels and predictions must contain exactly the same IPs')
    versions = {row.get('data_version') for row in predictions.values()}
    if len(versions) != 1 or not all(isinstance(v, str) and v for v in versions):
        raise ValueError('predictions must share one nonempty data_version')
    result = {name: dict(tp=0, fp=0, tn=0, fn=0) for name in CATEGORIES}
    mismatches = []
    for ip, record in labels.items():
        if not record.get('label_source') or not record.get('observed_at'):
            raise ValueError('labels require label_source and observed_at provenance')
        if record.get('independent') is not True:
            raise ValueError('labels must declare independent=true after provenance review')
        truth = record.get('labels')
        if not isinstance(truth, dict) or not truth or set(truth) - set(CATEGORIES):
            raise ValueError('labels must contain supported categories')
        prediction = predictions[ip]
        for name, actual in truth.items():
            if type(actual) is not bool:
                raise ValueError('labels must be true/false; omit unknown categories')
            predicted = prediction.get(name)
            if name == 'risky' and threshold is not None:
                score = prediction.get('score')
                if type(score) is not int or not 0 <= score <= 100:
                    raise ValueError('threshold evaluation requires integer score 0..100')
                predicted = False if prediction.get('whitelisted') is True else score >= threshold
            if type(predicted) is not bool:
                raise ValueError('missing or invalid prediction flag')
            bucket = ('tp' if actual else 'fp') if predicted else ('fn' if actual else 'tn')
            result[name][bucket] += 1
            if predicted != actual:
                mismatches.append({'ip': ip, 'category': name, 'error': bucket})
    for counts in result.values():
        tp, fp, tn, fn = (counts[k] for k in ('tp', 'fp', 'tn', 'fn'))
        counts.update(n=tp+fp+tn+fn, precision=ratio(tp, tp+fp), recall=ratio(tp, tp+fn),
                      false_positive_rate=ratio(fp, fp+tn), f1=ratio(2*tp, 2*tp+fp+fn))
    return {'samples': len(labels), 'data_version': next(iter(versions)),
            'risky_threshold_override': threshold, 'categories': result, 'mismatches': mismatches}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--labels', required=True)
    parser.add_argument('--predictions', required=True)
    parser.add_argument('--threshold', type=int)
    args = parser.parse_args()
    if args.threshold is not None and not 0 <= args.threshold <= 100:
        parser.error('threshold must be 0..100')
    try:
        report = evaluate(load_records(args.labels), load_records(args.predictions), args.threshold)
    except (ValueError, KeyError, OSError) as error:
        parser.exit(1, f'Evaluation rejected: {error}\n')
    print(json.dumps(report, indent=2, allow_nan=False))


if __name__ == '__main__':
    main()
