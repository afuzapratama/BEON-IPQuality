# Kontrak API

Spesifikasi mesin: [OpenAPI 3.1](openapi.yaml). Kontrak ini menggambarkan API dengan path default, autentikasi production, dan batch diaktifkan.

## Akses dan autentikasi

| Route | Key | Akses VPS |
| --- | --- | --- |
| `/api/v1/check/:ip`, `/api/v1/check/batch` | `X-API-Key: API_KEY` | HTTPS publik + API key |
| `/api/v1/stats` | `API_KEY` | HTTPS publik + API key |
| `/api/v1/cache/stats`, `DELETE /api/v1/cache`, `POST /api/v1/reload` | `X-API-Key: ADMIN_API_KEY` | HTTPS publik + admin key terpisah |
| `/health` | Tanpa key | HTTPS publik untuk readiness/uptime |
| `/metrics` | Tanpa key internal | Hanya jaringan internal; HTTPS publik 403 |
| `/grafana/` | Login Grafana | HTTPS publik dengan autentikasi operator |

Key admin kosong menonaktifkan route admin dengan 404 text/plain. Key lookup tidak dapat menjalankan operasi admin. Limiter lookup menggunakan hash key setelah autentikasi; default 1.000 request/menit, configurable. Health/metrics tidak memakai kuota lookup. Nilai `API_KEY` tidak boleh ditanam di frontend browser.

## Lookup satu IP

```bash
curl -H "X-API-Key: ${API_KEY}" https://ipq.example.com/api/v1/check/8.8.8.8
```

Hanya alamat publik yang diterima. Private, loopback, link-local, multicast, reserved, dan IP yang tidak valid menghasilkan 400. IPv4-mapped IPv6 dinormalisasi menjadi IPv4.

Respons berhasil memuat `ip`, `score`, `risk_score`, `risk_level`, `risky`, flag ancaman, `query_time_ms`, `cached`, dan `data_version`. `score`/`risk_score` berada pada 0–100. Level: clean <25, low 25–49, medium 50–69, high 70–84, critical >=85. `risky` mengikuti threshold scoring konfigurasi; ini terpisah dari nama severity. Whitelist memberikan skor nol, risky false, dan flag ancaman false. Geo/ASN dan daftar sumber/threat types opsional. Field legacy `threats` belum berisi rincian per-source dari compiler; gunakan `threat_types` dan `sources`.

IP yang tidak ditemukan pada snapshot valid mendapat score 0 / clean, yang berarti tidak ada sinyal dalam dataset ini. Ini bukan jaminan IP aman. Snapshot/source yang tidak tersedia atau stale menghasilkan 503, termasuk jika cache masih memiliki hasil lama.

`data_version` adalah identitas SHA-256 gabungan file reputasi/GeoIP/ASN. Cached response memakai generation yang sama. `query_time_ms` mengukur lookup/cache sampai hasil diperoleh, tidak mencakup seluruh HTTP transfer maupun semua waktu penyimpanan cache; ukur latency HTTP end-to-end saat benchmark.

## Batch

```bash
curl -X POST -H "X-API-Key: ${API_KEY}" -H 'Content-Type: application/json' \
  -d '{"ips":["8.8.8.8","1.1.1.1"]}' https://ipq.example.com/api/v1/check/batch
```

Alamat private/loopback dan alamat yang tidak cocok untuk reputation check ditolak dengan 400 pada lookup single. Whitelist diuji melalui alamat publik yang valid.

Batas default 100 IP, sesuai `api.batch_max_size`. Input kosong, JSON salah, atau melebihi batas menghasilkan 400. Urutan hasil mengikuti input. Item invalid memakai `score: -1`, `risk_level: error` atau `invalid`; `risk_score` pada item tersebut tetap default 0 dan bukan alias skor yang dapat dipakai untuk keputusan. Jika lookup IP valid gagal karena reputasi unavailable/stale, seluruh batch mengembalikan 503. Batch diproses berurutan.

## Health, statistik, dan administrasi

