# Deployment langsung VPS ARM64

Target: Linux ARM64, Docker Compose, Nginx host, geoipupdate host, dan monitoring Glances pada phase 4. Tidak memakai aaPanel. Dokumen ini paket pelaksanaan; pengaturan nyata OS/resource/domain/SSH baru diterapkan setelah akses VPS tersedia.

## Direktori dan konfigurasi

| Host | Container / fungsi |
| --- | --- |
| `/opt/beon-ipquality` | Root paket release; configs, migrations, scripts, deployments |
| `/opt/beon-ipquality/data/mmdb` | `/app/data/mmdb`; reputasi dan GeoLite2 City/ASN |
| `/etc/beon-ipquality/GeoIP.conf` atau path pengguna | AccountID/LicenseKey MaxMind, dibaca host saja |
| `/etc/beon-ipquality/geoipupdate.env` | Path file GeoIP.conf dan output MMDB |
| `/opt/beon-ipquality/.env` | Secrets aplikasi/database serta resource configuration |
| `/opt/beon-ipquality/deployment-lock.env` | Image references dengan digest dari release |
| Named volume `<project>_postgres_data` | Database persisten; tidak dihapus saat upgrade |

`MMDB_HOST_DIR` harus sama pada `.env` Compose dan `geoipupdate.env`. Mount direktori, bukan masing-masing file: atomic rename harus terlihat oleh reader. File MaxMind tidak dimasukkan ke image/repository. `geoipupdate -f FILE -d DIR` menimpa DatabaseDirectory pada file konfigurasi: [MaxMind CLI](https://maxmind.github.io/geoipupdate/doc/geoipupdate/).

## Pemeriksaan host pada phase 4

Periksa `uname -m`, OS, vCPU/RAM, disk bebas, port, service existing dan status sudo. Total memory cap pada contoh bukan rekomendasi ukuran VPS; sesuaikan dari hardware dan peak RSS dataset. Jangan overwrite Nginx/Docker existing. Pasang Docker/Compose, Nginx, Certbot dan geoipupdate yang sesuai OS/ARM64. Jika paket geoipupdate distro tidak tersedia, gunakan release ARM64 resmi MaxMind dengan checksum yang diverifikasi, bukan binary amd64.

API/data workers menggunakan user container UID/GID 10001. Setelah direktori proyek final dipilih:

```bash
sudo install -d -m 0755 /opt/beon-ipquality
sudo install -d -m 0750 -o 10001 -g 10001 /opt/beon-ipquality/data/mmdb
```

Glances dipasang pada host dan dibatasi loopback/SSH tunnel; CPU/RAM/network/disk I/O serta overhead-nya diverifikasi sebelum pilot. Port aplikasi internal/database/monitoring tidak dibuka ke internet. Firewall HTTP/HTTPS disesuaikan tanpa memutus SSH.

## Paket release dan image

Unduh [v1.1.0-rc.1](https://github.com/afuzapratama/BEON-IPQuality/releases/tag/v1.1.0-rc.1), release prerelease yang tercatat di tracker, checksum binary archive, `SHA256SUMS`, `deployment-lock.env`, dan `images.json`. Verifikasi checksum sebelum extract. Pilih archive `linux_arm64`; archive berisi binary dan support files dari Git yang tracked, tanpa MMDB/secret runtime. Image service tersedia pada GHCR untuk linux/amd64 dan linux/arm64, dipilih otomatis oleh Docker. Pilih digest, bukan tag mutable.

GHCR package baru dapat bersifat private meskipun repo public. Jika pull memerlukan autentikasi, gunakan token `read:packages` dari file pribadi dengan `docker login ghcr.io --password-stdin`; agent akan menentukan kebutuhan setelah inspeksi akses. Jangan menyalin token ke `.env` proyek atau shell history. Paket release/checksum public tetap dapat dibaca tanpa registry login.

Salin `deployments/production.env.example` ke `.env` dan isi password PostgreSQL, password Redis, API key dan admin key berbeda. Admin key boleh kosong untuk menonaktifkan route admin. `chmod 600 .env`; simpan deployment lock release sebagai file tersendiri. Jangan source `.env` sebagai shell code. Semua script operasi memakai Docker Compose `--env-file`.

## MaxMind dengan file kredensial pilihan pengguna

File harus berformat GeoIP.conf, sekurangnya:

```text
AccountID <account-id>
LicenseKey <license-key>
EditionIDs GeoLite2-City GeoLite2-ASN
```

Pengguna memberi path file asli nanti. Path tersebut dipakai langsung; `DatabaseDirectory` lama di file tidak perlu diubah karena output ditetapkan lewat `-d`.

```bash
GEOIP_CONFIG_FILE='/path/pribadi/GeoIP.conf' \
MMDB_HOST_DIR=/opt/beon-ipquality/data/mmdb \
/opt/beon-ipquality/scripts/update-geoip.sh
```

Tidak ada auto-install package atau copy kredensial dari script updater. Native timer memakai `configs/systemd/beon-geoipupdate.service` dan `.timer`; env template tersedia pada `configs/geoipupdate.env.example`. Agent menyesuaikan WorkingDirectory/ExecStart/ReadWritePaths bila root proyek berbeda. `geoipupdate.env` hanya berisi path, mode 600; GeoIP.conf juga mode 600. AccountID dan LicenseKey tidak dikirim sebagai command arguments/log.

Timer Rabu dan Sabtu pukul 12:00 Asia/Jakarta (WIB), random delay sampai 30 menit dan Persistent. Output GeoLite2 mode 644 agar reader container dapat membacanya. Periksa cron/timer bawaan paket dan updater existing agar dataset proyek hanya mempunyai satu updater; workload lain tidak diubah tanpa pemeriksaan. Host timer adalah satu-satunya updater produksi; profil updater container pada Compose development tidak dijalankan bersamaan. Setelah update, compiler membaca ASN pada compile berikutnya dan API/Judge memeriksa checksum pada reload berikutnya. Download MaxMind nyata memerlukan kredensial pengguna; fixture test hanya memverifikasi routing path.

## Bootstrap

Setelah image, secret dan direktori siap, GeoLite2 dapat di-update terlebih dahulu. Tanpa kredensial, reputasi tetap bisa diuji; Geo/ASN opsional dan klasifikasi ASN belum tersedia sampai database valid hadir.

```bash
cd /opt/beon-ipquality
docker compose --env-file .env --env-file deployment-lock.env -f deployments/compose.production.yml config --quiet
docker compose --env-file .env --env-file deployment-lock.env -f deployments/compose.production.yml pull
docker compose --env-file .env --env-file deployment-lock.env -f deployments/compose.production.yml up -d
```

Urutan service: PostgreSQL sehat → migrate → ingestor-init → compiler-init → API → daemon ingest/compiler. Bootstrap berhenti jika feed gagal atau compiler kosong; periksa log dan perbaiki source sebelum mengulang. Daemon tidak mengulang init, tetapi harus menemukan MMDB valid. Retired/unreachable feed tidak boleh dibiarkan enabled lalu dianggap sukses. Pilot harus mengecek semua feed nyata.

Health lokal 200, lookup berkey 200, tanpa key 401, IPv6/batch/whitelist sesuai kontrak. `./compiler -verify-mmdb data/mmdb` dapat memvalidasi artefak offline. Test compiler hanya memeriksa integrity, bukan menjamin feed fresh. Jangan menganggap `up -d` berarti semua job berhasil.

## Nginx dan HTTPS

Gunakan `configs/nginx/beon-acme-bootstrap.conf` saat sertifikat belum ada. Ganti `ipq.example.com`, siapkan webroot ACME, uji Nginx, lalu terbitkan sertifikat melalui Certbot webroot. Setelah itu gunakan `beon-production.conf`; ganti domain, certificate path, dan upstream port sesuai `.env`. Uji `nginx -t` sebelum reload.

Template publik hanya memproxy `/api/v1/check/`; health, metrics, stats, reload, cache dan Judge mendapat 404 dari publik. API host port selalu 127.0.0.1. Tentukan peer IP Nginx yang terlihat melalui Docker bridge dan isi `TRUSTED_PROXIES` secara eksplisit; jangan percaya seluruh 0.0.0.0/0. Nginx menimpa forwarding header dengan client address. CDN tambahan memerlukan konfigurasi real-IP tersendiri, berdasarkan IP proxy yang dipercaya.

Uji HTTPS lookup dengan/tanpa key, route admin publik ditolak, health lokal tetap tersedia saat kuota lookup habis, serta header spoofing. Uji renewal sertifikat dan reload hook. Setting domain/sertifikat belum diterapkan di environment lokal.

## Upgrade dan migrasi

Runner `scripts/migrate.sh` memakai advisory lock satu session, transaksi per file, dan checksum `schema_migrations`; rerun tidak mengulang migrasi dan perubahan file migration lama ditolak. Tambah file SQL baru untuk upgrade. Untuk DB legacy, baseline 001 bersifat idempotent dan diadopsi ke ledger; 002 membatasi row legacy tanpa expiry hingga last_seen +48h. Bootstrap semua feed lagi sesudah upgrade agar TTL baru terpasang.

Urutan upgrade: backup + restore-check → simpan lock/config release lama → hentikan worker ingest/compiler → pasang support files baru tanpa menimpa `.env`/data → pull image → `docker compose ... run --rm migrate` → bootstrap ulang bila perubahan dataset/schema membutuhkan → recreate service dengan `up -d --force-recreate` → health/lookup/monitoring. Jangan mengandalkan service migrate yang pernah exit 0 untuk menjalankan SQL baru; run eksplisit.

Tidak menggunakan `docker compose down -v`. Migrasi rollback tidak dilakukan otomatis: restore DB backup jika schema tidak kompatibel dengan image lama. Lihat [backup/restore/rollback](BACKUP_RESTORE_ROLLBACK.md).

## Data policy

Satu source dipublikasi dalam transaksi COPY/upsert/expiry. IP yang hilang dari hasil lengkap kedaluwarsa segera; snapshot kosong/malformed/truncated/failed storage ditolak dan tidak menghapus dataset lama. IP yang muncul kembali mempertahankan first_seen sampai row historis dipruning.

TTL aktif adalah dua kali freshness budget source saat fetch berhasil; ini lebih longgar daripada health freshness agar data tidak berubah menjadi clean hanya karena fetch gagal. Freshness health tetap satu interval jadwal + configured compile/reload/retry budget. Record expired tidak masuk compiler. Data expired disimpan 30 hari untuk audit, lalu dihapus. Fetch history 30 hari, dengan latest attempt dan latest success per source tetap disimpan. Cleanup harian pada ingestor; PostgreSQL autovacuum menangani ruang mati. Penghapusan source dari konfigurasi tidak me-refresh data tersebut; row-nya habis melalui TTL.

Transaksi mencegah publication parsial. Akurasi parser, perubahan sumber, volume data dan waktu copy/compile nyata tetap perlu validasi pilot/load test. Snapshot MMDB lama tetap disimpan saat compile/reload gagal; lookup menolak source/snapshot yang melewati freshness.

## Pilot ARM 4 GB yang telah diuji

Lihat [hasil pilot 12 Oktober 2026](VPS_ARM_PILOT_2026-10-12.md) dan manifest
`deployments/releases/v1.1.0-rc.2.env`. Untuk fresh deployment host setara,
salin `configs/feeds.pilot.arm-4gb.yaml` ke konfigurasi aktif `configs/feeds.yaml`
sebelum bootstrap. Template mengecualikan FireHOL Anonymous (OOM), source kosong,
dan URL datacenter/VPN duplikat. Menonaktifkan feed pada DB existing tidak otomatis
mengexpire row; lakukan perubahan terencana dengan backup dan expiry/filter.

Host pilot memakai Caddy existing, bukan Nginx. Tambahkan virtual host dari
`configs/caddy/beon-production.caddyfile`, pertahankan blok domain existing,
validasi konfigurasi dan gunakan SIGUSR1 apabila admin API disabled. Bila file bind
mount menunjuk inode lama, recreate hanya service proxy dengan volume sertifikat
tetap dipertahankan. Pasang Nginx hanya bila belum ada reverse proxy host.

Glances distro Ubuntu 26.04 tidak menyertakan asset web; pilot memakai venv
`/opt/beon-monitor` dengan `glances[web]==4.5.8`, dependensi aktual dikunci di
`/opt/beon-monitor/requirements.lock`. Service `configs/systemd/beon-glances.service`
memakai user `glances` dan loopback 61208; akses melalui SSH tunnel.
Template `beon-backup.service`/timer membuat bundle privat setiap hari; enable
setelah backup/restore pertama berhasil. Retention/offsite/alert tetap harus diselesaikan.

Jadwal MaxMind disesuaikan 13 Oktober 2026: GeoLite2 City/Country dirilis Selasa dan Jumat, sedangkan ASN harian. Proyek mengambil City/ASN bersama dua kali seminggu untuk mengurangi frekuensi akses. Pukul 12:00 WIB pada hari berikutnya merupakan jeda operasional, bukan jaminan jam rilis upstream. Rilis terlambat atau di luar jadwal akan diambil pada eksekusi berikutnya.

Referensi: https://dev.maxmind.com/geoip/release-notes/2025-07-16-a-number-of-geoip-databases-now-released-every-weekday/ dan https://dev.maxmind.com/geoip/release-notes/2024-06-18-change-to-geolite2-asn-update-schedule/
