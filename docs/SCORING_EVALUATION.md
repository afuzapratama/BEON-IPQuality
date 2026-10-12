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
