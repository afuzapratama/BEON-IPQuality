# Uji pemulihan dari Bunny di host terpisah

Backup offsite dienkripsi age. Download dengan `scripts/operations.py download-latest`
(memerlukan environment Bunny privat), verifikasi checksum ciphertext, lalu decrypt
**di komputer pemegang private key**. Private key age tidak disalin ke VPS.
Tar hasil decrypt dan direktori hasil extract harus privat, mode 0600/0700.

```bash
age -d -i /private/ipproxy-backup.agekey -o /private/backup.tar /private/backup.tar.age
# Extract menggunakan tarfile filter=data ke direktori baru yang privat.
python3 scripts/disaster-restore-drill.py \
  --backup /private/extracted/BACKUP_ID --output /private/new-drill
```

Drill memeriksa seluruh checksum komponen wajib, memulihkan konfigurasi/MMDB,
membuat project Docker acak dengan DB/Redis baru, memulihkan pg_dump dan menjalankan
migrasi idempotent. Feed reguler di-refresh, MMDB dikompilasi ulang dan diverifikasi,
kemudian API diuji melalui port loopback acak dengan key sintetis. Uji meliputi
health/freshness, sampel account proxy, IPv6, whitelist dan penolakan autentikasi.
Hanya container/network/volume project acak tersebut dibersihkan. Tidak ada service
host, Caddy, DNS, Telegram atau timer produksi yang dijalankan/diganti oleh drill.
Log, laporan dan konfigurasi hasil restore tetap privat untuk pemeriksaan operator.

Jalankan di host Docker terpisah dengan minimal sekitar 4 GB RAM tersedia dan disk
cukup. Limit compiler 2,5 GiB; pengujian ini tidak mengukur kapasitas VPS ARM.
Digest lock berasal dari backup dan image harus masih dapat di-pull.

Account `externally_managed` tidak di-download ulang oleh ingestor. Drill ini memakai
backup baru dengan account source yang masih dalam freshness budget. Untuk recovery
backup lama, restore key provider ke `/etc/beon-ipquality/avastel.key`, pasang source
service privat, kemudian jalankan `avastel-sync.py prepare` setelah DB/migrasi siap,
sebelum compile/API. Lihat [runbook account](AVASTEL_PRIVATE_PRODUCTION.md).
Jangan memperpanjang freshness budget untuk menyembunyikan data yang kedaluwarsa.

Waktu yang diukur drill dimulai setelah download/decrypt/extract. Ini pengujian
pemulihan data dan layanan pada host terpisah, bukan RTO end-to-end kehilangan VPS:
provisioning OS, firewall, DNS/TLS, timer dan upstream account refresh perlu diuji
secara terpisah untuk klaim RTO tersebut.

Hasil drill 13 Oktober 2026: backup Bunny `20261012T234730Z-20383`, restore 1.287.574 rows, migrasi/refresh/compile/verify lulus, API healthy dan 25 account samples, auth/IPv6/whitelist lulus. Durasi 129,334 detik setelah download/decrypt/extract; cleanup project uji lulus. [Evidence](evidence/phase6-bunny-disaster-restore-2026-10-13.json).
