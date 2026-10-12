# Phase 6 — scoring, backup offsite, alert dan kapasitas

## Kebijakan scoring

Policy `evidence-family-max-v1` adalah heuristic operasional, bukan probabilitas
kejahatan dan belum memiliki angka akurasi/false-positive yang terkalibrasi.
Gunakan flag proxy/VPN/Tor untuk jenis sinyal, bukan menganggap `risky` berarti proxy.

- Infrastructure (`datacenter`, `hosting`, konteks ASN) maksimum 20.
- Anonymity (`proxy`, `vpn`, `tor`, `anonymous`) mengambil kontribusi terkuat.
- Abuse (C2, malware, hijacked, attack, spam, suspicious) mengambil kontribusi terkuat.
- Kontribusi: bobot source × confidence (maksimum 1) × decay; duplicate source,
  daftar agregator berkorelasi dan overlap CIDR tidak dijumlahkan dalam family sama.
- Family berbeda digabung secara bounded complement product; bukan asumsi
  independensi statistik dan bukan probability. Max score konfigurasi tetap berlaku.
- ASN bergabung sebagai konteks maksimum 20; tidak lagi bonus berulang/multiplier.
  ASN pendidikan/pemerintah tidak mengurangi bukti C2/malware.
- Compiler menyimpan evidence dalam MMDB; merger CIDR mengambil maksimum setiap
  family dan menghitung ulang sekali. Legacy record tanpa evidence memakai score
  terkuat, tidak menjumlahkan score yang kehilangan provenance.
- Whitelist tetap mengoverride semua sinyal. Disabled/removed source difilter
  compiler meskipun DB TTL belum expired.

Uji mencakup pengulangan sinyal 100 kali, datacenter-only, C2, decay, cap score,
CIDR overlap dua urutan insertion, whitelist serta pipeline PostgreSQL/Redis nyata.
Korpus label independen untuk precision/recall diperlukan sebelum klaim akurasi;
Google/Cloudflare DNS adalah sanity check infrastruktur, bukan corpus keseluruhan.

## Backup dan Bunny S3

Pipeline harian: pg_dump + MMDB + config + manifest + credential → checksum →
restore PostgreSQL/MMDB terisolasi → VERIFIED → age encryption → upload Bunny →
GET seluruh object/checksum → OFFSITE_VERIFIED → retention.

Kunci age privat berada di komputer pengguna di
`/home/natama/Documents/sshkey/ipproxy-backup.agekey` mode 0600. VPS menyimpan
public recipient `/etc/beon-ipquality/backup.recipient`; kunci privat harus
memiliki salinan aman terpisah dari VPS untuk pemulihan bencana.

Template secret: `configs/operations.env.example`; konfigurasi nyata
`/etc/beon-ipquality/operations.env` mode 0600, tidak masuk repository.
Bunny Access Key ID adalah nama Storage Zone; secret adalah password zone,
endpoint regional `https://<region>-s3.storage.bunnycdn.com`. S3 Compatibility
harus aktif ketika zone dibuat (public preview). Bucket backup tidak memerlukan
Pull Zone/CDN publik. Referensi: https://bunny.net/docs/storage/s3.

Retain lokal minimum 7 hari dan 7 bundle VERIFIED terbaru; pruning ditunda jika
backup terbaru belum offsite-verified. Retain offsite minimum 30 hari; hanya object
backup dalam prefix proyek dan nama terkontrol dipruning setelah readback terbaru
lulus. DeleteObjects batch Bunny tidak didukung, gunakan DeleteObject satu per satu.
Incomplete/unverified bundle tidak dipruning otomatis. Object berisi cipher age,
checksum ciphertext terpisah; recovery memerlukan private key age.

Download terautentikasi:

```sh
# Jalankan dengan environment Bunny privat, tidak dengan secret di argv.
python3 scripts/operations.py download \
  --key beon-ipquality/YYYYMMDDTHHMMSSZ-NNN.tar.age --output /private/backup.tar.age
age -d -i /private/ipproxy-backup.agekey -o /private/backup.tar /private/backup.tar.age
# Ekstrak ke direktori privat, lalu jalankan restore-check.sh pada bundle.
```

RPO target backup DB harian sekitar 24 jam, ditambah jitter maksimum 15 menit; schedule bukan bukti sukses.
Konfigurasi operations Bunny/Telegram dan public recipient juga masuk bundle privat.
Private key age sengaja disimpan di komputer pengguna dan tidak masuk bundle.
Restore dataset membutuhkan refresh feed/recompile sebelum API dinyatakan fresh.
RTO final bergantung ukuran DB, download offsite dan refresh upstream, harus diukur.

