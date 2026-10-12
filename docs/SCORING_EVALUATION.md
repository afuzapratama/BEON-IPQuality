# Evaluasi akurasi scoring

Alat `scripts/evaluate-scoring.py` membandingkan label independen dengan respons
API yang telah disimpan, tanpa mengubah scoring produksi atau mengirim request.
Korpus nyata belum tersedia; fixture unit test hanya memeriksa perhitungan alat.

## Korpus dan provenance

Siapkan JSONL privat: satu IP per baris, label boolean untuk kategori yang memang
terverifikasi. Kategori tidak diketahui harus dihilangkan, bukan diberi false.
Contoh berikut hanya ilustrasi format, **bukan label nyata**:

```json
{"ip":"8.8.8.8","labels":{"risky":false},"label_source":"referensi independen yang telah ditinjau","observed_at":"2026-10-13T00:00:00Z","independent":true}
```

Label wajib memiliki sumber dan waktu pengamatan. Operator meninjau bahwa sumber
label tidak berasal dari feed penyusun MMDB atau hasil API sendiri sebelum
menetapkan `independent=true`; alat tidak dapat membuktikan independensi sumber.
Ketiadaan IP dalam blacklist tidak cukup untuk memberi label bersih.

Pisahkan label risky/abuse dari proxy, VPN, Tor, dan datacenter. Proxy/VPN tidak
otomatis membuktikan aktivitas berbahaya. Sertakan positive dan negative per
kategori, IPv4/IPv6, jaringan residensial/infrastruktur, serta sampel per ASN.
Pisahkan dataset tuning dan holdout evaluasi; IP terkait dalam subnet/ASN/sumber
harus dikelompokkan agar tidak bocor antarsplit. Catat komposisi sampel dan umur
label; hasil korpus yang dipilih bukan estimasi prevalensi internet keseluruhan.

## Prediksi dan laporan

Simpan setiap respons JSON lookup single sebagai satu baris pada predictions.jsonl.
API key dikirim pada header `X-API-Key`, bukan URL. Gunakan satu snapshot dengan
`data_version` sama untuk seluruh korpus; hentikan pengumpulan saat terjadi
401/429/503 atau perubahan versi, lalu ulangi pada snapshot konsisten.
Jangan menghapus sampel gagal untuk menaikkan hasil akurasi.

```bash
python3 scripts/evaluate-scoring.py \
  --labels /private/holdout-labels.jsonl \
  --predictions /private/holdout-predictions.jsonl > /private/report.json

# Eksperimen threshold offline; tidak mengubah konfigurasi server.
python3 scripts/evaluate-scoring.py \
  --labels /private/tuning-labels.jsonl \
  --predictions /private/tuning-predictions.jsonl --threshold 70

python3 -m unittest discover -s tests/scoring -v
```

Alat menolak IP duplikat, coverage tidak lengkap, versi prediksi campuran,
provenance kosong, serta label/flag yang bukan boolean. IPv4-mapped IPv6
dinormalisasi sebelum pencocokan. Laporan memuat TP/FP/TN/FN, jumlah label,
precision, recall, false-positive rate, F1, dan daftar mismatch per kategori.
Denominator nol menghasilkan null, bukan klaim akurasi 100%. Override threshold
hanya memengaruhi risky dan tetap menghormati whitelist.

Review mismatch dan provenance sebelum mengubah bobot/threshold. Catat ukuran
sampel dan ketidakpastian statistik pada laporan hasil nyata; alat ini belum
menghitung confidence interval. Gate akurasi phase 6 masih terbuka sampai korpus
independen serta evaluasi holdout selesai dan disepakati target tiap kategori.

## Baseline Avastel: pembanding eksternal kategori proxy

`scripts/compare-avastel.py` mengambil daftar satu hari dari commit upstream yang
immutable, menyimpan hash/timestamp, lalu memilih sampel acak deterministik.
Format upstream adalah CSV bertanda titik koma dengan kolom `ip_address` dan
`autonomous_system`; jangan memasangnya sebagai feed plain tanpa parser sesuai.
Script menolak source berumur lebih dari 36 jam, respons terlalu besar, IP
non-global, error HTTP, batch tidak lengkap, dan perubahan versi snapshot API.
Kredensial dibaca sebagai data dari file, bukan dieksekusi sebagai shell.

```bash
python3 scripts/compare-avastel.py \
  --client-env /private/ipproxy-client.env \
  --output /private/scoring-avastel-run-new --samples 500
```

