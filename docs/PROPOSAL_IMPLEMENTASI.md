# Proposal dan status implementasi BEON-IPQuality

Diperbarui: 12 Oktober 2026. Acuan pekerjaan: [phase perbaikan dan VPS ARM](PHASE_PERBAIKAN_DAN_VPS_ARM.md). [Proposal lama](archive/PROPOSAL_IMPLEMENTASI_2025.md) adalah arsip historis; angka data/benchmark dan klaim progress 100% di dalamnya tidak mewakili kondisi produksi saat ini.

## Tujuan dan arsitektur

Feed → ingestor Go → PostgreSQL dengan INET/CIDR → compiler/scoring → MMDB → API Fiber. Redis opsional pada aplikasi. GeoLite2 City/ASN di-update oleh `geoipupdate` native di VPS, diarahkan ke direktori proyek yang dibaca container melalui bind mount. Target awal satu VPS ARM64 dengan Docker Compose dan reverse proxy HTTPS (pilot memakai Caddy existing; Nginx tersedia sebagai alternatif), tanpa aaPanel. Go mengikuti `go.mod` (saat ini 1.27.2). PostgreSQL memakai pg_trgm; ip4r bukan dependency implementasi.

## Status

| Area | Status |
| --- | --- |
| Baseline GitHub dan perbaikan dasar API | Selesai; branch review/draft PR, belum merge main |
| Ingest/storage/scoring/MMDB/reload/freshness/cache | Teruji fixture PostgreSQL/Redis dan pilot ARM dengan scoring evidence family max |
| Paket ARM64, image release, operasi VPS dan kontrak API | Release rc.3 multiarch, seluruh CI sukses; digest dicatat pada tracker |
| Deployment VPS, GeoIP update nyata dan monitoring server | Pilot aktif; HTTPS, native GeoIP timer dan Glances loopback teruji |
| Load test, kapasitas, alert dan SLA produksi | Baseline privat terukur; Bunny backup/restore dan Telegram aktif; sustained/public load dan SLA belum final |
| ClickHouse request analytics, key pelanggan, distribusi multi-node | Phase 7; bukan fitur produksi selesai |

## Kontrak dan perilaku

Lihat [API.md](API.md) dan [OpenAPI](openapi.yaml). Endpoint readiness kanonik `/health`; lookup `/api/v1/check/:ip` dan batch. Key lookup berbeda dari admin. Snapshot tidak tersedia/stale memberikan 503; IP tanpa sinyal pada dataset valid memberikan clean, bukan jaminan keamanan universal. Caching menggunakan generation reputasi/GeoIP/ASN dan operasi Redis dibatasi waktu.

Scoring memakai policy evidence-family-max-v1: source/feed weight, confidence dan decay menghasilkan kontribusi terkuat per family; infrastructure/ASN maksimum 20. Overlap CIDR digabung dari evidence, bukan sum score atau bonus ASN berulang. Score cap dan threshold tetap dikonfigurasi. ASN type hanya dari klasifikasi yang tersedia di PostgreSQL, bukan tebakan nama organisasi; sample bawaan masih terbatas. Neighborhood analysis belum diterapkan.

Source yang berhasil diperbarui dipublikasi atomik. IP yang hilang dari snapshot source kedaluwarsa; kegagalan refresh tidak merusak snapshot source sebelumnya. TTL sumber dan freshness berbasis jadwal masing-masing, dengan riwayat/audit yang dibatasi retention. Compile default 15m, reload polling 5m; file maksimum 1h. Source harian mempunyai freshness budget lebih dari 24h. Notifikasi push/multi-node belum dibutuhkan untuk pilot satu VPS.

## Target yang masih memerlukan bukti

Latensi p50 <1ms, p99 <5ms, throughput >100k req/s, freshness dan uptime 99,9% adalah target penelitian/operasional, bukan hasil produksi yang sudah dicapai. Ukur latency HTTP, error rate, peak RAM compiler, storage I/O dan pengaruh Redis pada VPS. Jumlah feed/record tergantung hasil download aktual; laporan jutaan record tahun 2025 tidak dipakai sebagai kapasitas sekarang.

Monitoring awal menggunakan Glances untuk CPU/RAM/network/disk I/O. Endpoint Prometheus API/pipeline tersedia; Telegram checker native setiap menit aktif untuk service/freshness/pipeline/host/backup. Histori panjang dan external uptime monitor tetap pekerjaan lanjutan. Lihat [hasil phase 6](PHASE_6_SCORING_BACKUP_CAPACITY.md). Kubernetes, Loki dan stack analytics lengkap bukan prasyarat satu VPS.

## Kriteria sebelum produksi

CI/release revision terverifikasi; bootstrap/migrasi/restart berhasil; lookup/auth/whitelist/IPv6/reload/freshness benar; HTTPS dan route internal terpisah; backup/restore/rollback diuji; credentials tidak masuk repo/image; monitoring dan alert nyata; kapasitas sesuai hardware; runbook dan pemilik operasional tersedia. Semua bukti dicatat pada tracker, bukan diganti dengan persentase progress perkiraan.
