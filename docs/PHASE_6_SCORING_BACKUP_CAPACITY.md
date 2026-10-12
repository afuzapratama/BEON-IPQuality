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

RPO target backup DB harian maksimum 24 jam; schedule bukan bukti sukses.
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
