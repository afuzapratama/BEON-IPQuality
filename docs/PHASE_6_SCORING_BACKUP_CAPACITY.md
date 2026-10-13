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
| Baseline pilot awal, sebelum upgrade | 2 | 4 GB | 30 GB |
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

## Kandidat compiler hemat retained aggregation — 13 Oktober 2026

Aggregation diubah menjadi stable-sort indeks + pemrosesan per-prefix group.
Output sama dengan reference lama; input tidak dimutasi. Go tests/vet, race pada
compiler/MMDB/scoring, integrasi PostgreSQL/Redis race dan 10 Python tests lulus.
Kandidat ARM dijalankan hanya di staging; produksi masih rc.3.

Pada 1.209.548 rows, kandidat melewati aggregation dan memasuki writer dengan
1.147.296 entries, lalu OOM pada cap compiler 1,5 GiB (exit 137, OOMKilled=true,
62,82 detik). Ini OOM container yang nyata, bukan guard kill seperti run awal.
Tidak ada MMDB kandidat selesai terverifikasi. Produksi tetap healthy/fresh;
container/volume/network staging dibersihkan.

Resource gate belum lolos. Rekomendasi untuk tahap dataset gabungan: 4 vCPU/8 GB
RAM; disk existing masih cukup untuk pengujian, 80 GB target headroom produksi.
Sesudah resize RAM, uji dengan cap compiler yang juga dinaikkan (awal 2,5 GiB
merupakan estimasi, bukan limit final terbukti). Source akun tetap di luar produksi.
Lihat [evidence kandidat](evidence/phase6-compiler-grouped-arm-2026-10-13.json).

## Upgrade VPS dan temporary NVMe — 13 Oktober 2026

Upgrade terverifikasi: ARM64 4 vCPU, RAM ~7,7 GiB; root disk tetap 30 GB.
NVMe tambahan 220 GB merupakan Azure temporary local disk, bukan managed
storage persisten. Dipasang ext4 pada `/mnt/beon-scratch` (~216 GiB usable),
dengan fstab nofail dan nodev/nosuid/noexec. Baca/tulis dan fstab lulus.
Staging memakai mount guard dan PostgreSQL bind data pada root scratch; data
produksi/backup tidak dipindahkan. Sebelas Python unit tests lulus.

Setelah reboot upgrade, proxy yang sempat berhenti sudah dijalankan kembali.
Pada post-check setup disk, snapshot lama sempat stale; reload admin memuat
MMDB baru dari compiler dan health kembali healthy/fresh. Boot/mount penuh
masih perlu drill; limit compiler produksi tetap 1,5 GiB.
Lihat [panduan scratch storage](VPS_SCRATCH_STORAGE.md).

## Gate dataset gabungan pada VPS 8 GB — 13 Oktober 2026

Uji ulang menggunakan scratch NVMe, compiler kandidat grouped aggregation,
cap 2,5 GiB dan Go target 1.800 MiB. Baseline 210.740 rows → 1.210.128 gabungan.
Ingest 68,60 detik/~587 MiB peak proses; compiler **53,75 detik/~1,94 GiB peak
proses**, exit 0 tanpa OOM/guard abort. MMDB ~57,8 MiB reputasi/City/ASN valid.

API staging (192 MiB, loopback, key sementara) lulus health fresh, 100 sampel
proxy, batch, IPv6, whitelist dan pemisahan auth/admin. Dua dari 100 sampel risky
(score range 32–65). Ini self-check integrasi, bukan angka akurasi independen.
Gate resource/API integration **pada run ini** lolos; sustained/SLO belum terbukti.
Produksi tetap rc.3 dan limit compiler 1,5 GiB, source akun belum diaktifkan.

Container/network/upload staging dibersihkan, bind data/artifact scratch privat
masih tersedia untuk audit. Post-check live healthy/fresh dan alert checker sukses.
Sebelas Python tests lulus. Lihat
[evidence 8 GB](evidence/phase6-avastel-8gb-arm-pass-2026-10-13.json).
Sisa gate: izin penggunaan hasil lookup dataset akun, release compiler immutable,
holdout independen, sustained/public TLS workload, reboot/rollback/external uptime.

## Produksi privat rc.4 dan penutupan drill — 13 Oktober 2026

