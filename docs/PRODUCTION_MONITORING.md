# Monitoring produksi

Grafana: https://ipproxy.beon-api.site/grafana/ — login operator diperlukan.
Dashboard `beon-production` menampilkan 16 panel untuk API, freshness, feed,
compiler, CPU/RAM, trafik jaringan, disk I/O dan ruang root persisten.
Prometheus mengambil lima target melalui jaringan Docker setiap 30 detik:
API, ingestor, compiler, node exporter dan Prometheus sendiri.

## Deployment

Stack terpisah dari aplikasi menggunakan `deployments/compose.monitoring.yml`,
network external `beon-ipquality_beon`, image multiarch dengan digest, serta volume
persisten Docker pada disk OS. Jangan memakai NVMe sementara untuk histori yang
harus dipertahankan. Port Prometheus 9090 dan Grafana 3000 hanya loopback;
node exporter memakai host network/PID untuk mengukur trafik host sebenarnya,
tetapi bind hanya ke gateway Docker privat pada port 9100. HTTPS `/metrics` ditolak 403.
Grafana anonymous access dan metrics Grafana dinonaktifkan; form login tetap aktif.

```bash
docker compose --env-file /etc/beon-ipquality/monitoring.env \
  -f /opt/beon-ipquality/deployments/compose.monitoring.yml up -d
```

File `/etc/beon-ipquality/monitoring.env` harus 0600. Template tanpa secret ada
pada `configs/monitoring.env.example`. Password operator disimpan privat pada
komputer operator; jangan commit atau kirim ke query URL. Login melalui HTTPS.
Initial admin password dipakai saat database Grafana pertama dibuat; mengganti
file env tidak otomatis mengganti password admin pada database yang sudah ada.

Budget container: Prometheus 512 MiB, Grafana 512 MiB, exporter 64 MiB.
Retention tujuh hari atau target 2 GB, mana yang lebih dahulu; WAL/compaction
membutuhkan ruang tambahan sehingga ini bukan quota disk keras. Exporter memakai
mount host /proc, /sys dan root secara readonly, tanpa Docker socket.
BEON_GATEWAY harus sesuai IPAM gateway network aplikasi; container bridge biasa
akan mengukur trafik network namespace container, bukan trafik host.

Grafana memakai plugin Prometheus yang sudah ada pada image. Plugin preinstall,
auto-update dan instalasi melalui UI dimatikan supaya image readonly tetap
reproducible. Pada Grafana 13.2, auto-update plugin dapat menghentikan plugin
bundled sebelum update gagal menulis filesystem readonly. Tidak ada dependency
ClickHouse atau plugin tambahan yang dibutuhkan dashboard.
[Konfigurasi plugin Grafana](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/).

## Alert dan pemulihan

`MONITORING_ENABLED=true` pada operations.env membuat checker Telegram memantau
tiga container monitoring dan lima target scrape. Aktifkan setelah deployment
sehat. Glances dan monitor uptime GitHub tetap berjalan. Jangan menambah scheduler
backup/feed baru pada stack ini.

Backup aplikasi mencakup `monitoring.env` di bundle terenkripsi, serta dashboard,
provisioning dan definisi Compose di config archive. Grafana dapat dibangun ulang
dengan initial admin dan dashboard repository. Histori Prometheus, database user/
preferences Grafana dan perubahan password melalui UI belum dibackup oleh bundle
aplikasi. Backup komponen tersebut membutuhkan snapshot konsisten tersendiri.

## Validasi

CI memeriksa konfigurasi Prometheus, seluruh 16 ekspresi PromQL dashboard melalui
promtool dan syntax Caddy/Nginx. Validasi VPS memeriksa lima target up, login,
akses data dashboard, anonymous API ditolak, `/metrics` publik 403 dan seluruh
route API dengan role key yang tepat. Evidence agregat berada di
`docs/evidence/production-monitoring-2026-10-13.json`.

Audit live menemukan label HTTP Prometheus memakai string method pinjaman dari
buffer fasthttp. Method POST lalu GET dapat mengubah label menjadi GETT dan
membuat gather metrik gagal 500. Perbaikan menyalin label menjadi string milik
collector, dengan regresi reuse RequestCtx; perubahan ini memerlukan runtime baru.

Runtime aplikasi mengikuti digest release. Konfigurasi monitoring host mengikuti revision support yang dicatat pada VPS dan source `main`; perubahan host support tidak selalu memerlukan rebuild image aplikasi. Gunakan konfigurasi monitoring dari revision support terbaru, bukan menyalin snapshot awal monitoring dari archive release lama.

Validasi akhir: rc.6 healthy/fresh; enam route authenticated 200; metrics internal 200 setelah GET/POST/DELETE, publik 403; 5/5 scrape target UP; login dan 16 query dashboard lulus. Stack teramati sekitar 234 MiB RAM pada satu pemeriksaan. Rollout pertama kembali ke rc.4 karena lima source stale; refresh reguler + compile memperbarui snapshot sebelum retry rc.6 lulus. Prosedur rollout harus memastikan source/snapshot fresh sebelum recreating API; rollback image tidak memperbarui metadata feed yang sudah tua.