Output harus direktori baru. Semua file dibuat privat dengan umask 077. Source,
sampel dan respons API disimpan untuk audit; tidak ada API key di output. Source
atau manifest dari run gagal tidak boleh dianggap laporan pengujian selesai.
Permintaan memakai batch 50 IP agar studi kecil tidak membebani API. Tidak ada
perubahan feed, database, threshold, whitelist, atau konfigurasi server.

### Hasil 13 Oktober 2026 (WIB)

Pada snapshot upstream `784596a3d415414a8c01b088ddcf524f0118ca2d`, berisi
500.000 IP unik, sampel acak 500 IP IPv4 seluruhnya selesai pada satu data_version.
Hasil: **0/500 mendapat flag proxy**, 5 VPN, 20 datacenter, dan 11 risky. Jumlah
flag dapat overlap. Ini menunjukkan gap agreement dengan label proxy penyedia,
bukan bukti bahwa setiap IP pasti masih proxy sekarang atau scoring keseluruhan
memiliki akurasi 0%. Tidak ada sampel negatif terverifikasi pada studi ini.
Precision dan false-positive rate tidak dapat dihitung.

[Evidence agregat](evidence/phase6-avastel-positive-agreement-2026-10-13.json)
memuat source hash, seed, versi MMDB, distribusi flag dan keterbatasan. Raw source,
sampel dan predictions disimpan privat di komputer operator pada
`/home/natama/Documents/beon-backups/scoring-avastel-20261013-run2`.

