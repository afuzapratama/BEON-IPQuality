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

## Hasil pilot akhir

Release `v1.1.0-rc.2`, revision `16c2413f44c76a69b8435f8d5f115c0f6d0d2c74`,
seluruh job release/CI sukses: https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38252514536.
Archive ARM64, images.json dan deployment-lock.env lolos SHA256 di VPS.
Manifest tersimpan di `deployments/releases/v1.1.0-rc.2.env` dan server
`/opt/beon-ipquality/deployment-lock.env`. Empat image berasal dari release
multiarch, dengan pull tanpa login registry. Template backup ditambahkan setelah
revision runtime; unit host tidak mengubah image release.

Bootstrap release baru berhasil: 394,290 parsed entries, 263,454 stored rows,
~28.2 detik; compiler 144,501 jaringan, ~3.1 detik. Pengukuran cgroup memory.peak
selama bootstrap: ingestor-init 66,252,800 byte (~63 MiB), compiler-init
320,028,672 byte (~305 MiB). PostgreSQL pernah mencapai cap ~512 MiB termasuk
page cache; idle sesudah pilot ~172 MiB. Pengukuran ini hanya berlaku untuk
konfigurasi pilot, bukan bukti feed jutaan baris muat.

Lookup nyata mengungkap URL identik dipakai oleh grup `datacenter_ips` dan
`datacenter_extended`, serta `vpn_providers` dan `vpn_extended`. Pilot menonaktifkan
grup duplikat lama, mempertahankan grup extended IPv4/IPv6, dan mengexpire
55,480 row dari dua source duplikat dalam transaksi (history tidak dihapus).
Recompile/reload berhasil. Konfigurasi aktif tersalin tanpa secret sebagai `configs/feeds.pilot.arm-4gb.yaml`.
Pada fresh deployment host setara, salin template ini ke `configs/feeds.yaml`
sebelum bootstrap. Jangan memakai template untuk menonaktifkan source pada DB lama
tanpa expiry/filter eksplisit dan backup terlebih dahulu.

Konfigurasi akhir: 20 kelompok feed, 32 source aktif,
207,974 row aktif saat compile, 144,501 jaringan MMDB. Tidak mengklaim sumber
berkorelasi sudah terdeduplikasi secara menyeluruh. Uji 8.8.8.8 berubah skor 100
menjadi 81 dengan satu sumber datacenter; proxy/VPN/Tor false. Phase 6 wajib
mengevaluasi scoring pada CIDR overlap, korelasi sumber dan false positive;
skor risiko tidak boleh disamakan dengan bukti sebuah IP adalah proxy.

Verifikasi yang lulus:

- Health lokal 200, versi v1.1.0-rc.2, snapshot/source fresh; API container healthy.
- HTTPS domain baru valid. Domain testing lama tetap merespons sesudah recreate
  proxy agar bind mount mengambil inode konfigurasi host yang baru.
- Tanpa key/key salah 401; single IPv4 dan IPv6 200; batch 200; Geo dan ASN terisi.
- Whitelist publik 66.249.64.1 menghasilkan score 0/whitelisted true. Private IP
  dan input invalid mendapat 400 sesuai validasi API; whitelist lokal tetap dapat
  ditulis ke MMDB, tetapi API publik tidak menerima lookup alamat private.
- Cache hit dan data_version konsisten. Reload sesudah recompile changed=true;
  pengulangan tanpa perubahan changed=false. Lookup key tidak dapat memakai admin.
- Health, metrics, reload, dan cache stats mendapat 404 melalui domain publik.
- Redis dihentikan sementara: lookup tetap 200 (~53 ms); Redis dipulihkan.
- Limiter melalui Caddy: 1,100 request terautentikasi, 998 mendapat 200 dan 102
  mendapat 429; health lokal tetap 200. Ini smoke test limiter, bukan benchmark.
- Restart API/ingestor/compiler berhasil; API memerlukan beberapa detik startup
  untuk validasi MMDB, lalu health 200. Container PostgreSQL/Redis tetap healthy.
- Dua backup lengkap lolos checksum dan restore ke PostgreSQL terisolasi, termasuk
  seluruh MMDB/GeoIP/ASN dan migration ledger 001/002; 265,850 total rows termasuk
  history expired terpulihkan. Target/container restore dihapus tanpa mengubah DB live.
- Backup akhir konfigurasi dedup tersimpan di `/opt/beon-ipquality/backups`.
  Timer backup harian 02:30 Asia/Jakarta + jitter 15 menit aktif; bundle mode privat,
  mencakup secret MaxMind. GeoIP timer 03:20 + jitter 30 menit aktif.
- Image API rc.1 dapat memuat snapshot akhir pada container terisolasi tanpa host
  port, health 200 versi rc.1. Container uji dihapus. Ini bukti kompatibilitas rollback
  API; compiler rc.1 tetap tidak aman untuk recompile reserved networks.

API lookup key disimpan privat di komputer pengguna sebagai
`/home/natama/Documents/sshkey/ipproxy-client.env` mode 0600; tidak ada secret di docs.
Admin key hanya pada `.env` privat VPS. Glances UI/API berhasil di loopback;
memakai sekitar 68 MiB pada baseline awal. Native updater dan backup unit enabled.

## Pekerjaan lanjutan phase 5–6

- Reboot seluruh VPS belum diuji; host juga melayani workload testing existing.
- Full rollback pipeline/compiler memerlukan release target kompatibel; rc.1
  hanya berhasil diuji untuk API dengan snapshot valid, bukan untuk recompile.
- Retention backup lokal dan penghapusan bundle lama belum otomatis; saat ini
  bundle lengkap dipertahankan. Finalkan retention bersama backup offsite.
- Tujuan/credential offsite backup dan kanal alert belum ditentukan.
- Uji beban, p95/p99, sustained traffic, peak compiler dataset penuh, snapshot rollback,
  monitoring history, alert nyata, security review dan akurasi skor tetap phase 6.
- Optimasi bounded-memory FireHOL Anonymous dan peninjauan eDROP/SSL blacklist
  diperlukan sebelum perluasan coverage. Penonaktifan feed harus mengexpire/filter
  row lama; compiler saat ini membaca seluruh row aktif DB, tidak otomatis memfilter
  berdasarkan enabled feed. Pilot mengexpire duplikat secara eksplisit.

Pilot berjalan; gate produksi penuh belum selesai.