Operator meminta dataset akun langsung dipakai pada API privat; integrasi sudah
aktif dengan update 7 jam, recovery job setelah boot, source HTTP bridge-only,
key privat, atomic normalized refresh dan compiler lock lintas proses.
Runtime rc.4 CI/release semua sukses; limit compiler 2,5 GiB/Go 1.800 MiB.

Public HTTPS probe 5 menit/5 req/s: 1.500 HTTP 200, zero error; p95 ~311 ms.
Live rollback rc4→rc3→rc4 19 detik dengan 502 sementara; warm reboot public healthy
28,67 detik, layanan/timer/scratch pulih otomatis. Backup rc4 termasuk key akun
lulus isolated restore + encrypted Bunny readback. Monitor GitHub eksternal
aktif (~5 menit) dengan tes Telegram incident/recovery sukses.

Operator belum punya korpus independen dan memilih alur review: validator,
group split dan runbook sudah disiapkan; angka accuracy belum tersedia.
Dokumen detail: [produksi akun](AVASTEL_PRIVATE_PRODUCTION.md),
[drill/limits](OPERATIONS_RC4_VALIDATION.md), [review](LABEL_REVIEW_WORKFLOW.md).
Eksperimen sebelumnya yang menyatakan akun staging/OOM adalah catatan historis;
status terbaru mengikuti bagian ini.

### Penyesuaian jadwal MaxMind — 13 Oktober 2026

Timer GeoIP sebelumnya harian 03:20 WIB diganti Rabu/Sabtu 12:00 WIB + jitter maksimal 30 menit. City mengikuti rilis Selasa/Jumat; ASN yang rilis harian sengaja ikut jadwal dua kali seminggu. Jam rilis upstream tidak dijamin, sehingga jeda ini bukan SLA ketersediaan rilis. Output tetap pada direktori MMDB proyek.

### Review label dan persiapan merge — 13 Oktober 2026

Cohort diagnostik 258 IP sudah disimpan privat dengan lembar review terpisah dari
prediksi: 250 account-feed samples + delapan DNS IPv4/IPv6. Seluruh label masih
unreviewed; jumlah approved independen nol. Prediksi proxy/VPN/datacenter/risky
bukan ground truth, sehingga precision/recall dan tuning bobot belum dilakukan.
Alur collect-after-freeze tersedia melalui `collect-reviewed-predictions.py`.
Lihat [workflow review](LABEL_REVIEW_WORKFLOW.md) dan [aggregate cohort](evidence/phase6-label-review-cohort-2026-10-13.json).

Email CI gagal 13 Oktober berasal dari run 38313090465: download dependency Go
mengalami upstream HTTP/2 INTERNAL_ERROR. Run berikutnya 38323416441 lulus semua
job. Dockerfile sekarang retry download maksimal tiga kali dan tetap gagal jika
seluruh attempt gagal. CI operations juga menjalankan test scoring/review; test
lokal saat perubahan ini: 15 scoring/review + sembilan operations lulus.

Pemulihan host terpisah tersedia melalui [runbook](DISASTER_RECOVERY_DRILL.md).
Restore drill memeriksa checksum, ledger migrasi, refresh feed reguler, compile dan
verify MMDB, health/auth/IPv6/whitelist serta account lookup. Ini tidak mengukur
provisioning OS atau pemulihan DNS/TLS dari kehilangan VPS.

Hasil drill 13 Oktober 2026: backup Bunny `20261012T234730Z-20383`, restore 1.287.574 rows, migrasi/refresh/compile/verify lulus, API healthy dan 25 account samples, auth/IPv6/whitelist lulus. Durasi 129,334 detik setelah download/decrypt/extract; cleanup project uji lulus. [Evidence](evidence/phase6-bunny-disaster-restore-2026-10-13.json).

### Monitoring produksi dan audit endpoint — 13 Oktober 2026

Prometheus/Grafana/node exporter diaktifkan melalui Compose terpisah, image
multiarch digest-pinned. Grafana di `/grafana/` memakai login; `/metrics` publik
403, scrape API/pipeline/server melalui jaringan internal. Node exporter memakai
host network/PID dan bind hanya gateway Docker privat agar trafik yang diukur
adalah trafik VPS. Dashboard memiliki 16 panel. Prometheus retention 7 hari /
target 2 GB plus headroom WAL, memory cap 512 MiB; Grafana 512 MiB/exporter 64 MiB.
Credential monitoring ikut backup terenkripsi; histori TSDB/user preferences
Grafana belum termasuk backup aplikasi. [Runbook](PRODUCTION_MONITORING.md).