## Telegram dan monitor

User memilih Telegram dan mengizinkan pengiriman alert/pesan tes ke chat tujuan.
Dibutuhkan TELEGRAM_BOT_TOKEN dan TELEGRAM_CHAT_ID; /start bot atau tambahkan bot
ke grup. Topic opsional memakai TELEGRAM_MESSAGE_THREAD_ID.

Timer setiap menit memeriksa health/snapshot/feed freshness, container down/OOM,
disk >=80%/<2 GiB, RAM available <10%, backup/offsite lebih tua dari 36 jam,
dan hasil service backup/GeoIP. Alert setelah dua pemeriksaan gagal berturut-turut;
kirim recovery dan reminder enam jam. Token dan URL Telegram tidak dicetak.
Referensi: https://core.telegram.org/bots/api#sendmessage.

Monitor di VPS tidak dapat mengirim alert ketika seluruh VPS/network mati.
Monitoring eksternal dari host lain tetap diperlukan untuk mendeteksi kejadian itu.
Glances tetap untuk inspeksi CPU/RAM/network/disk I/O melalui SSH tunnel.

## Rekomendasi awal hardware

| Pemakaian | vCPU ARM64 | RAM | SSD/NVMe |
|---|---:|---:|---:|
| Pilot terkurasi saat ini | 2 | 4 GB | 30 GB |
| Target awal satu node API + PostgreSQL + pipeline | 4 | 8 GB | 80 GB |
| Feed jutaan baris / pertumbuhan dan traffic lebih besar | 8 | 16 GB | 160 GB |

Baris kedua/ketiga merupakan rekomendasi engineering dengan headroom, bukan hasil
benchmark host tersebut. Pilot lama peak ingest ~63 MiB, compiler ~305 MiB;
FireHOL Anonymous 2.87 juta baris OOM pada cap ingest 1 GiB. Jangan mengaktifkan
feed tersebut hanya karena menaikkan memory API. Ukur ingest/compiler dataset
penuh serta disk DB/retention sebelum menentukan ukuran final.

Capacity test harus memakai API terisolasi tanpa public host port, snapshot/data
nyata, dan benchmark limiter terpisah agar 429 tidak disalahartikan sebagai batas
hardware. Catat latency p50/p95/p99, error, throughput dan overhead load generator;
benchmark dari VPS yang sama bukan pengukuran latency internet/publik produksi.

## Hasil eksekusi 12 Oktober 2026

Runtime `v1.1.0-rc.3`, source `1d121398f533a02f6313ba2a896a44851a04e00e`.
Seluruh CI/release sukses: https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38262970419.
Manifest image/digest: `deployments/releases/v1.1.0-rc.3.env`.
Host support scripts dilanjutkan pada revision `a1422f2c2ba0b580e2994dcfb518451b2d13ed74`
untuk freshness berdasarkan waktu pembuatan backup, counter pipeline error, dan
backup credentials operations/recipient. Ini tidak mengubah digest runtime Go.

API live setelah deploy lulus health, auth salah/benar, IPv4/IPv6, batch,
whitelist publik, private-IP rejection, admin isolation dan reload tanpa perubahan.
Health snapshot mengungkap policy `evidence-family-max-v1`. Google DNS 8.8.8.8
berubah dari score 81 menjadi 20, risky=false, datacenter=true, proxy/VPN=false.
Tiga IP C2 dari feed aktif masing-masing score 92, risky=true, botnet=true.
Ini sanity/provenance check, tidak membuktikan angka precision/recall keseluruhan.

### Baseline loopback terisolasi

API benchmark menggunakan reader rc.2 yang kompatibel dengan MMDB candidate
policy baru; perubahan API rc.3 hanya menambahkan identitas policy pada health.
Load generator berada pada VPS yang sama, single IP berulang dan batch empat IP,
auth tetap aktif, limiter dimatikan hanya pada container benchmark loopback 18080.
Delapan tahap masing-masing lima detik; semua HTTP 200, nol error. Semua container
uji dihapus. Angka ini bukan kapasitas produksi publik/sustained, dan mengandung
overhead client/logging/monitor serta workload existing.

