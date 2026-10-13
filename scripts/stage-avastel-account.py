#!/usr/bin/env python3
"""Download and normalize Avastel account CSV privately; no production writes."""
import argparse
import csv
from datetime import datetime, timezone
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import resource
import sqlite3
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile
import io

ENDPOINT = 'https://deviceandbrowserinfo.com/proxy-api/api/ips/db/free-trial/csv.zip'
MAX_ZIP = 128 * 1024 * 1024
MAX_CSV = 512 * 1024 * 1024


def read_key(path):
    text = Path(path).read_text().strip()
    if len(text.splitlines()) != 1:
        raise ValueError('expected one token or one assignment')
    if '://' in text:
        raise ValueError('provide token only, not URL')
    if '=' in text:
        text = text.split('=', 1)[1].strip()
    text = text.strip('\"\'')
    if not text or any(ch.isspace() for ch in text):
        raise ValueError('invalid token format')
    return text


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # Query credentials must never follow redirects to another endpoint.
        return None


def normalize(archive, directory):
    with zipfile.ZipFile(archive) as bundle:
        members = [entry for entry in bundle.infolist() if not entry.is_dir()]
        if len(members) != 1 or not members[0].filename.lower().endswith('.csv'):
            raise ValueError('expected exactly one CSV file')
        member = members[0]
        if member.file_size > MAX_CSV:
            raise ValueError('CSV exceeds expanded size bound')
        db = sqlite3.connect(directory / 'dedup.sqlite')
        try:
            db.execute('PRAGMA cache_size=-8192')
            db.execute('CREATE TABLE ips (ip TEXT PRIMARY KEY) WITHOUT ROWID')
            rows = invalid = excluded = proxy_rows = 0
            with bundle.open(member) as binary:
                reader = io.TextIOWrapper(binary, encoding='utf-8-sig', newline='')
                first = reader.readline()
                while first.startswith('#') or not first.strip():
                    first = reader.readline()
                    if not first:
                        raise ValueError('empty CSV')
                delimiter = ';' if ';' in first else ','
                header = next(csv.reader([first], delimiter=delimiter))
                aliases = ('ip_address', 'ip', 'ipaddress')
                normalized = [name.strip().lower() for name in header]
                indices = [i for i, name in enumerate(normalized) if name in aliases]
                if len(indices) != 1:
                    raise ValueError('CSV lacks unambiguous IP column')
                index = indices[0]
                if "isproxy" not in normalized:
                    raise ValueError("account CSV must include isProxy column")
                flag_index = normalized.index("isproxy")
                batch = []
                for row in csv.reader(reader, delimiter=delimiter):
                    if not row or row[0].startswith('#'):
                        continue
                    rows += 1
                    try:
                        addr = ipaddress.ip_address(row[index].strip())
                        if getattr(addr, 'ipv4_mapped', None):
                            addr = addr.ipv4_mapped
                        if not addr.is_global:
                            raise ValueError('non-global')
                        if flag_index is not None:
                            flag = row[flag_index].strip().lower()
                            if flag not in ("true", "false"):
                                raise ValueError("invalid isProxy flag")
                            if flag == "false":
                                excluded += 1
                                continue
                        proxy_rows += 1
                        batch.append((str(addr),))
                    except (ValueError, IndexError):
                        invalid += 1
                    if len(batch) >= 5000:
                        db.executemany('INSERT OR IGNORE INTO ips VALUES (?)', batch)
                        batch.clear()
                db.executemany('INSERT OR IGNORE INTO ips VALUES (?)', batch)
                db.commit()
            if invalid or not rows:
                raise ValueError('invalid IP rows or empty dataset; no accepted manifest')
            count = db.execute('SELECT COUNT(*) FROM ips').fetchone()[0]
            if not count:
                raise ValueError('no proxy-positive records')
            with (directory / 'proxy-ips.txt').open('w') as output:
                for (ip,) in db.execute('SELECT ip FROM ips ORDER BY ip'):
                    output.write(ip+'\n')
            return dict(csv_bytes=member.file_size, csv_columns=header, delimiter=delimiter,
                        parsed_rows=rows, proxy_rows=proxy_rows, excluded_nonproxy_rows=excluded,
                        unique_ips=count, duplicates=proxy_rows-count, invalid_rows=invalid)
        finally:
            db.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--key-file', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    os.umask(0o077)
    directory = Path(args.output)
    directory.mkdir(mode=0o700, parents=True, exist_ok=False)
    started = time.monotonic()
    key = read_key(args.key_file)
    url = ENDPOINT+'?'+urllib.parse.urlencode({'api_key': key})
    archive = directory / 'dataset.csv.zip'
    digest = hashlib.sha256()
    size = 0
    opener = urllib.request.build_opener(NoRedirect())
    request = urllib.request.Request(url, headers={'User-Agent': 'BEON-IPQuality-private-staging'})
    with opener.open(request, timeout=60) as response, archive.open('wb') as output:
        while chunk := response.read(1024 * 1024):
            size += len(chunk)
            if size > MAX_ZIP:
                raise ValueError('ZIP exceeds download bound')
            digest.update(chunk)
            output.write(chunk)
    downloaded = time.monotonic()
    details = normalize(archive, directory)
    manifest = dict(endpoint=ENDPOINT, fetched_at=datetime.now(timezone.utc).isoformat(),
                    zip_sha256=digest.hexdigest(), zip_bytes=size,
                    normalized_sha256=hashlib.sha256((directory/'proxy-ips.txt').read_bytes()).hexdigest(),
                    download_seconds=downloaded-started, normalization_seconds=time.monotonic()-downloaded,
                    peak_rss_kib=resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
                    measurement_scope='Local downloader + SQLite bounded-cache dedup; not Go ingest/compiler or ARM benchmark',
                    production_enabled=False, redistribution_authorized=False, **details)
    (directory/'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')
    print(json.dumps(manifest, indent=2))


if __name__ == '__main__':
    try:
        main()
    except urllib.error.HTTPError as error:
        raise SystemExit(f'Staging failed: HTTP {error.code}; no accepted manifest') from None
    except Exception as error:
        raise SystemExit(f'Staging failed: {type(error).__name__}; no accepted manifest; secrets redacted') from None
