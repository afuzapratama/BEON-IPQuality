# Validasi operasional rc.4 — 13 Oktober 2026

Runtime release [v1.1.0-rc.4](https://github.com/afuzapratama/BEON-IPQuality/releases/tag/v1.1.0-rc.4),
source `6f14784`, manifest `deployments/releases/v1.1.0-rc.4.env`.
[Seluruh 11 job CI/release sukses](https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38318346394).
Host publisher/config/monitor/runbook dilanjutkan pada commit support sesudah tag;
image Go tetap immutable. Runtime rc.3 sebelum deploy disimpan untuk rollback.

## Hasil

- Dataset akun aktif produksi privat, 1.000.000 CSV rows; 999.388 isProxy=True,
  612 isProxy=False dikecualikan. Source age/freshness terlihat pada health.
- API production self-check 100 record teringest: proxy 100, risky 2. Bukan
  precision/recall atau validasi independent accuracy.
- [Probe public HTTPS](evidence/phase6-public-https-rc4-2026-10-13.json): 300,09
  detik, 5 HTTP req/s, 1.500 request semua HTTP 200; zero invalid/error. Cohort
  2.005 IP, single dan batch4 bergantian; p95 single 310,68 ms, batch 311,04 ms.
  Client di luar VPS; quota 1.000/min/key tidak dimatikan. Backup berjalan pada
  sebagian periode uji. Ini stability probe rate terbatas, bukan kapasitas puncak
  atau SLO jangka panjang. Tidak menguji disk growth selama berhari-hari.
- Live image rollback **API/ingestor/compiler rc4→rc3→rc4** berhasil dalam 19 detik
  total, mempertahankan DB/MMDB terbaru (schema sama). HTTP 502 sementara terlihat
  selama pergantian container; deployment ini belum zero downtime. Tidak ada
  downgrade schema atau penggantian database produksi selama drill.
- Restore PostgreSQL/MMDB terisolasi dilakukan oleh backup cycle. Bundle rc4
  `20261012T225515Z-4695` VERIFIED + OFFSITE_VERIFIED, key akun tercakup dan
  full encrypted object readback SHA256 cocok. Target restore bukan DB live.
- [Warm reboot](evidence/phase6-reboot-rc4-2026-10-13.json): SSH kembali 21,31
  detik, public HTTPS healthy 28,67 detik. API/PG/Redis/ingestor/compiler/Caddy/
  existing shortlink agent, Glances, source dan seluruh timer kembali aktif;
  scratch mounted. Bukan deallocation/reimage atau full disaster-recovery RTO.
- Bug form browser IPv6 diperbaiki: colon path dipertahankan setelah URL escaping;
  form flow IPv6 HTTP 200. Reload Caddy memakai `docker exec ... kill -USR1 1`,
  bukan Docker kill API, untuk menghindari penandaan container manually stopped.

## Monitoring eksternal

`.github/workflows/external-uptime.yml` aktif di default branch `main`, tanpa
merge seluruh branch aplikasi. Setiap ~5 menit, runner di luar VPS memeriksa
HTTPS health/freshness. Status disimpan di branch `monitoring-state`, hanya boolean/
timestamp/counter; tidak berisi secret atau raw data akun. Telegram secret memakai
GitHub Actions secrets, tidak dicetak. Dua kegagalan berturut-turut memicu incident,
recovery saat sehat, reminder 6 jam. Workflow concurrency mencegah state race.

Real check sukses, simulasi [TEST] incident dan recovery mendapat acknowledgement:
[incident test](https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38318665286),
[recovery test](https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38318737866),
[scheduled run](https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38319891729).

GitHub schedule best effort, dapat terlambat dan public repository dapat disabled
setelah 60 hari tanpa aktivitas. Review kesehatan workflow berkala; jangan
menganggap monitor ini SLA alert real-time.
[Referensi schedule GitHub](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule).

## Sisa pekerjaan

Operator belum mempunyai label independen; alur [review label](LABEL_REVIEW_WORKFLOW.md)
sudah siap. Accuracy gate tetap terbuka sampai cohort/prediksi holdout tersedia.
Restore Bunny ke host terpisah sudah lulus (129,334 detik setelah decrypt/extract);
lihat [runbook dan evidence](DISASTER_RECOVERY_DRILL.md). RTO end-to-end termasuk
provisioning/DNS/TLS, sustained/high-load SLO dan disk growth masih perlu pengukuran. Analytics/history stack berat tidak diaktifkan pada pilot privat ini.
