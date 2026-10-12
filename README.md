# BEON-IPQuality

BEON-IPQuality mengumpulkan feed reputasi IP ke PostgreSQL, menggabungkan sinyal lintas sumber, lalu menyusun database MaxMind (MMDB) untuk lookup API yang cepat. API bersifat fail-closed: bila MMDB belum tersedia atau gagal dibaca, endpoint reputasi mengembalikan HTTP 503 dan tidak menyatakan IP sebagai bersih.

## Status deployment

Hasil verifikasi 12 Oktober 2026: pilot ARM64 berjalan dengan runtime
[`v1.1.0-rc.3`](https://github.com/afuzapratama/BEON-IPQuality/releases/tag/v1.1.0-rc.3)
dan [manifest image berbasis digest](deployments/releases/v1.1.0-rc.3.env).
VPS memakai Docker Compose, reverse proxy Caddy yang sudah ada, updater MaxMind
native, dan monitoring Glances tanpa aaPanel. Perubahan berada pada branch
`fix/production-readiness`, dengan [draft PR #1](https://github.com/afuzapratama/BEON-IPQuality/pull/1).

| Bagian | Status pilot |
|---|---|
| API HTTPS | `https://ipproxy.beon-api.site/api/v1/check/8.8.8.8`, memerlukan API key |
| Dataset | Feed terkurasi dari [konfigurasi pilot ARM 4 GB](configs/feeds.pilot.arm-4gb.yaml) |
| Scoring | `evidence-family-max-v1`; precision/recall belum diukur pada korpus independen |
| GeoIP | GeoLite2 City/ASN diperbarui ke direktori MMDB proyek oleh timer host |
| Backup | Backup terenkripsi age ke Bunny S3; download, decrypt, dan restore terisolasi sudah diuji |
| Alert | Telegram untuk health, pipeline, resource, dan backup; Glances melalui SSH tunnel |

Domain utama menyediakan halaman lookup IP dengan kolom API key. `/health` dan
`/metrics` dapat diakses langsung tanpa key. Route `/api/v1/*` tersedia lewat HTTPS:
lookup dan stats memerlukan API key; cache/reload memerlukan admin key yang berbeda.
Kuota publik pilot 1.000 request/menit/key. Key tidak disimpan oleh halaman lookup.

Phase 6 belum menutup seluruh gate produksi. Bukti pengujian, batas kapasitas,
dan pekerjaan tersisa tercatat pada [laporan phase 6](docs/PHASE_6_SCORING_BACKUP_CAPACITY.md).

## Komponen

- `ingestor`: mengambil feed, memvalidasi IP/CIDR, menyinkronkan whitelist, dan melakukan upsert ke PostgreSQL.
- `compiler`: mengagregasi sumber dan threat type, menghitung skor, lalu menulis MMDB secara atomik.
- `api`: lookup MMDB, cache Redis opsional, autentikasi API key, rate limit, health check, dan metrik Prometheus.
- `judge`: lookup dan active proxy scanning opsional. Service ini dinonaktifkan dari Compose standar dan hanya bind ke loopback ketika profilnya dipakai.

## Menjalankan dengan Docker Compose

Persyaratan: Docker dengan plugin Compose modern dan akses internet untuk mengambil feed.
Jalur ini untuk pengembangan. Deployment VPS memakai paket produksi pada bagian
[Paket produksi ARM64](#paket-produksi-arm64). Konfigurasi feed default berbeda
dari pilot terkurasi; seluruh feed default belum terbukti muat pada VPS 4 GB.

```bash
cp .env.example .env
```

Isi sekurangnya `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, dan `API_KEY` (minimal 32 karakter acak), lalu jalankan:

```bash
docker compose up --build -d
docker compose ps
curl http://127.0.0.1:8080/health
```

Startup pertama menjalankan migrasi berversi/checksum melalui service `migrate`, ingest awal, dan kompilasi MMDB sebelum API dinyalakan. Proses ini bergantung pada ukuran feed dan koneksi jaringan. API secara default hanya tersedia di `127.0.0.1:8080`; gunakan reverse proxy HTTPS sebelum mengubah `API_BIND_ADDRESS` menjadi `0.0.0.0`.

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

Health dan metrics tidak memerlukan API key dan dapat dibuka langsung melalui HTTPS pada VPS pilot. Endpoint lookup `/api/v1/*` memerlukan `X-API-Key: $API_KEY` ketika autentikasi aktif. Route reload dan cache management memakai `X-API-Key: $ADMIN_API_KEY` yang berbeda; bila `ADMIN_API_KEY` kosong, route admin mengembalikan 404. Route admin dapat diakses melalui HTTPS pada pilot, dengan admin key terpisah. Kuota lookup dihitung per key setelah autentikasi; health/metrics tidak dibatasi kuota tersebut.

Header `X-Real-IP` hanya dipercaya dari peer pada `TRUSTED_PROXIES` (IP/CIDR dipisahkan koma). Default kosong mempercayai tidak ada proxy. Reverse proxy (Caddy pada VPS pilot, atau Nginx) harus menimpa header tersebut dengan alamat klien; tentukan peer nyata dari jaringan deployment sebelum mengisi daftar ini.

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

Contoh akses pilot dari komputer klien; isi variabel dari file kredensial privat
yang sudah disiapkan, tanpa memasukkan secret ke repository:

```bash
# API_BASE_URL=https://ipproxy.beon-api.site
# API_KEY diambil dari konfigurasi klien privat.
curl --fail-with-body -H "X-API-Key: $API_KEY" \
  "$API_BASE_URL/api/v1/check/8.8.8.8"
```

Endpoint berikut tersedia langsung melalui domain pilot:

| Method | Path | Akses pada VPS pilot | Keterangan |
|---|---|---|---|
| `GET` | `/` | HTTPS publik; tanpa key | Halaman lookup IP dengan input API key |
| `GET` | `/health` | HTTPS publik; tanpa key | Kesiapan API/MMDB; 503 jika tidak siap atau stale |
| `GET` | `/metrics` | HTTPS publik; tanpa key | Metrik Prometheus |
| `GET` | `/api/v1/check/:ip` | HTTPS publik + API key | Lookup satu IP publik |
| `POST` | `/api/v1/check/batch` | HTTPS publik + API key | Lookup hingga batas `api.batch_max_size` |
| `GET` | `/api/v1/stats` | HTTPS publik + API key | Statistik sejak proses API dimulai |
| `GET` | `/api/v1/cache/stats` | HTTPS publik + admin key | Statistik cache |
| `DELETE` | `/api/v1/cache` | HTTPS publik + admin key | Menghapus cache aplikasi |
| `POST` | `/api/v1/reload` | HTTPS publik + admin key | Reload MMDB |

Pada pilot, Caddy menyajikan halaman utama serta meneruskan `/health`, `/metrics`,
dan `/api/v1/*` ke API. Lookup tanpa header `X-API-Key` mengembalikan 401.
Gunakan form pada [domain utama](https://ipproxy.beon-api.site) untuk mencoba lookup
melalui browser; API key tetap diperlukan. Halaman tidak menyimpan key di browser.
Batch harus memakai POST; reload memakai POST dan penghapusan cache memakai DELETE.

API key klien tersedia pada file privat `ipproxy-client.env` yang sudah diberikan
kepada operator. Jangan memasukkan key ke URL/query string atau repository.

Pemeriksaan langsung 13 Oktober 2026 setelah perubahan proxy: halaman utama,
health, metrics, stats dengan key, lookup single, dan batch HTTPS HTTP 200.
Lookup tanpa key dan route admin dengan key klien HTTP 401. Konfigurasi tersedia
pada [template Caddy](configs/caddy/beon-production.caddyfile), dengan sumber
halaman pada [index.html](configs/web/index.html). Template memuat HTML halaman;
sinkronkan blok HTML tersebut bila halaman diubah.


Respons lookup memakai field `score`/`risk_score`, `risk_level`, flag seperti `proxy`, `vpn`, dan `tor`, serta `threat_types` dan `sources` bila ditemukan. IP private, loopback, link-local, multicast, dan alamat non-global lain ditolak.

## Scoring, freshness, dan reload

Policy `evidence-family-max-v1` memakai kontribusi terkuat per family anonymity/abuse/infrastructure; datacenter/ASN sendiri maksimum 20, overlap CIDR dihitung ulang dari evidence dan tidak menggandakan skor. Nilai ini heuristic, bukan probabilitas atau angka akurasi yang sudah terkalibrasi. Lihat [phase 6](docs/PHASE_6_SCORING_BACKUP_CAPACITY.md).

Compiler memakai `scoring.decay_lambda`, `max_score`, `weights`, `asn_bonuses`, dan `risk_threshold`. Bobot dapat memakai ID feed pada `configs/feeds.yaml` atau ID feed/source; override source mengalahkan override feed, lalu bobot tersimpan, lalu default threat type. Respons menambahkan `risky` sesuai threshold (whitelist selalu false) dan `data_version` yang mengidentifikasi reputasi, GeoIP, serta ASN yang dipakai. ASN type berasal dari `asn_info`; GeoLite2-ASN hanya memberi nomor/org. Prefix yang melintasi beberapa record ASN tidak diberi satu klasifikasi ASN. Daftar ASN bawaan masih contoh terbatas.

Default compile 15 menit, pemeriksaan reload 5 menit, dan umur maksimum snapshot 1 jam. Freshness source memakai satu interval cron feed ditambah compile/reload dan anggaran retry HTTP. Feed harian tetap mempunyai anggaran lebih dari 24 jam; ini bukan janji semua feed diperbarui dalam satu jam. Waktu antre/storage/compile nyata perlu diukur pada pilot. `/health` menampilkan versi, build time, umur snapshot, timestamp sukses tiap source, dan status stale. Lookup dan health mengembalikan 503 saat snapshot/feed melewati anggaran. Snapshot legacy tanpa metadata feed menampilkan `feed_freshness_known: false`; bootstrap pipeline baru diperlukan untuk memantau tiap source.

Reload memverifikasi isi MMDB dan hanya mengganti generation bila checksum berbeda. Reload gagal mempertahankan semua database yang sebelumnya valid. File MMDB harus diganti secara atomik, bukan ditulis ulang di tempat. Cache memakai namespace versi; generation lama berakhir melalui TTL, sehingga reload tidak perlu menghapus semua cache. Operasi lookup cache dibatasi 50ms; error cache membuka jeda satu detik sebelum percobaan berikutnya dan lookup tetap memakai MMDB. Clear cache manual tetap tersedia dengan admin key.

Compose mengekspor metrik pipeline secara internal pada `ingestor:9091` dan `compiler:9092`; API memakai `/metrics`. Untuk daemon native di host yang sama, set `METRICS_PORT` berbeda pada masing-masing service, dan biarkan `METRICS_HOST=127.0.0.1`.

Uji integrasi deterministik dengan PostgreSQL 17, Redis 7, dan fixture feed/ASN:

```bash
./scripts/test-integration.sh
```

Script membuat container sementara dengan port loopback dan membersihkannya setelah pengujian; tidak mengambil threat feed internet. CI menjalankan test yang sama dengan service PostgreSQL/Redis.

## Pengembangan

Tracker pekerjaan berada di [phase perbaikan dan deployment VPS ARM](docs/PHASE_PERBAIKAN_DAN_VPS_ARM.md), berdasarkan [audit docs dan implementasi](docs/AUDIT_DOCS_IMPLEMENTASI_2026-10-11.md). Status operasional terbaru mengikuti [laporan phase 6](docs/PHASE_6_SCORING_BACKUP_CAPACITY.md); panduan deployment juga memuat rencana dan catatan tahap sebelumnya.

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

Kontrak API lengkap berada di [API.md](docs/API.md) dan [OpenAPI](docs/openapi.yaml).

Konfigurasi file berada di `configs/config.yaml` dan `configs/feeds.yaml`. Semua executable menggunakan environment override, termasuk `DATABASE_URL`, `REDIS_URL`, `PORT`, `API_KEY`, dan path MMDB. Lihat `.env.example` serta `railway.env.example`.

## Deployment Railway

Image API dapat dibangun melalui `railway.json`, tetapi MMDB bukan bagian dari image. Pasang persistent volume pada `/app/data`, lalu pastikan `/app/data/mmdb/reputation.mmdb` sudah dihasilkan compiler sebelum health check API. PostgreSQL dan Redis dapat menggunakan `DATABASE_URL` dan `REDIS_URL`. Detailnya ada di `docs/RAILWAY_DEPLOYMENT.md`.

## Catatan installer Ubuntu

`scripts/install-ubuntu.sh` membangun binary dari source checkout dan mempertahankan database yang sudah ada saat dijalankan ulang. Untuk server penting, clone repository, tinjau commit dan script lebih dahulu, lalu jalankan file lokal; jangan menyalurkan script internet langsung ke shell root tanpa verifikasi.

## Lisensi

[MIT](LICENSE)

## Paket produksi ARM64

Pilot berjalan di `/opt/beon-ipquality`, dengan API loopback port 8080 dan Caddy untuk HTTPS publik. Jalur VPS langsung memakai [Compose produksi](deployments/compose.production.yml) dan [panduan VPS ARM64](docs/VPS_ARM_DEPLOYMENT.md), dengan [backup/restore/rollback](docs/BACKUP_RESTORE_ROLLBACK.md). Image release dan `deployment-lock.env` memakai digest; source tidak dibuild ulang di VPS. Bind mount `/opt/beon-ipquality/data/mmdb` ke `/app/data/mmdb` membuat output geoipupdate host dibaca compiler/API.

Pada VPS pilot, konfigurasi MaxMind berada di `/etc/beon-ipquality/GeoIP.conf` dengan permission 0600. `GEOIP_CONFIG_FILE=/path/GeoIP.conf MMDB_HOST_DIR=/opt/beon-ipquality/data/mmdb ./scripts/update-geoip.sh` mengarahkan hasil update ke direktori proyek; timer systemd tersedia pada `configs/systemd`. File harus memuat AccountID, LicenseKey dan EditionIDs City/ASN. Download City/ASN dan timer `beon-geoipupdate.timer` sudah terpasang pada pilot. Jadwal 03:20 WIB dengan jitter maksimum 30 menit. Jalankan hanya satu updater; profil GeoIP container development tidak perlu diaktifkan bersamaan dengan timer host.

Refresh source sekarang atomik dan menggantikan sinyal aktif source tersebut. Removed IP langsung expired; source gagal/empty/malformed tidak menghapus snapshot lama. TTL dua freshness budget, expired history 30 hari dan cleanup harian; lihat kebijakan pada panduan VPS. Latest attempt/success feed dipertahankan walau history lain dipruning.

## Backup, monitoring, dan kapasitas

`beon-backup.timer` menjalankan backup harian pukul 02:30 WIB dengan jitter
maksimum 15 menit. Pipeline memverifikasi checksum dan restore PostgreSQL/MMDB
terisolasi, mengenkripsi bundle dengan age, mengunggah ke Bunny S3, lalu membaca
ulang seluruh object untuk memverifikasi checksum ciphertext.
Retention lokal minimum tujuh hari dan tujuh recovery point terverifikasi;
offsite minimum 30 hari. Pruning menunggu backup terbaru lolos verifikasi offsite.
Target RPO sekitar 24 jam ditambah jitter; RTO pemulihan penuh belum diukur.

Secret Bunny/Telegram berada di `/etc/beon-ipquality/operations.env` (0600),
dengan [template tanpa secret](configs/operations.env.example). VPS hanya menyimpan
public recipient age. Private key pemulihan disimpan terpisah dari VPS dan
harus tersedia untuk decrypt backup. Ikuti [panduan backup/restore/rollback](docs/BACKUP_RESTORE_ROLLBACK.md).

`beon-alert-check.timer` memeriksa health, freshness, pipeline, container,
resource, dan umur backup setiap menit. Telegram mengirim incident setelah dua
pemeriksaan gagal berturut-turut, recovery, serta reminder. Monitor eksternal
masih diperlukan untuk mendeteksi VPS atau jaringan yang mati total.

Glances tersedia di loopback port 61208 untuk CPU, RAM, trafik jaringan, dan disk I/O:

```bash
ssh -i /path/key.pem -L 61208:127.0.0.1:61208 ubuntu@VPS_IP
# Buka http://127.0.0.1:61208 pada komputer klien.
```

| Kebutuhan | vCPU ARM64 | RAM | SSD/NVMe |
|---|---:|---:|---:|
| Pilot terkurasi saat ini | 2 | 4 GB | 30 GB |
| Rekomendasi awal produksi satu node | 4 | 8 GB | 80 GB |
| Estimasi untuk dataset/traffic lebih besar | 8 | 16 GB | 160 GB |

Dua rekomendasi terakhir merupakan estimasi dengan headroom, belum benchmark
pada hardware tersebut. Feed jutaan baris masih membutuhkan optimasi dan
pengukuran; FireHOL Anonymous belum aktif karena ingest mengalami OOM pada cap
1 GiB. Peak compiler dataset terkurasi sekitar 492 MiB.

Pilot memakai `REDIS_ENABLED=false` berdasarkan perbandingan singkat loopback;
container Redis tetap tersedia. Hasil tersebut belum mewakili traffic publik,
variasi IP luas, atau beban berkelanjutan. Angka mentah dan batas pengujian ada
di [bukti kapasitas phase 6](docs/evidence/phase6-arm-capacity-2026-10-12.json).

## Pekerjaan berikutnya

1. Siapkan korpus IP berlabel independen untuk mengukur precision, recall, dan
   false positive per kategori; evaluasi threshold dan bobot berdasarkan hasilnya.
   [Evaluator offline dan panduan korpus](docs/SCORING_EVALUATION.md) sudah tersedia.
   Baseline Avastel 500 IP telah dijalankan; dataset positif saja belum cukup
   untuk mengukur precision/false positive.
   Dataset akun 1 juta baris juga sudah diuji pada pipeline staging lokal;
   belum diaktifkan di produksi karena ketentuan penggunaan dan headroom ARM.
2. Uji beban HTTPS berkelanjutan dengan variasi IP, sekaligus ukur resource
   ingest/compiler dan pertumbuhan disk pada dataset yang ditargetkan.
3. Tambahkan uptime monitoring eksternal, lalu lakukan reboot dan full rollback/
   recovery drill untuk mengukur RTO dan memastikan service pulih otomatis.

Gate ini diperlukan sebelum pilot dinyatakan siap produksi penuh. Checklist
pelaksanaan tetap mengikuti [tracker phase perbaikan](docs/PHASE_PERBAIKAN_DAN_VPS_ARM.md).