Review awal header FireHOL level 1/2 tidak menyebut Avastel sebagai constituent;
IPsum mengagregasi banyak source sehingga independensi transitif belum terbukti.
IP overlap tidak dengan sendirinya membuktikan bahwa suatu feed menyalin Avastel.
Karena itu studi dicatat sebagai agreement dengan penyedia eksternal, bukan
holdout berlabel independen yang telah tersertifikasi. README upstream menyatakan
observasi proxy berasal dari sistem deteksinya sendiri dan IP dapat dipakai bersama:
[Avastel README](https://github.com/antoinevastel/avastel-bot-ips-lists#readme).

### Urutan penerapan berikutnya

1. Tambahkan negative controls yang terverifikasi dan cohort dari penyedia/observasi
   berbeda, lalu tinjau umur label, subnet/ASN, serta dependensi source.
2. Persiapkan parser semicolon dan ingest 500k baris pada staging terisolasi;
   ukur memory, durasi ingest/compiler, disk dan provenance. Jangan mengaktifkan
   dataset besar langsung pada worker pilot 4 GB yang pernah mengalami OOM.
3. Bila integrasi layak, klasifikasikan sebagai proxy/anonymity; jangan otomatis
   memberi label botnet/malware. Pertahankan baseline sebelum integrasi dan
   gunakan holdout berbeda untuk menguji perubahan sesudahnya. Uji TTL dan
   removal pada refresh agar IP lama tidak terus dianggap proxy.

Avastel belum dimasukkan ke feed produksi. Gate akurasi tetap terbuka.

## Dataset akun Avastel: staging privat — 13 Oktober 2026

Key dari file operator dipakai hanya untuk download ZIP CSV pada endpoint akun.
Source akun berbeda ketentuannya dari repository GitHub MIT. Paket Free resmi
menyatakan tidak termasuk hak redistribusi. Dataset akun tidak masuk API produksi;
penggunaan lookup turunan harus dipastikan diperbolehkan sebelum aktivasi.
[Ketentuan paket resmi](https://deviceandbrowserinfo.com/product/proxies-ips).

Alat `scripts/stage-avastel-account.py` membaca token/file assignment tanpa
mengeksekusinya, menolak redirect, membatasi ZIP 128 MiB dan CSV 512 MiB,
memvalidasi header/IP/flag, serta menormalisasi **hanya isProxy=True**.
Error tidak mencetak URL berkredensial. Arsip tidak diekstrak ke path dari ZIP.
Dedup memakai SQLite dengan cache 8 MiB; cache ini bukan batas total RAM Python.
Seluruh hasil bersifat privat, tanpa raw data atau API key di repository.

```bash
python3 scripts/stage-avastel-account.py \
  --key-file /private/ipavastel.txt --output /private/avastel-account-stage-new

mkdir -p /private/avastel-account-stage-new/bin
go build -o /private/avastel-account-stage-new/bin/ ./cmd/ingestor ./cmd/compiler ./cmd/api
python3 scripts/stage-avastel-pipeline.py \
  --dataset-dir /private/avastel-account-stage-new
```

Pipeline membutuhkan Linux, Docker, `psql`, dan Go sesuai `go.mod`. PostgreSQL
sementara dibatasi 1 GiB dan bind ke loopback port acak. Worker menggunakan feed
normalized sementara; API serta HTTP source staging juga hanya bind loopback.
Container/volume sementara dan server dibersihkan setelah selesai/gagal. Database
produksi tidak dipakai. Hasil MMDB/log privat dipertahankan untuk audit.

### Hasil nyata

- Download: 22.648.205 byte ZIP, 87.507.971 byte CSV, tepat 1.000.000 baris IP valid.
- `isProxy=True`: 999.388 baris; `isProxy=False`: 612 baris yang dikecualikan dari
  feed proxy. Semua flag divalidasi, tidak diganti berdasarkan nama dataset.
- CSV mencakup ASN, network ASN, negara, isDataCenter, isProxy dan proxyType;
  tahap ini hanya memasukkan IP/label proxy, tidak mengganti klasifikasi ASN/GeoIP
  produksi memakai metadata provider.
- Native Go ingestor: 26,23 detik, peak RSS 630.264 KiB (~616 MiB).
- Native Go compiler: 25,37 detik, peak RSS 1.513.436 KiB (~1.443 GiB).
- PostgreSQL: 999.388 active rows, ukuran database 391.607.987 byte (~373 MiB).
- MMDB: 51.384.657 byte (~49 MiB), offline verification lulus.
- Lookup API staging 100 sampel yang sudah diingest: proxy=true 100/100,
  score=32, risky=false seluruhnya, satu data_version dan health healthy.
  Ini verifikasi integrasi pada data sendiri, **bukan angka akurasi independen**.
  Bobot 40/confidence 0,8 hanya konfigurasi eksperimen staging.

[Evidence agregat](evidence/phase6-avastel-account-staging-2026-10-13.json) merekam
hash, kolom, jumlah data, versi MMDB dan resource. Raw artifacts ada di
`/home/natama/Documents/beon-backups/avastel-account-stage-20261013` (privat).

Pengukuran dilakukan pada mesin lokal **x86_64**, source Avastel saja, tanpa
GeoLite2, tanpa pilot feeds, dan tanpa load produksi. Peak worker bukan peak RAM
seluruh stack. GOMEMLIMIT 700/1100 MiB dan GOGC=50 merupakan target Go, bukan cap
RSS. Hasil ini tidak membuktikan dataset gabungan aman pada VPS ARM 4 GB.

Compiler source tunggal sudah dekat dengan cap compiler VPS 1,5 GiB. Sebelum
produksi, lakukan pengujian dataset gabungan pada ARM dengan cap nyata, cek
retention/refresh/removal source, kemudian tentukan optimasi atau upgrade resource.
Korpus negatif independen dan holdout berbeda tetap diperlukan untuk evaluasi
akurasi sesudah integrasi. Tidak ada timer akun atau ingest produksi yang diaktifkan.

## Gabungan pilot + Avastel pada ARM — 13 Oktober 2026

Staging di VPS memakai image rc.3 berbasis digest ARM64, clone PostgreSQL konsisten
melalui read-only pg_dump, salinan GeoLite2 City/ASN, serta konfigurasi source
pilot aktif ditambah source akun staging. Network Docker internal, PostgreSQL,
HTTP source, dan MMDB terpisah. Tidak ada perubahan feed/MMDB produksi.

Guard membutuhkan RAM available minimum 2.000 MiB sebelum staging dan 1.800 MiB
sebelum worker. Worker dihentikan bila RAM available turun di bawah 600 MiB atau
melewati 300 detik. Batas container: PostgreSQL 384 MiB/0,5 CPU; ingestor 896 MiB;
compiler 1.536 MiB; masing-masing worker 1 CPU. Ini pengujian dengan batas
resource, bukan benchmark throughput maksimum VPS.

Hasil nyata:

- Baseline 210.160 active rows, bertambah menjadi 1.209.548 setelah ingest akun.
- Ingest ARM berhasil: 71,47 detik, process HWM teramati 679.862.272 byte
  (~648 MiB), tidak OOM. Peak cgroup teramati 674.111.488 byte.
- Compiler membaca seluruh 1.209.548 rows, tetapi **dihentikan guard setelah
  15,80 detik** karena host pressure. Peak proses teramati 1.601.388.544 byte
  (~1,49 GiB), cgroup 1.593.069.568 byte, dekat cap 1,5 GiB.
- Exit 137 akibat guard kill; Docker `OOMKilled=false`. Jangan mencatatnya sebagai
  kernel OOM atau compile sukses. Tidak ada MMDB kandidat yang selesai diterbitkan.
- Database staging gabungan berukuran 595.875.507 byte (~568 MiB).
- Pemeriksaan API produksi tetap healthy, snapshot stale=false, runtime rc.3;
  alert checker Result=success. Container/volume/network staging sudah dihapus.

[Evidence ARM](evidence/phase6-avastel-combined-arm-2026-10-13.json) tersedia tanpa
raw data atau secret. Artifact privat disimpan pada
`/var/lib/beon-ipquality/staging/avastel-arm-20261013` dan laporan lokal di folder
staging akun operator. Script reusable `scripts/stage-avastel-arm.py` memerlukan
root, Python/PyYAML, Docker dan layout pilot yang sama. Ia membuat direktori baru
serta membaca source normalized melalui path/hash; API key akun tidak dikirim ke VPS.

### Keputusan

Dataset gabungan **belum lolos gate resource** pada pilot 4 GB. Jangan menaikkan
cap compiler pada host yang sama tanpa headroom terukur. Optimasi berikutnya
menargetkan peak aggregation: implementasi sekarang menahan raw rows, map semua
prefix dengan map source/threat per-prefix, dan output entries pada satu waktu.
Ukur perbaikan dengan regresi CIDR overlap, whitelist, determinisme evidence,
ASN, serta pipeline nyata sebelum mengulang staging ARM dengan batas yang sama.

Rekomendasi awal 4 vCPU/8 GB tetap merupakan estimasi headroom; bukan host yang
sudah diuji atau di-upgrade. Hak penggunaan lookup turunan dataset akun dan
korpus holdout independen tetap merupakan gate terpisah.

## Optimasi aggregation dan uji ulang ARM — 13 Oktober 2026

Compiler kini menormalisasi/sort stable indeks prefix, lalu memproses satu grup
prefix pada satu waktu. Temporary aggregate/source/threat maps tidak lagi disimpan
untuk seluruh jaringan sekaligus. Input tidak dimutasi; stable sort mempertahankan
urutan tie primary threat, lexicographic prefix order, evidence, confidence dan flags.
Regresi membandingkan output terhadap implementasi sebelumnya pada prefix alias,
IPv4/IPv6, duplicate sources, invalid ranges, decay dan equal-weight ties.

Microbenchmark lokal 10k prefix/3 iterasi: allocation total ~16,8 → 14,9 MB/op,
150.086 → 140.006 allocations/op; waktu ~29,4 → 37,5 ms/op. Ini tradeoff sorting
versus retained maps, bukan klaim peak RSS atau peningkatan throughput produksi.

Binary kandidat ARM64 dipasang **hanya** pada container staging lewat bind mount;
image/digest produksi tidak diubah. Pada clone gabungan 1.209.548 rows,
ingest selesai 71,03 detik. Kandidat berhasil melewati aggregation dan mulai writer
pada 1.147.296 entries, tetapi kemudian **container OOM** pada cap 1.536 MiB.
Compiler exit 137, OOMKilled=true, ~62,82 detik. Peak proses teramati 1.627.529.216
byte; peak cgroup teramati 1.608.556.544 byte. Counter process HWM dan cgroup memory
berbeda cakupan; angka ini bukan pengukuran peak kebutuhan final compile sukses.
Berbeda dari percobaan sebelumnya yang dihentikan host-pressure guard,
percobaan kandidat ini dihentikan kernel/container OOM; guard aborted=null.

[Evidence kandidat](evidence/phase6-compiler-grouped-arm-2026-10-13.json)
memuat binary hash, limit, outcomes dan regresi. Tidak ada MMDB kandidat selesai
terverifikasi. API pilot rc.3 tetap healthy/fresh; container/network staging sudah
dibersihkan. Optimasi ini belum dirilis atau dipasang pada compiler produksi.

### Rekomendasi upgrade untuk tahap dataset gabungan

Prioritaskan **RAM 8 GB**, dengan **4 vCPU** sebagai rekomendasi headroom CPU.
Disk VPS masih ~22 GB bebas sehingga resize disk belum menjadi syarat pengujian;
80 GB tetap target headroom produksi untuk DB/history/backup. Pilot feed terkurasi
saat ini tetap berjalan pada VPS 4 GB.

Setelah resize RAM, sesuaikan cap compiler karena VM resize tidak otomatis
mengubah `mem_limit` container. Batas awal 2,5 GiB untuk eksperimen compiler adalah
estimasi engineering, bukan peak kebutuhan yang sudah terbukti. Ulangi bounded
ARM staging, ukur writer/pipeline gabungan dan headroom keseluruhan sebelum
menentukan limit final atau mengaktifkan source akun. Penggunaan dataset akun
pada API dan evaluasi holdout tetap belum ditutup.
