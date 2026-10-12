#!/usr/bin/env python3
"""Verified encrypted Bunny S3 backups and lightweight Telegram alerts.
Secrets are read from the process environment, never logged or passed in argv.
"""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
import urllib.request
import urllib.error

UTC = dt.timezone.utc
BUNDLE = re.compile(r"^(\d{8}T\d{6}Z)-\d+$")


def root():
    return Path(os.environ.get("PROJECT_ROOT", "/opt/beon-ipquality"))


def backups():
    return Path(os.environ.get("BACKUP_DIR", str(root() / "backups")))


def state_dir():
    p = Path(os.environ.get("OPS_STATE_DIR", "/var/lib/beon-ipquality"))
    p.mkdir(mode=0o700, parents=True, exist_ok=True)
    return p


def atomic_json(path, value):
    tmp = path.with_suffix(path.suffix + ".tmp")
    with tmp.open("w") as f:
        json.dump(value, f, indent=2)
    tmp.chmod(0o600)
    os.replace(tmp, path)


def stamp(name):
    match = BUNDLE.fullmatch(name)
    if not match:
        return None
    try:
        return dt.datetime.strptime(match[1], "%Y%m%dT%H%M%SZ").replace(tzinfo=UTC)
    except ValueError:
        return None


def complete_bundles():
    return sorted(p for p in backups().iterdir() if p.is_dir() and not p.is_symlink()
                  and stamp(p.name) and (p / "COMPLETE").is_file())