- `/health`: 200 jika reader siap dan snapshot/source dalam anggaran, 503 bila unavailable/stale. JSON mencakup status, versi aplikasi, uptime, timestamp, services dan snapshot. Snapshot memuat scoring_policy (jika tersedia), build time/age, data version, freshness source, dan `feed_freshness_known`. Metadata unknown pada snapshot legacy tidak membuktikan feed fresh.
- `/metrics`: text format Prometheus. Path dapat diubah lewat konfigurasi.
- `/api/v1/stats`: total request/IP, rata-rata response time dan error rate sejak proses hidup. Counter reset saat restart; bukan billing analytics.
- `/api/v1/cache/stats`: `enabled: false` jika caching off; jika aktif mengembalikan stats. Memory field saat ini belum diisi dan bernilai 0, bukan ukuran Redis yang terukur.
- `DELETE /api/v1/cache`: menghapus namespace cache aplikasi. Cache disabled menghasilkan 200 dengan `success: false`.
- `POST /api/v1/reload`: tanpa body. Memeriksa database; `changed: false` bila checksum sama. Sukses mengembalikan data version; gagal menghasilkan 500 dan mempertahankan generation sebelumnya. Cache mengikuti versi, tanpa mass delete otomatis.

## Error

| HTTP | Kondisi |
| --- | --- |
| 400 | Invalid IP/body, batch kosong/terlalu besar, konfigurasi reload belum tersedia |
| 401 | Lookup/admin key hilang atau salah |
| 404 | Route tidak ditemukan, admin disabled, atau route internal ditolak Nginx publik |
| 429 | Kuota lookup habis |
| 500 | Operasi admin cache/reload gagal |
| 503 | MMDB unavailable, snapshot/source stale |

Mayoritas error aplikasi berbentuk `{"error":"code","message":"detail"}`. Error admin dapat memakai `error` berupa kalimat dan `success: false`; 404 admin disabled memakai text/plain. Nginx dapat memberikan error sebelum request mencapai aplikasi. `/api/v1/health` bukan path default yang didukung.

## Dataset akun privat dan waktu pembaruan

Runtime VPS saat ini rc.8, source akun Avastel aktif pada API berkey. Snapshot
reputasi tetap lokal; API lookup tidak memanggil Avastel per request. Publisher
memperbarui dataset setiap 7 jam dan sesudah boot untuk recovery. Hanya row
isProxy=True masuk source proxy; metadata ASN/GeoIP tetap dari pipeline yang
ada. `sources` dapat menampilkan `Avastel Account/normalized_account`.

Weight 40/confidence 0,8 adalah baseline heuristic, bukan accuracy 80%. Flag
proxy berbeda dari risky; proxy-only biasanya score 32, evidence family lain
dapat menaikkan skor. Uji 100 record teringest adalah integration, bukan holdout.
[Runbook akun](AVASTEL_PRIVATE_PRODUCTION.md) dan
[alur review label](LABEL_REVIEW_WORKFLOW.md).

## Kebijakan akses produksi — 13 Oktober 2026

Route lookup/stats aktif melalui HTTPS dengan API key, cache/reload dengan admin key. `/metrics` kini internal: HTTPS publik mengembalikan 403, sedangkan Prometheus mengambil langsung `api:8080/metrics`. Grafana ada di `/grafana/` dengan login operator. `/health` tetap publik untuk readiness dan uptime eksternal. Tabel di README menjadi ringkasan akses produksi terbaru; catatan akses metrics publik sebelumnya adalah kondisi historis.

### Halaman status produksi

Pada reverse proxy Caddy produksi, `GET /health` dengan `Accept: text/html` menampilkan halaman status ringan tanpa dependensi eksternal. Halaman mengambil `/health?format=json` setiap 30 detik, menampilkan komponen yang dilaporkan API, uptime proses, versi, snapshot, dan freshness tiap sumber dalam WIB. Pencarian dan filter membantu menemukan source stale atau fetch gagal. `Accept: application/json`, request tanpa Accept HTML, dan `?format=json` tetap mendapat kontrak JSON asli beserta HTTP 200/503. Shell HTML memakai HTTP 200; kondisi sebenarnya ditampilkan dari respons JSON. Halaman tidak mengukur availability historis atau menjamin status container yang tidak dilaporkan health API. Grafana tetap memerlukan login.

Sumber tampilan: `web/health.html`, di-embed dengan `python3 scripts/render-health-page.py` sebagai respons inline pada `configs/caddy/beon-production.caddyfile` untuk deployment tanpa mount atau paket tambahan. Nginx dan akses API langsung tetap memakai JSON.
