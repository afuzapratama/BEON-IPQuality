# BEON-IPQuality

BEON-IPQuality mengumpulkan feed reputasi IP ke PostgreSQL, menggabungkan sinyal lintas sumber, lalu menyusun database MaxMind (MMDB) untuk lookup API yang cepat. API bersifat fail-closed: bila MMDB belum tersedia atau gagal dibaca, endpoint reputasi mengembalikan HTTP 503 dan tidak menyatakan IP sebagai bersih.

## Komponen

- `ingestor`: mengambil feed, memvalidasi IP/CIDR, menyinkronkan whitelist, dan melakukan upsert ke PostgreSQL.
- `compiler`: mengagregasi sumber dan threat type, menghitung skor, lalu menulis MMDB secara atomik.
- `api`: lookup MMDB, cache Redis opsional, autentikasi API key, rate limit, health check, dan metrik Prometheus.
- `judge`: lookup dan active proxy scanning opsional. Service ini dinonaktifkan dari Compose standar dan hanya bind ke loopback ketika profilnya dipakai.

## Menjalankan dengan Docker Compose

Persyaratan: Docker dengan Compose v2 dan akses internet untuk mengambil feed.

```bash
cp .env.example .env
```

Isi sekurangnya `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, dan `API_KEY` (minimal 32 karakter acak), lalu jalankan:

```bash
docker compose up --build -d
docker compose ps
curl http://127.0.0.1:8080/health
```

Startup pertama menjalankan migrasi, ingest awal, dan kompilasi MMDB sebelum API dinyalakan. Proses ini bergantung pada ukuran feed dan koneksi jaringan. API secara default hanya tersedia di `127.0.0.1:8080`; gunakan reverse proxy HTTPS sebelum mengubah `API_BIND_ADDRESS` menjadi `0.0.0.0`.

Profil opsional:

```bash
# Prometheus, Grafana, ClickHouse, dan exporter
docker compose --profile monitoring up -d

# Judge active scanner (tetap bind ke 127.0.0.1)
docker compose --profile active-scanning up -d judge

# Pembaruan GeoLite2; isi kredensial MaxMind terlebih dahulu
docker compose --profile geoip up -d geoip-updater
```

Jangan menjalankan Judge sebagai endpoint publik. Walaupun target non-publik ditolak dan autentikasi diwajibkan di production, fitur ini tetap melakukan koneksi aktif ke host tujuan.

## API

Health dan metrics tidak memerlukan API key. Endpoint lookup `/api/v1/*` memerlukan `X-API-Key: $API_KEY` ketika autentikasi aktif. Route reload dan cache management memakai `X-API-Key: $ADMIN_API_KEY` yang berbeda; bila `ADMIN_API_KEY` kosong, route admin mengembalikan 404. Admin tetap diakses melalui loopback/SSH. Kuota lookup dihitung per key setelah autentikasi; health/metrics tidak dibatasi kuota tersebut.

Header `X-Real-IP` hanya dipercaya dari peer pada `TRUSTED_PROXIES` (IP/CIDR dipisahkan koma). Default kosong mempercayai tidak ada proxy. Nginx harus menimpa header tersebut dengan alamat klien; tentukan peer nyata dari jaringan deployment sebelum mengisi daftar ini.

```bash
curl http://127.0.0.1:8080/health

curl -H "X-API-Key: $API_KEY" \
  http://127.0.0.1:8080/api/v1/check/8.8.8.8

curl -X POST \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"ips":["8.8.8.8","1.1.1.1"]}' \
  http://127.0.0.1:8080/api/v1/check/batch
```

Endpoint yang tersedia:

| Method | Path | Keterangan |
|---|---|---|
| `GET` | `/health` | Kesiapan API dan MMDB; 503 bila reputasi tidak tersedia |
| `GET` | `/metrics` | Metrik Prometheus |
| `GET` | `/api/v1/check/:ip` | Lookup satu IP publik |
| `POST` | `/api/v1/check/batch` | Lookup hingga batas `api.batch_max_size` |
| `GET` | `/api/v1/stats` | Statistik sejak proses API dimulai |
| `GET` | `/api/v1/cache/stats` | Statistik cache |
| `DELETE` | `/api/v1/cache` | Menghapus cache aplikasi |
| `POST` | `/api/v1/reload` | Reload MMDB dan menghapus cache |

Respons lookup memakai field `score`/`risk_score`, `risk_level`, flag seperti `proxy`, `vpn`, dan `tor`, serta `threat_types` dan `sources` bila ditemukan. IP private, loopback, link-local, multicast, dan alamat non-global lain ditolak.

## Pengembangan

Rencana pekerjaan berikutnya mengikuti [phase perbaikan dan deployment VPS ARM](docs/PHASE_PERBAIKAN_DAN_VPS_ARM.md), berdasarkan [audit docs dan implementasi](docs/AUDIT_DOCS_IMPLEMENTASI_2026-10-11.md). Target deployment awal adalah VPS ARM64 langsung dengan Compose, Nginx, dan monitoring server ringan tanpa aaPanel. Akses VPS diperlukan mulai phase 4, setelah perbaikan dan paket deployment siap.

Go minimum mengikuti directive di `go.mod`.

```bash
go mod download
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
go build ./cmd/...
```

Perintah ringkas juga tersedia melalui `make test`, `make test-race`, `make vet`, `make security`, dan `make build`. Untuk migrasi manual:

```bash
DATABASE_URL='postgresql://user:pass@host/db?sslmode=require' make migrate
```

Konfigurasi file berada di `configs/config.yaml` dan `configs/feeds.yaml`. Semua executable menggunakan environment override, termasuk `DATABASE_URL`, `REDIS_URL`, `PORT`, `API_KEY`, dan path MMDB. Lihat `.env.example` serta `railway.env.example`.

## Deployment Railway

Image API dapat dibangun melalui `railway.json`, tetapi MMDB bukan bagian dari image. Pasang persistent volume pada `/app/data`, lalu pastikan `/app/data/mmdb/reputation.mmdb` sudah dihasilkan compiler sebelum health check API. PostgreSQL dan Redis dapat menggunakan `DATABASE_URL` dan `REDIS_URL`. Detailnya ada di `docs/RAILWAY_DEPLOYMENT.md`.

## Catatan installer Ubuntu

`scripts/install-ubuntu.sh` membangun binary dari source checkout dan mempertahankan database yang sudah ada saat dijalankan ulang. Untuk server penting, clone repository, tinjau commit dan script lebih dahulu, lalu jalankan file lokal; jangan menyalurkan script internet langsung ke shell root tanpa verifikasi.

## Lisensi

[MIT](LICENSE)