def sha256(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def bunny_client():
    import boto3
    from botocore.config import Config
    region = os.environ["BUNNY_REGION"]
    endpoint = os.environ["BUNNY_ENDPOINT"].rstrip("/")
    if endpoint != f"https://{region}-s3.storage.bunnycdn.com":
        raise ValueError("Use the regional Bunny S3 HTTPS endpoint")
    return boto3.client("s3", endpoint_url=endpoint, region_name=region,
        aws_access_key_id=os.environ["BUNNY_ACCESS_KEY_ID"],
        aws_secret_access_key=os.environ["BUNNY_SECRET_ACCESS_KEY"],
        config=Config(signature_version="s3v4", s3={"addressing_style": "path"},
            request_checksum_calculation="when_required", response_checksum_validation="when_required",
            connect_timeout=10, read_timeout=60, retries={"max_attempts": 3, "mode": "standard"}))


def has_offsite():
    return all(os.environ.get(key) for key in ["BUNNY_BUCKET", "BUNNY_REGION", "BUNNY_ENDPOINT",
                                               "BUNNY_ACCESS_KEY_ID", "BUNNY_SECRET_ACCESS_KEY"])


def prefix():
    value = os.environ.get("BUNNY_PREFIX", "beon-ipquality").strip("/")
    if not re.fullmatch(r"[A-Za-z0-9/_-]+", value) or ".." in value:
        raise ValueError("Invalid backup prefix")
    return value + "/"


def seal(bundle):
    directory = backups() / "sealed"
    directory.mkdir(mode=0o700, exist_ok=True)
    target = directory / (bundle.name + ".tar.age")
    temp = target.with_suffix(".tmp")
    recipient = os.environ.get("BACKUP_RECIPIENT_FILE", "/etc/beon-ipquality/backup.recipient")
    tar = subprocess.Popen(["tar", "-C", str(bundle.parent), "-cf", "-", bundle.name], stdout=subprocess.PIPE)
    try:
        age = subprocess.run(["age", "-R", recipient, "-o", str(temp)], stdin=tar.stdout,
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        tar.stdout.close()
        if tar.wait() != 0 or age.returncode != 0:
            raise RuntimeError("Backup encryption failed")
        temp.chmod(0o600)
        os.replace(temp, target)
    finally:
        if tar.poll() is None:
            tar.kill()
            tar.wait()
        temp.unlink(missing_ok=True)
    return target


def verify_remote(client, bucket, key, digest):
    body = client.get_object(Bucket=bucket, Key=key)["Body"]
    h = hashlib.sha256()
    try:
        for chunk in iter(lambda: body.read(1024 * 1024), b""):
            h.update(chunk)
    finally:
        body.close()
    if h.hexdigest() != digest:
        raise RuntimeError("Offsite readback checksum mismatch")


def upload(bundle, encrypted):
    from boto3.s3.transfer import TransferConfig
    client = bunny_client()
    bucket = os.environ["BUNNY_BUCKET"]
    key = prefix() + encrypted.name
    digest = sha256(encrypted)
    client.upload_file(str(encrypted), bucket, key,
        Config=TransferConfig(multipart_threshold=100 * 1024 * 1024, max_concurrency=2))
    client.put_object(Bucket=bucket, Key=key + ".sha256", Body=(digest + "\n").encode())
    verify_remote(client, bucket, key, digest)
    atomic_json(bundle / "OFFSITE_VERIFIED", {"key": key, "bucket": bucket, "sha256": digest, "time": time.time()})
    print("Offsite upload and full readback verified:", key)
    return client, bucket, key


def local_prune(latest, now=None):
    # No automatic deletion until the newest local recovery point passed restore
    # and has a verified offsite copy. Keep at least seven verified local bundles.
    if not (latest / "VERIFIED").is_file() or not (latest / "OFFSITE_VERIFIED").is_file():
        print("Local retention deferred until latest backup is offsite-verified")
        return []
    days = max(7, int(os.environ.get("LOCAL_RETENTION_DAYS", "7")))
    cutoff = (now or dt.datetime.now(UTC)) - dt.timedelta(days=days)
    bundles = complete_bundles()
    protected = set(p for p in bundles if (p / "VERIFIED").is_file())
    protected = set(sorted(protected)[-7:]) | {latest}
    deleted = []
    for p in bundles:
        if p in protected or stamp(p.name) >= cutoff or not (p / "VERIFIED").is_file():
            continue
        shutil.rmtree(p)
        (backups() / "sealed" / (p.name + ".tar.age")).unlink(missing_ok=True)
        deleted.append(p.name)
    return deleted


def offsite_prune(client, bucket, newest):
    days = max(30, int(os.environ.get("OFFSITE_RETENTION_DAYS", "30")))
    cutoff = dt.datetime.now(UTC) - dt.timedelta(days=days)
    for page in client.get_paginator("list_objects_v2").paginate(Bucket=bucket, Prefix=prefix()):
        for item in page.get("Contents", []):
            key = item["Key"]
            if not key.startswith(prefix()) or key == newest or not key.endswith(".tar.age"):
                continue
            name = key[len(prefix()):-len(".tar.age")]
            if stamp(name) and stamp(name) < cutoff:
                # Bunny does not implement batch DeleteObjects.
                client.delete_object(Bucket=bucket, Key=key)
                client.delete_object(Bucket=bucket, Key=key + ".sha256")


def notify(message):
    token, chat = os.environ.get("TELEGRAM_BOT_TOKEN"), os.environ.get("TELEGRAM_CHAT_ID")
    if not token or not chat:
        print("Telegram pending: token/chat ID not configured")
        return False
    if os.environ.get("BEON_ALERT_TEST_MODE") == "true":
        message = "[TEST] " + message
    body = {"chat_id": chat, "text": message, "disable_web_page_preview": True}
    if os.environ.get("TELEGRAM_MESSAGE_THREAD_ID"):
        body["message_thread_id"] = int(os.environ["TELEGRAM_MESSAGE_THREAD_ID"])
    req = urllib.request.Request("https://api.telegram.org/bot" + token + "/sendMessage",
                                 data=json.dumps(body).encode(), headers={"Content-Type": "application/json"})
    # Never log the request URL or HTTP exception: both can contain the bot token.
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            if not json.load(response).get("ok"):
                raise RuntimeError("Telegram rejected notification")
    except Exception:
        raise RuntimeError("Telegram notification failed") from None
    return True


def finalize():
    bundles = complete_bundles()
    if not bundles:
        raise RuntimeError("No complete backup")
    latest = bundles[-1]
    subprocess.run([str(root() / "scripts/restore-check.sh"), str(latest)], check=True)
    atomic_json(latest / "VERIFIED", {"time": time.time(), "restore": "isolated PostgreSQL and MMDB"})
    encrypted = seal(latest)
    print("Encrypted recovery bundle:", encrypted)
    if has_offsite():
        client, bucket, key = upload(latest, encrypted)
        offsite_prune(client, bucket, key)
    else:
        print("Bunny S3 pending: credentials not configured")
    print("Local bundles pruned:", local_prune(latest))
    notify("BEON backup verified: " + latest.name + ("; Bunny readback verified" if has_offsite() else "; offsite pending"))



def pipeline_failures():
    """Track new error counters and clear only after a later successful attempt."""
    path=state_dir()/"pipeline-state.json"
    established=path.exists()
    previous=json.loads(path.read_text()) if established else {}
    current={};problems=[]
    project=os.environ.get("COMPOSE_PROJECT_NAME","beon-ipquality")
    for service,port in [("ingestor",9091),("compiler",9092)]:
        try:
            text=subprocess.check_output(["docker","exec",project+"-"+service+"-1","wget","-q","-O","-","http://127.0.0.1:"+str(port)+"/metrics"],stderr=subprocess.DEVNULL,timeout=8).decode()
            for line in text.splitlines():
                metric=re.match(r'ipquality_(feed_fetches|compilations)_total\{([^}]+)\} ([0-9.eE+-]+)$',line)
                if not metric: continue
                labels=dict(re.findall(r'(\w+)="([^"]*)"',metric[2]))
                status=labels.get("status")
                if status not in ["error","success"]: continue
                key="compile" if service=="compiler" else "feed/"+labels.get("feed","")+"/"+labels.get("source","")
                current.setdefault(key,{"error":0,"success":0})[status]=float(metric[3])
        except Exception:
            problems.append(service+" internal metrics unavailable")
    for key,counts in current.items():
        old=previous.get(key, {"error":0,"success":0,"active":False} if established else None)
        active=bool(old and old.get("active"))
        if old and (counts["error"]<old["error"] or counts["success"]<old["success"]): active=False
        elif old and counts["error"]>old["error"]: active=True
        elif old and counts["success"]>old["success"]: active=False
        counts["active"]=active
        if active: problems.append("Pipeline attempt failed: "+key)
    atomic_json(path,current)
    return problems


def checks():
    problems = []
    try:
        with urllib.request.urlopen(os.environ.get("HEALTH_URL", "http://127.0.0.1:8080/health"), timeout=8) as r:
            data=json.load(r)
            if r.status != 200 or data.get("status") != "healthy":
                problems.append("API health or feed/snapshot freshness failed")
            for feed in data.get("snapshot",{}).get("feeds",[]):
                if feed.get("latest_status")=="error":
                    problems.append("Feed refresh failed: "+feed.get("feed","")+"/"+feed.get("source",""))
    except Exception:
        problems.append("API health or feed/snapshot freshness failed")
    project = os.environ.get("COMPOSE_PROJECT_NAME", "beon-ipquality")
    for service in ["api", "postgres", "redis", "ingestor", "compiler"]:
        try:
            output = subprocess.check_output(["docker", "inspect", "--format", "{{.State.Running}} {{.State.OOMKilled}}",
                                              project + "-" + service + "-1"], stderr=subprocess.DEVNULL, timeout=5).decode().strip()
            if output != "true false":
                problems.append(service + " container down or OOM")
        except Exception:
            problems.append(service + " container inspection failed")
    usage = shutil.disk_usage(root())
    if usage.used / usage.total >= .8 or usage.free < 2 * 1024**3:
        problems.append("Disk usage >=80% or free space <2 GiB")
    mem = {key: int(value.strip().split()[0]) for key, value in
           (line.split(":", 1) for line in Path("/proc/meminfo").read_text().splitlines())}
    if mem["MemAvailable"] / mem["MemTotal"] < .1:
        problems.append("Available RAM <10%")
    verified = [p for p in complete_bundles() if (p / "VERIFIED").is_file()]
    if not verified or time.time() - stamp(verified[-1].name).timestamp() > 36 * 3600:
        problems.append("No restore-verified backup in 36 hours")
    if has_offsite():
        uploaded = [p for p in verified if (p / "OFFSITE_VERIFIED").is_file()]
        if not uploaded or time.time() - stamp(uploaded[-1].name).timestamp() > 36 * 3600:
            problems.append("No offsite-verified backup in 36 hours")
    for unit in ["beon-backup.service", "beon-geoipupdate.service", "beon-avastel-update.service", "beon-avastel-source.service"]:
        try:
            result = subprocess.check_output(["systemctl", "show", unit, "--value", "-p", "Result"], timeout=5).decode().strip()
            if result != "success":
                problems.append(unit + " failed")
        except Exception:
            problems.append(unit + " status unavailable")
    problems.extend(pipeline_failures())
    return sorted(problems)


def monitor():
    path = state_dir() / "alert-state.json"
    previous = json.loads(path.read_text()) if path.exists() else {}
    problems = checks()
    failures = previous.get("failures", 0) + 1 if problems else 0
    delivered = previous.get("delivered", [])
    sent = previous.get("sent", 0)
    if failures >= 2 and problems and (problems != delivered or time.time() - sent >= 6 * 3600):
        if notify("BEON alert:\n" + "\n".join(problems)):
            delivered, sent = problems, time.time()
    elif not problems and delivered:
        if notify("BEON recovered: all monitored checks healthy"):
            delivered, sent = [], time.time()
    atomic_json(path, {"failures": failures, "delivered": delivered, "sent": sent, "checked": time.time()})
    print(json.dumps({"problems": problems, "consecutive_failures": failures, "telegram_configured": bool(os.environ.get("TELEGRAM_BOT_TOKEN"))}))


def download(key, destination):
    if not key.startswith(prefix()) or not stamp(key[len(prefix()):-len(".tar.age")]) or not key.endswith(".tar.age"):
        raise ValueError("Download key must identify a project backup")
    client = bunny_client()
    bucket = os.environ["BUNNY_BUCKET"]
    digest_body = client.get_object(Bucket=bucket, Key=key + ".sha256")["Body"]
    try:
        digest = digest_body.read(256).decode().strip()
    finally:
        digest_body.close()
    if not re.fullmatch(r"[a-f0-9]{64}", digest):
        raise ValueError("Invalid remote checksum")
    target = Path(destination)
    if target.exists():
        raise ValueError("Download destination exists")
    tmp = target.with_suffix(target.suffix + ".partial")
    created = False
    try:
        with tmp.open("xb") as f:
            created = True
            tmp.chmod(0o600)
            client.download_fileobj(bucket, key, f)
        if sha256(tmp) != digest:
            raise RuntimeError("Downloaded backup checksum mismatch")
        os.replace(tmp, target)
    finally:
        if created:
            tmp.unlink(missing_ok=True)
    print("Encrypted backup downloaded and verified:", target)


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=["finalize-latest", "monitor", "notify-test", "download", "download-latest"])
    parser.add_argument("--key")
    parser.add_argument("--output")
    args = parser.parse_args()
    state = state_dir()
    with (state / (args.command + ".lock")).open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.command == "finalize-latest": finalize()
        elif args.command == "monitor": monitor()
        elif args.command == "notify-test":
            if not notify("BEON test: Telegram alert connected"):
                raise RuntimeError("Telegram credentials required")
        elif args.command == "download-latest":
            if not args.output: raise ValueError("Download requires --output")
            client=bunny_client();bucket=os.environ["BUNNY_BUCKET"]
            keys=set()
            for page in client.get_paginator("list_objects_v2").paginate(Bucket=bucket,Prefix=prefix()):
                keys.update(item["Key"] for item in page.get("Contents",[]))
            candidates=[key for key in keys if key.startswith(prefix()) and key.endswith(".tar.age")
                        and stamp(key[len(prefix()):-len(".tar.age")]) and key+".sha256" in keys]
            if not candidates: raise ValueError("No complete offsite backup")
            download(max(candidates),args.output)
        elif args.command == "download":
            if not args.key or not args.output: raise ValueError("Download requires --key and --output")
            download(args.key, args.output)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # Exceptions from providers can include URLs/secrets. Report type only.
        print("Operation failed:", type(error).__name__, file=sys.stderr)
        sys.exit(1)