| Redis | Jenis | Concurrency | HTTP req/s | p95 ms | p99 ms |
|---|---|---:|---:|---:|---:|
| aktif | single | 1 | 2223 | 0.61 | 0.88 |
| aktif | single | 8 | 4115 | 4.25 | 6.22 |
| aktif | single | 32 | 5039 | 14.92 | 21.73 |
| aktif | batch 4 IP | 8 | 2392 | 6.30 | 8.81 |
| off | single | 1 | 2826 | 0.46 | 0.60 |
| off | single | 8 | 4803 | 4.15 | 7.86 |
| off | single | 32 | 6554 | 12.03 | 17.71 |
| off | batch 4 IP | 8 | 4194 | 4.65 | 7.13 |

Peak compiler candidate 515,473,408 byte (~492 MiB), dibanding baseline lama
~305 MiB; evidence meningkatkan kebutuhan heap. Scope dataset tetap terkurasi,
FireHOL Anonymous 2.87 juta baris belum aktif/teruji.

Pilot memakai `REDIS_ENABLED=false` berdasarkan hasil single/batch ini. Redis
container tetap tersedia; workload lain dan variasi IP lebih luas perlu diuji
sebelum generalisasi keputusan. Quota publik tetap 1,000 request/menit/key;
benchmark tanpa limiter tidak mengubah quota tersebut. Target 100k req/s belum
terbukti; perlu hardware/topologi dan pengujian berbeda.

### Bukti backup dan alert

- Bunny endpoint regional DE dan Storage Zone credential berhasil PUT/GET,
  readback seluruh object serta SHA256 cocok.
- Bundle `20261012T142058Z-30728` diunduh dari Bunny, didekripsi dengan age,
  dan PostgreSQL + MMDB/GeoIP/ASN dipulihkan terisolasi: 269,087 rows dan ledger
  migrations 001/002. Target uji dibersihkan, DB live tidak diganti.
- Backup sesudah rc.3 `20261012T144016Z-3155` serta bundle lengkap credentials
  operations `20261012T145106Z-29316` juga lulus isolated restore + encrypted
  upload/readback. Cycle terakhir ~28 detik, native job peak ~147 MiB.
- Kunci pemulihan telah disalin/diuji, mode 0600 di komputer pengguna; private key
  di VPS dihapus setelah drill. VPS menyimpan public recipient saja.
- Telegram test, simulated incident bertanda [TEST], recovery, serta laporan
  backup mendapat acknowledgement sukses dari Bot API.
- Native alert checker aktif setiap menit; baseline ~40 MiB peak per check dan
  <1 detik CPU/check sebelum metric scrapes. Scrape pipeline tetap internal lewat
  docker exec, tanpa membuka host/public port. Error dipertahankan hingga success
  berikutnya; container restart/reset counter tidak dianggap error historis baru.
- Backup harian 02:30 WIB + maksimum 15 menit jitter; GeoIP 03:20 WIB + maksimum
  30 menit jitter. Retention minimum tujuh hari/tujuh recovery points lokal dan
  30 hari offsite; belum ada object tua sehingga pruning nyata belum terjadi.
  Guard pruning, corruption, token redaction, stale backup dan counter recovery
  mendapat tujuh unit tests Python; Go race, integration dan ops CI juga lulus.

### Sisa gate produksi

Korpus label independen/precision-recall, sustained/public TLS load dan varied-IP
workload, optimasi parser/compiler dataset jutaan baris, external uptime monitor
untuk VPS/network mati total, serta reboot/full rollback drill masih diperlukan.
Jangan menandai seluruh phase 6 selesai hanya berdasarkan baseline singkat ini.

## Akses langsung HTTPS — 13 Oktober 2026

Atas permintaan operator, proxy Caddy kini menyajikan halaman lookup pada `/`
dan meneruskan `/health`, `/metrics`, serta `/api/v1/*` melalui domain publik.
Health/metrics tidak membutuhkan key. Lookup dan stats memerlukan API key;
cache/reload tetap membutuhkan admin key terpisah. Form browser mengirim key
melalui header dan tidak menyimpannya. Tidak ada perubahan digest runtime Go.

Validasi Caddy lulus sebelum reload SIGUSR1; konfigurasi sebelumnya dibackup
sebagai `/opt/shortlink-agent/Caddyfile.before-direct-access`. Root, health,
metrics, lookup, batch, dan stats dengan key lulus HTTP 200. Lookup tanpa key
serta cache/reload memakai key klien mendapat 401. Endpoint lama yang dicatat
sebagai internal-only pada laporan historis kini mengikuti akses di bagian ini.