Audit API authenticated membuktikan semua enam route lookup/admin aktif dan 200.
README diringkas sesuai produksi, tanpa catatan deployment historis yang tumpang
tindih. Caddy dan template Nginx menyediakan route API yang sama; GET/POST/DELETE
dan key role tetap dibedakan.

Aktivasi scrape menemukan bug label method Fiber: buffer request yang dipakai ulang
mengubah POST menjadi GETT pada label Prometheus dan menyebabkan gather 500.
Label disalin sebelum disimpan; regression RequestCtx reuse gagal pada kode lama,
lulus pada perbaikan. Runtime perbaikan diterbitkan sebagai `v1.1.0-rc.6`.
Konfigurasi host mengikuti revision support terkini, terpisah dari digest runtime.
Source `main`, runtime release dan status stable adalah hal yang berbeda.
Akurasi independen, sustained-load/disk growth dan RTO provisioning/DNS/TLS masih
terbuka; tidak ada stable tag yang diterbitkan dengan klaim gate tersebut selesai.

[Evidence monitoring rc.6](evidence/production-monitoring-2026-10-13.json): lima target UP, 16 query Grafana valid, login dan isolasi metrics lulus. Seluruh enam route API/admin kembali diuji 200. Internal metrics tetap 200 setelah campuran method yang sebelumnya merusak label. Rollout final healthy/fresh, 9,475 detik; PG/Redis digest dipertahankan. Percobaan rollout pertama gagal health gate akibat source stale dan rollback; snapshot disegarkan sebelum retry, bukan dengan melonggarkan freshness budget.

### Insiden pipeline 14 Oktober 2026

Compiler menolak IPv6 6to4 yang berada dalam alias IPv4 MMDB; satu record membuat
seluruh publikasi gagal dan snapshot lama melewati freshness budget. Writer produksi
kini menonaktifkan alias otomatis agar record IPv4/IPv6 eksplisit tetap independen.
Regresi mencakup IPv4, 6to4 dan Teredo serta factory writer produksi.

Updater timeout setelah 600 detik meninggalkan container/transaction tetap berjalan.
Query expiry source besar memakai staging tanpa index/statistik. Staging sekarang
memiliki index expression inet dan ANALYZE. Worker publisher dinamai, timeout ingest
30 menit/compile 15 menit dan container miliknya dihapus saat timeout; unit mempunyai
budget keseluruhan 60 menit. Freshness budget tetap sama.

GreenSnow sedang HTTP 503 dan dikarantina dari profil ARM produksi: sinyalnya tidak
ikut compile, bukan dianggap fresh. Aktifkan kembali setelah upstream sehat dan
refresh sukses. Jangan memakai retry manual saja sebagai bukti stabilitas 24–48 jam.

PostgreSQL pada VPS 8 GB masih memakai cap pilot 512 MiB; cap dinaikkan ke 1.536 MiB tanpa restart DB untuk memberi headroom cache pada update massal. Worker timeout juga menghentikan koneksi PostgreSQL yang berasal dari IP container miliknya sebelum container dihapus; close Docker CLI/container saja tidak menjamin query server berhenti. Runtime rc.7 dan host support revision dipantau terpisah.

Percobaan index staging rc.7 masih memerlukan 17 menit untuk update account penuh
pada disk produksi. Expiry kemudian diganti dengan marker generasi: timestamp DB
diambil setelah advisory lock, seluruh row present mendapat marker itu, dan row
source yang tidak mendapat marker di-expire dalam transaksi yang sama. Tidak ada
anti-join ke staging lagi; source lain tidak diubah. Capture setelah lock menghindari
transaction-NOW yang lebih tua daripada publisher yang selesai sebelumnya.

Validasi akhir rc.8: API HTTPS 200/fresh, updater systemd exit 0 (142,367 detik lengkap), compile 34,252 detik, 5/5 target scrape UP, checker alert failures=0, tanpa worker orphan. [Evidence](evidence/pipeline-recovery-2026-10-14.json). Benchmark ulang memakai snapshot/keadaan cache berbeda, sehingga bukan klaim rasio speedup terkontrol atau penutupan gate 24–48 jam.
