# Pilot ARM — 12 Oktober 2026

## Host dan keputusan deployment

Target domain `ipproxy.beon-api.site`, host ARM64 Ubuntu 26.04.1 LTS,
2 vCPU, RAM 3.8 GiB, root disk 29 GiB (26 GiB kosong saat inspeksi).
DNS A cocok dengan VPS. SSH/sudo berhasil; Docker Engine/Compose tersedia.

Host existing menjalankan proyek Docker `sl-a249ebf2bedd` di
`/opt/shortlink-agent`: agent loopback 8091 dan Caddy host-network di 80/443
untuk `armtest.beon-api.site`. Pertahankan workload ini. Gunakan Caddy existing
untuk virtual host IPQuality; tidak memasang Nginx yang berebut port.
Caddyfile dibackup sebelum perubahan dan divalidasi sebelum reload SIGUSR1
(admin API existing dinonaktifkan). Template: `configs/caddy/beon-production.caddyfile`.
Referensi reload: https://caddyserver.com/docs/command-line#signals.

IPQuality terisolasi dalam `/opt/beon-ipquality`, project Compose
`beon-ipquality`, API loopback 8080, PostgreSQL/Redis tanpa host port.
Secret runtime mode 0600, generated di server; tidak disalin ke repository.
Direktori MMDB UID/GID 10001 mode 0750. Trusted proxy hanya gateway jaringan
Docker yang telah diperiksa, `172.18.0.1/32`.

## GeoIP dan monitoring

Native geoipupdate 7.1.1 dipasang dari paket Ubuntu ARM64.
Konfigurasi privat `/etc/beon-ipquality/GeoIP.conf` mode 0600; output
`/opt/beon-ipquality/data/mmdb`, dibaca container sebagai `/app/data/mmdb`.
Download GeoLite2-City (~49 MiB) dan GeoLite2-ASN (~8.6 MiB) berhasil;
unit oneshot Result=success/ExecMainStatus=0. Timer proyek aktif: 03:20 Asia/Jakarta
+ jitter maksimum 30 menit. Timer bawaan paket dinonaktifkan agar satu updater.

Paket distro Glances tidak menyertakan static Web UI dan gagal memulai web.
Solusi: venv `/opt/beon-monitor`, `glances[web]==4.5.8`; service
`beon-glances.service` sebagai user glances, loopback `127.0.0.1:61208`,
interval 5 detik, cap memory 192 MiB. UI HTTP 200; API network terbaca;
sekitar 68 MiB memory pada awal operasi. Service glances bawaan dinonaktifkan.

Akses monitoring melalui SSH tunnel:

```sh
ssh -i /path/private-key.pem -L 61208:127.0.0.1:61208 ubuntu@VPS_IP
# Buka http://127.0.0.1:61208
```

## Temuan data nyata dan perbaikan

Release awal v1.1.0-rc.1 tervalidasi SHA256 dan binary ARM native dapat berjalan.
Namun gate pilot menemukan masalah yang tidak tercakup fixture sebelumnya:

1. FireHOL Anonymous berisi 2,874,739 baris (~41 MB), menyebabkan OOM pada cap
   ingestor 512 MiB dan 1 GiB, termasuk dengan concurrency=1 dan soft budget Go.
   Nonaktifkan feed ini hanya pada konfigurasi pilot, pertahankan konfigurasi
   release asli sebagai `configs/feeds.release.yaml`. Tambahkan phase 6:
   streaming/bounded-memory parser + snapshot database, uji dataset jutaan baris,
   peak RSS serta compiler; aktifkan kembali hanya setelah bukti muat atau upgrade RAM.
2. eDROP dan SSL blacklist mengembalikan daftar kosong; bootstrap strict menolak.
   Pilot menghapus source eDROP dari grup Spamhaus dan menonaktifkan abuse_sslbl.
   Jangan menganggap coverage keduanya tersedia. Source DROP tetap aktif.
3. Ingest dengan 22 kelompok feed berhasil: 394,264 parsed entries, 263,351 rows
   tersimpan, ~24.3 detik. Angka berubah mengikuti snapshot upstream.
4. Compiler menolak 19 jaringan reserved termasuk `0.0.0.0/32`; whitelist lokal
   juga memerlukan reserved networks. Perbaikan compiler mengaktifkan support
   reserved networks; uji regresi meliputi bogon, IPv4 lokal dan IPv6 loopback,
   serta memastikan whitelist mengoverride threat.
5. Soft budget GC per proses sekarang dapat dikonfigurasi melalui Compose:
   API 128MiB, ingestor 700MiB, compiler 1100MiB; sesuaikan di bawah cap container.
   Soft budget bukan hard limit dan tidak menjamin feed jutaan baris muat.

## Status yang masih harus dibuktikan

Deploy release berisi perbaikan compiler; validasi MMDB, API/HTTPS/auth/batch,
route privat, restart pipeline, backup/restore data nyata, dan baseline resource.
Offsite backup destination/credential dan kanal alert belum ditentukan.
Reboot seluruh VPS belum dilakukan karena juga melayani workload testing existing.
Pilot tidak boleh ditandai production-ready sebelum gate phase 5–6 selesai.