## Persiapan evaluasi akurasi — 13 Oktober 2026

Evaluator offline tersedia pada `scripts/evaluate-scoring.py`, dengan
[panduan korpus dan provenance](SCORING_EVALUATION.md). Alat melaporkan confusion
matrix, precision/recall/F1 dan false-positive rate per kategori; input dengan
coverage tidak lengkap atau snapshot campuran ditolak. Tiga unit tests lulus,
termasuk denominator nol dan whitelist pada eksperimen threshold. Korpus label
independen nyata belum tersedia; gate akurasi belum ditutup dan scoring VPS
belum diubah dari hasil fixture.

## Baseline proxy eksternal Avastel — 13 Oktober 2026

Studi read-only menggunakan 500 sampel acak dari 500.000 IP provider-reported
proxy, source dipin ke commit dan hash, hasil API satu data_version. Flag proxy
0/500, VPN 5, datacenter 20, risky 11. Ini positive-only provider agreement;
precision/false-positive rate dan akurasi keseluruhan tidak dapat disimpulkan.
Independensi upstream transitif belum sepenuhnya terbukti. Raw artifacts privat
tersimpan lokal; evidence agregat ada di
[evidence Avastel](evidence/phase6-avastel-positive-agreement-2026-10-13.json).

Avastel tetap di luar feed produksi untuk menjaga baseline. Sebelum integrasi,
perlukan negative controls, holdout berbeda, parser semicolon dan pengukuran
staging ingest/compiler 500k baris. Panduan lengkap pada
[SCORING_EVALUATION.md](SCORING_EVALUATION.md). Lima unit tests evaluasi/parsing
lulus; runtime dan scoring VPS tidak diubah oleh studi ini.

## Staging dataset akun Avastel — 13 Oktober 2026

Key akun dari file privat digunakan untuk download ZIP CSV, tanpa dicetak atau
masuk repository. Dataset 1.000.000 baris valid; 999.388 isProxy=True dinormalisasi,
612 isProxy=False dikecualikan. Go ingestor/compiler berhasil pada PostgreSQL
sementara lokal: ingest 26,23 detik/~616 MiB peak; compiler 25,37 detik/~1,44 GiB
peak; MMDB ~49 MiB terverifikasi. API staging 100/100 record teringest memberi
proxy=true, score 32, risky=false; ini self-check integrasi, bukan accuracy.

Pengukuran x86_64, source tunggal tanpa GeoLite2/pilot feeds; tidak membuktikan
kapasitas ARM atau dataset gabungan. Peak compiler sudah dekat cap VPS 1,5 GiB.
Dataset akun tetap privat staging: Free plan tidak mencakup redistribusi,
penggunaan lookup turunan perlu dipastikan. Source Github MIT tetap terpisah.
Tidak ada konfigurasi API produksi atau timer akun yang diubah.

Delapan unit tests evaluator/parser lulus. Raw artifacts privat dipertahankan,
container PostgreSQL/volume staging dan server loopback sudah dibersihkan.
Lihat [panduan](SCORING_EVALUATION.md) dan
[evidence agregat](evidence/phase6-avastel-account-staging-2026-10-13.json).

## Gate resource dataset gabungan ARM — 13 Oktober 2026

Clone DB pilot + GeoLite2 + source akun diuji dengan rc.3 ARM64 pada network/DB/
MMDB staging terpisah. Rows aktif 210.160 → 1.209.548. Ingest berhasil 71,47 detik,
peak proses teramati ~648 MiB. Compiler **belum selesai**: guard host pressure
menghentikannya setelah 15,80 detik, peak proses ~1,49 GiB dekat cap 1,5 GiB.
Exit 137 merupakan guard kill; OOMKilled=false. Tidak ada MMDB kandidat selesai.

API live healthy, snapshot fresh, alert checker sukses sesudah uji. Seluruh
container/volume/network staging dibersihkan. Source akun tidak diaktifkan pada
produksi. Gate resource tetap terbuka; prioritas berikutnya mengurangi memory
aggregation lalu mengulang bounded ARM staging. Rekomendasi 8 GB belum merupakan
hasil resize/benchmark. Penggunaan dataset akun untuk API masih perlu diperjelas.

[Evidence gabungan ARM](evidence/phase6-avastel-combined-arm-2026-10-13.json)
dan [detail prosedur/keputusan](SCORING_EVALUATION.md). Sepuluh unit tests
parser/evaluator/guard lulus; tidak ada digest runtime produksi yang diubah.
