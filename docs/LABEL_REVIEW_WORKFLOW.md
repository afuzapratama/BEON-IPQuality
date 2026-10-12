# Review label independen

Operator belum mempunyai korpus berlabel independen. Tahap ini menyiapkan review;
angka precision/recall/false-positive produksi belum tersedia.

1. Kumpulkan IP dari observasi operator atau pengujian proxy yang dimiliki/diizinkan,
   dengan waktu, sumber bukti dan kategori yang benar-benar diketahui. Ketiadaan
   blacklist tidak membuktikan IP non-proxy. Dataset Avastel yang sudah diingest,
   hasil API BEON, serta feed penyusun MMDB bukan ground truth independen.
2. Reviewer melihat bukti, bukan skor/prediksi API. Isi label per kategori; unknown
   dihilangkan dari `labels`, jangan diberi false. Proxy/VPN tidak otomatis risky,
   botnet atau malware. Tinjau bukti yang bertentangan sebelum approval.
3. Catat `training_overlap` dan independensi source, termasuk upstream agregator.
   Sampel yang berada pada source/range training disimpan sebagai diagnostic,
   tidak masuk evaluasi holdout independen.
4. Isi `split_group` dengan ASN/jaringan/grup observasi terkait, agar sampel yang
   saling berkorelasi tetap pada split sama. Freeze label sebelum mengambil
   prediksi API; hitung baseline per-category dan balance positive/negative.
5. Jalankan validator/splitter, ambil prediksi pada satu data_version, lalu pakai
   evaluator scoring. Tune threshold hanya pada tuning; holdout dipakai setelah
   keputusan tuning dibekukan. Dataset kecil/label tidak lengkap tetap dilaporkan
   dengan keterbatasannya; tidak ada target accuracy yang dikarang.

Template **belum berlabel** (IP hanya contoh format):

```json
{"ip":"8.8.8.8","labels":{},"label_source":"","observed_at":"","reviewed_by":"","evidence_uri":"","split_group":"ASN15169","independent":false,"training_overlap":true,"review_status":"unreviewed"}
```

Simpan review dan bukti privat di luar repository. `scripts/label-review.py`
menolak approval tanpa provenance, independence, training-overlap review dan
split group. Input approved harus memakai boolean untuk label yang diketahui.
Record unreviewed/rejected tidak dihitung sebagai negative. Split group hash
bersifat deterministic 80/20 secara grup; pastikan kedua split mempunyai jumlah
sampel/class balance cukup (grup sedikit dapat menghasilkan split kosong).

```bash
python3 scripts/label-review.py --reviews /private/reviews.jsonl --output /private/reviewed-v1
python3 scripts/evaluate-scoring.py \
  --labels /private/reviewed-v1/holdout-labels.jsonl \
  --predictions /private/holdout-predictions.jsonl
```

API tetap memakai heuristic family-max. Source akun diberi weight 40/confidence
0,8 sebagai baseline engineering; skor 32 bukan probabilitas atau angka akurasi.
