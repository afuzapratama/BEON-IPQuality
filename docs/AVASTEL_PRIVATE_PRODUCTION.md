# Dataset akun Avastel untuk produksi privat

Operator mengonfirmasi penggunaan di lingkup privat dan meminta dataset akun
langsung diintegrasikan. Source aktif pada runtime `v1.1.0-rc.4` mulai 13 Oktober
2026. File mentah dan key tidak dipublikasikan; API lookup memerlukan key BEON.

## Alur dan interval

`beon-avastel-update.timer`: **7 jam dari aktivasi job sebelumnya**, plus job
pemulihan 10 menit setelah boot. Interval 6 jam adalah jadwal pembaruan dataset
penyedia, bukan interval wajib downloader kita. Timer dapat mengalami delay ketika
host sibuk; monitor tetap memeriksa freshness dan hasil job.

Download ZIP terautentikasi → bounded validation → dedup SQLite → hanya
`isProxy=True` → switch direktori normalized/manifest atomik → dedicated Go ingest
→ compile → reload API → prune expired history akun. Retry download tiga kali;
kegagalan tidak menghapus snapshot MMDB yang sebelumnya valid.

- Config main: `configs/feeds.pilot.arm-8gb.yaml`, source `avastel_account`,
  `externally_managed: true`, `schedule: '@every 7h'`.
- Config job: `configs/feeds.avastel-publisher.yaml`; hanya source akun,
  whitelist disabled agar tidak mengubah whitelist lain.
- Daemon ingestor melewati source externally managed; compiler tetap memasukkan
  data dan freshness-nya. Dedicated job menyimpan last-success/expiry melalui
  mekanisme transaksi refresh source yang sama dengan feed lain.
- Scoring: proxy/anonymity, weight 40, confidence 0,8; tidak otomatis botnet atau
  malware. Source sendiri umumnya score 32; evidence lain dapat menambah risiko.
- Compiler memakai advisory lock PostgreSQL lintas proses, sehingga daemon dan
  job publisher tidak membuat dua writer besar bersamaan.

## Path dan perlindungan secret

Key `/etc/beon-ipquality/avastel.key` mode 0600, dibaca sebagai token/data,
bukan shell. `scripts/stage-avastel-account.py` tidak mencetak URL berkredensial
atau key. Go feed URL hanya `http://172.18.0.1:18090/proxy-ips.txt`.

`beon-avastel-source.service` bind **gateway bridge Docker saja**, bukan IP publik.
Ia hanya melayani path normalized tersebut; data lebih tua dari 7 jam 20 menit
mengembalikan 503 agar file lama tidak ditandai sebagai download fresh.
API memakai metadata freshness source dengan budget 26.535 detik (~7j22m15d).
Waktu fetch bukan per-IP observation time; label provider tidak membuktikan
bahwa proxy masih aktif saat lookup. Data akun `isProxy=False` dikecualikan.

`/opt/beon-ipquality/data/avastel/current` adalah link snapshot normalized privat
pada storage persisten. Temporary ZIP/SQLite dibersihkan; snapshot normalized
lama dipruning setelah 24 jam. Account reputation yang expired dan last_seen
lebih tua dari 24 jam dihapus setelah publish sukses; active rows tidak dihapus.
History feed lain mengikuti retention sebelumnya. Pantau pertumbuhan disk DB/WAL/
backup; NVMe sementara tidak digunakan sebagai database produksi.

API key akun masuk bundle backup **privat yang kemudian dienkripsi age** sebagai
`avastel.key`, termasuk checksum. Private recovery key age tetap di komputer
operator, tidak dimasukkan bundle atau disimpan di VPS.

## Operasi

```bash
sudo systemctl status beon-avastel-source.service
sudo systemctl list-timers beon-avastel-update.timer
sudo systemctl start beon-avastel-update.service
sudo journalctl -u beon-avastel-update.service --no-pager -n 30
```

Monitor Telegram memeriksa Result service source/update selain health, pipeline,
container dan backup. Freshness failure mempertahankan kebijakan API fail-closed.

## Bootstrap/recovery

Restore `.env`, lock image, config, database/MMDB dan operations dari bundle privat.
Copy `avastel.key` mode 0600 ke `/etc/beon-ipquality`, pasang source service/timer,
dan cocokkan gateway aktual Docker dengan AVASTEL_SOURCE_HOST dan URL config.

Pada DB baru: hidupkan PostgreSQL dan jalankan migrasi, lalu jalankan
`sudo python3 scripts/avastel-sync.py prepare` untuk download + ingest akun
sebelum bootstrap compiler. Mode prepare tidak membutuhkan API sudah berjalan.
Ingest-init regular dan compiler-init dapat dilanjutkan setelah data/freshness akun
ada. Setelah API berjalan, refresh penuh dan aktifkan timer.

Cap compiler aktual 2.560 MiB, Go target 1.800 MiB pada VPS 4 vCPU/8 GB. Pilot
terkurasi + akun telah diuji; jangan mengaktifkan seluruh default feed jutaan baris
berdasarkan angka ini saja. Source/DB/secrets tetap persisten di disk OS.

## Penanganan timeout — 14 Oktober 2026

Worker ingest mempunyai deadline 30 menit dan compiler 15 menit; unit keseluruhan 60 menit. Worker memakai nama container unik. Saat timeout, koneksi PostgreSQL dari IP container yang masih dimiliki job dihentikan sebelum container dibuang, sehingga query tidak tertinggal. Expiry source sekarang memakai marker timestamp publikasi DB setelah source lock, di dalam transaksi yang sama dengan upsert; tidak memakai anti-join jutaan row. Batas freshness tidak dilonggarkan. PG cache cap pilot 512 MiB dinaikkan ke 1.536 MiB pada VPS 8 GB. Validasi satu job systemd sukses lengkap 2 menit 22 detik; siklus dataset berubah masih perlu dipantau.
