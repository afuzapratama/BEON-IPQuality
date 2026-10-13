# BEON-IPQuality

Layanan lookup reputasi IP privat: feed dikumpulkan ke PostgreSQL, compiler menyusun
MMDB, lalu API mengembalikan sinyal proxy/VPN/Tor, reputasi dan GeoIP. Skor merupakan
heuristic, bukan probabilitas atau angka akurasi. Data yang tidak tersedia atau
kedaluwarsa menghasilkan HTTP 503.

## Produksi

Base URL: **https://ipproxy.beon-api.site**. Runtime memakai image digest
[`v1.1.0-rc.6`](https://github.com/afuzapratama/BEON-IPQuality/releases/tag/v1.1.0-rc.6),
source aplikasi dan konfigurasi host dikelola di `main`. VPS ARM64: 4 vCPU, RAM 8 GB, disk persisten 30 GB.
NVMe Azure 220 GB hanya untuk staging sementara; data utama tetap persisten.

| Komponen | Status |
|---|---|
| API dan halaman lookup | HTTPS aktif; lookup memerlukan API key |
| Dataset | 21 grup / 33 sumber, termasuk akun Avastel privat |
| Scoring | `evidence-family-max-v1`, threshold risky 50 |
| Monitoring | Prometheus internal + Grafana dengan login di `/grafana/`; Glances melalui SSH |
| Alert | Telegram setiap menit + uptime eksternal GitHub sekitar lima menit |
| Backup | Harian, terenkripsi age ke Bunny S3; restore host terpisah sudah diuji |

Konfigurasi Caddy digunakan pada VPS. Template Nginx tersedia sebagai alternatif.
Judge/active scanning, ClickHouse dan stack analytics bukan bagian deployment aktif.

## Akses API

Halaman utama menyediakan lookup IPv4/IPv6 dengan kolom API key. Halaman tidak
menyimpan key. API key klien dan admin key berbeda; keduanya dikirim melalui header
`X-API-Key`, bukan query URL.

| Method | Path | Akses | Fungsi |
|---|---|---|---|
| GET | `/` | Publik | Halaman lookup |
| GET | `/health` | Publik | Kesiapan API dan freshness dataset; 503 bila belum siap |
| GET | `/api/v1/check/:ip` | API key | Lookup IP publik |
| POST | `/api/v1/check/batch` | API key | Lookup batch; maksimal 100 IP pada produksi saat ini |
| GET | `/api/v1/stats` | API key | Statistik sejak proses API dimulai |
| GET | `/api/v1/cache/stats` | Admin key | Statistik cache |
| DELETE | `/api/v1/cache` | Admin key | Hapus cache aplikasi |
| POST | `/api/v1/reload` | Admin key | Reload database MMDB |
| GET | `/grafana/` | Login Grafana | Dashboard API, feed dan server |
| GET | `/metrics` | Internal Prometheus | Ditolak pada HTTPS publik (403) |

Browser address bar tidak dapat mengirim header API key. Gunakan halaman lookup,
curl, atau client API. GET tidak bisa menggantikan method POST/DELETE.
Kuota produksi: 1.000 request/menit/API key. Admin key kosong menonaktifkan route
admin; produksi mempunyai admin key terpisah. Cache Redis untuk lookup saat ini
dinonaktifkan, tetapi service Redis tetap tersedia.

```bash
# API_BASE_URL dan API_KEY berasal dari file client privat.
curl -H "X-API-Key: $API_KEY" "$API_BASE_URL/api/v1/check/8.8.8.8"
curl -X POST -H "X-API-Key: $API_KEY" -H 'Content-Type: application/json' \
  -d '{"ips":["8.8.8.8","2606:4700:4700::1111"]}' \
  "$API_BASE_URL/api/v1/check/batch"
```

Kontrak lengkap: [API](docs/API.md) dan [OpenAPI](docs/openapi.yaml).

## Pembaruan data

- Feed reguler mengikuti [profil ARM 8 GB](configs/feeds.pilot.arm-8gb.yaml),
  dengan interval per sumber: lima menit sampai mingguan.
- Avastel akun diperbarui setiap **7 jam**, dengan validasi dan publikasi atomik.
  Gagal download tidak menghapus snapshot valid sebelumnya.
- GeoLite2 City/ASN: **Rabu dan Sabtu pukul 12:00 WIB**, jitter maksimal 30 menit.
  City rilis Selasa/Jumat; ASN yang rilis harian ikut diambil dua kali seminggu.
- Compiler reguler berjalan setiap 15 menit; API memeriksa reload setiap 5 menit.
  Publisher Avastel langsung compile/reload setelah refresh berhasil.

GeoIP ditulis ke `/opt/beon-ipquality/data/mmdb`. Source freshness dihitung per
interval feed; umur maksimum snapshot compiler satu jam. Scoring memakai sinyal
terkuat per keluarga agar feed yang tumpang tindih tidak berulang kali menaikkan skor.
[Runbook Avastel](docs/AVASTEL_PRIVATE_PRODUCTION.md) menjelaskan publikasi privat.

## Monitoring dan backup

Buka **https://ipproxy.beon-api.site/grafana/** dan login dengan kredensial operator.
Prometheus mengambil metrik melalui jaringan Docker, setiap 30 detik. Port 3000/9090
hanya loopback; node exporter tidak dipublikasikan. Retention Prometheus tujuh hari
atau target 2 GB, mana yang tercapai dahulu; sediakan ruang tambahan untuk WAL dan
compaction. Tidak ada ClickHouse atau plugin tambahan pada stack produksi.

Backup pukul **02:30 WIB** + jitter maksimal 15 menit: dump PostgreSQL, MMDB,
konfigurasi dan secret pemulihan → verifikasi restore → enkripsi age → Bunny S3 →
verifikasi checksum hasil download. Retention lokal minimum tujuh hari/tujuh point,
offsite minimum 30 hari. Private key age disimpan terpisah dari VPS.
Target RPO sekitar 24 jam + jitter.

Restore Bunny ke host terpisah sudah lulus: 1.287.574 rows dan API pulih sekitar
129 detik setelah download/decrypt/extract. Angka tersebut tidak mencakup provisioning
VPS dan DNS/TLS. Dashboard diprovision dari repository; kredensial monitoring ikut
backup terenkripsi. Riwayat Prometheus dan perubahan user/preferences Grafana tidak
tercakup backup aplikasi saat ini.

Panduan: [monitoring produksi](docs/PRODUCTION_MONITORING.md),
[backup/restore](docs/BACKUP_RESTORE_ROLLBACK.md),
[drill Bunny](docs/DISASTER_RECOVERY_DRILL.md),
[deployment ARM](docs/VPS_ARM_DEPLOYMENT.md).

## Development

Development memakai `docker-compose.yml`; produksi memakai
`deployments/compose.production.yml` dan stack monitoring terpisah
`deployments/compose.monitoring.yml`. Jangan menjalankan Compose development di
folder produksi. Feed default development lebih besar daripada profil VPS terkurasi.

```bash
cp .env.example .env
# Isi secret lokal sesuai template sebelum menjalankan Compose.
docker compose up -d --build
curl http://127.0.0.1:8080/health

go test -race ./...
go vet ./...
python3 -m unittest discover -s tests/scoring
python3 -m unittest discover -s tests/operations
```

Test staging ARM membutuhkan PyYAML. CI memeriksa race/vet/build/vulnerability,
integrasi PostgreSQL/Redis, Compose bootstrap/migration, backup/restore, OpenAPI,
proxy, GeoIP dan validasi monitoring. Profil development opsional dijelaskan di
[Compose development](docker-compose.yml).

## Release dan pekerjaan tersisa

`main` adalah branch source aktif; **release produksi tetap `v1.1.0-rc.6`**.
Workflow release memberi label pre-release pada tag yang mengandung suffix seperti
`-rc.4`. Merge source tidak mengganti tag atau image yang berjalan di VPS.

Sudah selesai: deployment ARM, dataset akun privat, backup offsite, restore host
terpisah, monitoring/alert, probe HTTPS 1.500 request, rollback dan warm reboot.
Masih terbuka sebelum menyatakan seluruh gate produksi selesai:

1. Label independen dan evaluasi precision/recall/false positive; cohort diagnostik
   258 IP tersedia, tetapi belum ada label independen yang disetujui.
2. Uji beban berkelanjutan dan pengukuran pertumbuhan disk beberapa hari.
3. RTO kehilangan VPS termasuk provisioning OS, refresh provider dan DNS/TLS.

Tidak ada bobot scoring yang dituning tanpa bukti. Lihat [review label](docs/LABEL_REVIEW_WORKFLOW.md),
[status phase 6](docs/PHASE_6_SCORING_BACKUP_CAPACITY.md) dan
[bukti operasional](docs/OPERATIONS_RC4_VALIDATION.md).
Stable berikutnya direncanakan sebagai `v1.1.0` setelah scope/gate release ditutup;
release lama `v1.0.0` tidak mencerminkan deployment rc.6 saat ini.

## Lisensi

[MIT](LICENSE). Lisensi kode tidak menggantikan ketentuan penggunaan setiap dataset.
