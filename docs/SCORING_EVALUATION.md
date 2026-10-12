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
