# Phase perbaikan dan deployment VPS ARM

Tanggal: 11 Oktober 2026. Dasar: [audit docs dan implementasi](AUDIT_DOCS_IMPLEMENTASI_2026-10-11.md). Semua checkbox adalah pekerjaan yang belum dinyatakan selesai; keberadaan kode/workflow tidak otomatis membuktikan hasil deployment.

## Arah deployment

Target awal adalah satu VPS Linux ARM64 dengan Docker Compose dan Nginx host untuk HTTPS, tanpa aaPanel. Pipeline: feed → PostgreSQL → compiler → volume MMDB → API → Nginx → pengguna. Redis dievaluasi lewat benchmark. Judge tetap opsional dan hanya untuk akses internal. ClickHouse, Grafana, dan stack monitoring lengkap tidak wajib dinyalakan pada pilot pertama.

Monitoring server awal memakai **Glances**, dipasang native sebagai service systemd agar metrik host terbaca tanpa menjalankan panel administrasi. Pantau CPU, RAM/swap, load, traffic RX/TX, disk I/O, kapasitas filesystem, dan proses. Gunakan tampilan terminal atau web pada loopback melalui SSH tunnel. Interval awal 5 detik; ukur overhead pada VPS sebelum menetapkan konfigurasi akhir. Glances mendukung metrik jaringan, disk I/O, dan container; lihat [situs resmi](https://nicolargo.github.io/glances/). Ini pilihan untuk pemantauan langsung, bukan pengganti histori metrik dan alerting aplikasi.

Jika histori dan alert host diperlukan, evaluasi **Netdata Agent standalone** pada phase observability; agent dapat dijalankan gratis dan mendukung AArch64. Batasi retention/collector berdasarkan RAM dan disk. Referensi: [standalone gratis](https://www.netdata.cloud/homelab/), [platform support](https://learn.netdata.cloud/docs/netdata-agent/versions-%26-platforms). Tidak mengasumsikan layanan Netdata Cloud gratis untuk penggunaan komersial.

## Alur dan dependency

Phase 0 → 1 → 2 → 3 → 4 → 5 → 6. Persiapan dokumen backup, monitoring, dan deployment bisa berlangsung selama phase 1–3. Akses VPS baru diperlukan ketika memasuki phase 4. Phase 7 adalah pengembangan lanjutan setelah kebutuhan kapasitas/fitur terukur.

| Phase | Fokus | Perlu VPS? | Kriteria selesai |
| --- | --- | --- | --- |
| 0 | Baseline dan GitHub | Tidak | Revision sumber jelas, akses remote terverifikasi, perubahan lokal terjaga |
| 1 | Perbaikan dasar produksi | Tidak | Limiter/admin/bootstrap benar dan test terkait lulus |
| 2 | Pipeline dan kualitas data | Tidak | Scoring/config/freshness/reload konsisten dan teruji |
| 3 | Pengujian serta paket ARM64 | Tidak | Smoke test, CI, artefak ARM64, dan runbook siap |
| 4 | Persiapan VPS dan monitoring ringan | Ya | Host ARM64 diperiksa, runtime/monitoring siap |
| 5 | Pilot deployment | Ya | Pipeline nyata, HTTPS, restart, backup/restore dan rollback teruji |
| 6 | Observability dan validasi kapasitas | Ya | Beban/overhead terukur, alert operasional dan docs final siap |
| 7 | Fitur lanjutan dan multi-node | Sesuai kebutuhan | Analytics/key pelanggan/distribusi teruji sesuai scope yang dipilih |

## Phase 0 — Baseline dan GitHub

- [x] Inventarisasi perubahan tracked/untracked yang sudah ada; review sebelum commit agar data/secret/artefak lokal tidak ikut.
- [x] Buat branch perbaikan dari baseline yang diketahui dan commit pekerjaan dalam bagian yang bisa direview.
- [x] Verifikasi akses baca/tulis GitHub serta status CI/release remote. Remote lokal sudah menunjuk `git@github.com:afuzapratama/BEON-IPQuality.git`, branch `main`; ini belum sama dengan bukti akses push.
- [x] Tetapkan tracker ini sebagai acuan pekerjaan berikutnya, dengan bukti test/commit untuk setiap status selesai.

## Phase 1 — Perbaikan dasar produksi

- [x] Terapkan trusted proxy terbatas dan uji spoofed forwarding headers; limiter memakai identitas/key sesuai kebutuhan, bukan semua klien proxy berbagi kuota.
- [x] Keluarkan health/metrics dari limiter lookup; uji tetap dapat diakses saat kuota habis.
- [x] Pisahkan kredensial/akses admin dari lookup; reload/clear cache hanya melalui akses internal.
- [x] Bootstrap compiler harus gagal bila tidak menghasilkan artefak MMDB valid; tangani feed gagal dan dataset kosong secara eksplisit.
- [x] Selaraskan konfigurasi Nginx/script domain agar Judge dan monitoring tidak dipublikasikan otomatis.
- [x] Hilangkan kerja ingest/compile awal ganda setelah init, dengan mekanisme yang tetap memverifikasi snapshot valid.
- [x] Tambahkan test perilaku di atas dan jalankan unit test, race, vet, build serta vulnerability scan.

## Phase 2 — Pipeline dan kualitas data

- [x] Hubungkan konfigurasi scoring ke compiler; pastikan bobot feed dan decay diterapkan sesuai kontrak.
- [x] Tentukan klasifikasi ASN dari sumber yang tersedia dan hubungkan modifier ASN; uji perbedaan metadata ASN dan sinyal risiko.
- [x] Tetapkan SLA freshness per feed. Ukur waktu ingest/compile/reload; target proposal <1 jam tidak bisa dijamin hanya dengan compile interval 1 jam.
- [x] Tambahkan versi/checksum snapshot dan waktu sukses ingest/compile; health/status membedakan data tersedia dari data stale.
- [x] Reload hanya pada versi berubah, invalidasi cache konsisten, snapshot lama tetap tersedia bila reload gagal.
- [x] Lengkapi notifikasi reload Judge jika profil Judge dipakai; bila tetap polling, dokumentasikan batas keterlambatannya dan tutup TODO dengan desain yang jelas.
- [x] Batasi waktu operasi cache dan uji fallback Redis lambat/mati; pertahankan lookup MMDB yang valid.
- [x] Tambahkan metrik ingest, compile, freshness, reload dan scanner yang benar-benar di-update.
- [x] Neighborhood analysis masuk phase 7 sampai kebutuhan/akurasi terukur; jangan tetap diklaim sebagai fitur selesai.

## Phase 3 — Pengujian, dokumen, dan paket ARM64

- [x] Tetapkan expiry/retention reputasi historis dan riwayat fetch; uji IP yang hilang dari feed dan refresh parsial.

- [x] Buat integration/smoke test deterministik dengan fixture feed: PostgreSQL → ingestor → compiler → API, whitelist/overlap/IPv6, reload, dan auth.
- [x] Uji integrasi Redis dan kegagalan dependency. Analytics diuji di phase integrasinya.
- [x] Verifikasi cross-build Linux ARM64 semua binary; hasil ELF AArch64 statically linked terkonfirmasi.
- [x] Jalankan workflow release binary matrix `amd64`/`arm64` dan verifikasi archive/checksum; eksekusi pada ARM asli menunggu VPS.
- [x] Siapkan image API/ingestor/compiler/Judge berlabel versi dan digest, publish ARM64 (atau multiarch) lewat CI; validasi base image/dependency tersedia untuk ARM64.
- [x] Siapkan Compose produksi memakai image release, volume persisten, rotasi log, restart policy, parameter resource dan secret dari environment host.
- [x] Siapkan Nginx HTTPS, loopback binding, bootstrap/upgrade dan migrasi schema terkontrol; startup SQL pada volume baru bukan sistem migrasi upgrade.
- [x] Tulis backup PostgreSQL/config/MMDB, retention, salinan di luar VPS, restore terisolasi dan rollback aplikasi+snapshot+schema.
- [x] Siapkan OpenAPI/API docs dan selaraskan proposal: status phase, versi Go aktual, schema INET/CIDR, response, health, startup Railway, dan angka benchmark historis.
- [x] Pastikan CI remote lulus pada revision yang akan dideploy. Catat tag/digest dan checksum artefak.

**Gate VPS:** setelah phase 1–3 siap, informasikan pengguna bahwa akses VPS diperlukan untuk phase 4. Jangan memasang atau mengubah server sebelum akses dan targetnya tersedia.

## Phase 4 — Persiapan VPS ARM dan monitoring ringan

Informasi yang diperlukan saat gate ini: IP/hostname, port SSH, username, lokasi file SSH private key yang bisa dipakai di environment kerja, status sudo, OS/versi, vCPU/RAM/disk, layanan yang sudah berjalan, serta domain API bila tersedia. Kunci privat tidak dicantumkan dalam docs/commit/log.

- [x] Periksa host identity, arsitektur nyata (`aarch64`/ARM64), OS, resource, ruang disk, port dan service yang sudah ada; ARM32 memerlukan penyesuaian paket/build.
- [x] Ukur kebutuhan pipeline pilot: peak ingestor ~63 MiB, compiler ~305 MiB; feed jutaan baris belum muat dan tetap dinonaktifkan. Dataset penuh tetap gate phase 6.
- [x] Periksa Docker Engine/Compose dan reverse proxy host; gunakan Caddy existing agar tidak berebut port 80/443.
- [x] Atur direktori deployment, permission secret, volume persisten, firewall dan akses SSH yang tetap berfungsi.
- [x] Pasang Glances native, service systemd dan akses loopback; verifikasi CPU/RAM/network/disk I/O terbaca serta ukur overhead.
- [ ] Tentukan tujuan backup di luar VPS dan kanal alert dengan pengguna; data ini dibutuhkan untuk melengkapi operasi produksi.

## Phase 5 — Pilot deployment satu instance

- [x] Tarik revision/image ARM64 yang sudah lolos CI; simpan manifest deployment berisi versi/digest.
- [x] Bootstrap PostgreSQL, ingest feed nyata, compile dan validasi MMDB, lalu API. Catat feed gagal/jumlah data/durasi tanpa menganggap angka historis masih berlaku.
- [x] Aktifkan GeoLite2 hanya jika kredensial MaxMind tersedia; Judge tetap dinonaktifkan kecuali diperlukan.
- [x] Uji health, auth benar/salah, single/batch IPv4/IPv6, whitelist, reload, cache fallback dan data freshness.
- [x] Sambungkan domain/HTTPS; uji route admin/metrics/Judge tetap internal dan limiter melalui Nginx.
- [ ] Uji restart/reboot, scheduler tanpa duplikasi, log rotation dan volume tetap tersedia.
- [ ] Jalankan backup pertama, restore pada target terisolasi, serta simulasi rollback revision/snapshot; dokumentasikan hasil dan batas rollback schema.
- [ ] Simpan baseline CPU/RAM/network/disk I/O dari Glances selama ingest/compile/lookup.

Pilot dinyatakan berhasil berdasarkan pemeriksaan ini; layanan produksi belum dianggap siap hanya karena container `running`.

## Phase 6 — Observability, kapasitas, dan penyelesaian produksi

- [ ] Jalankan load-test bertahap, ukur p50/p95/p99, throughput, error rate, peak RAM compiler, disk I/O dan overhead monitoring.
- [ ] Bandingkan dengan/tanpa Redis serta batch; pilih konfigurasi dari hasil pengukuran. Target 100k req/s tetap target hingga dibuktikan pada hardware nyata.
- [ ] Tambahkan alert service down, disk penuh, memory pressure, feed/compile gagal dan data stale; uji notifikasi benar-benar diterima.
- [ ] Jika butuh histori, pilih Netdata standalone yang dibatasi resource atau Prometheus retention terbatas; Grafana/ClickHouse/Loki diaktifkan hanya jika kebutuhan dan resource mendukung.
- [ ] Ukur waktu pemulihan dan kehilangan data dari restore drill; finalkan retention/log/resource/connection pool.
- [ ] Tetapkan runbook insiden, penanggung jawab operasional, jadwal update/backup, dan hasil security review pada revision final.
- [ ] Catat batas kapasitas yang terukur dan semua pengecualian scope sebelum menyatakan siap produksi.

## Phase 7 — Analytics, pelanggan, dan multi-node

Phase ini tidak menghalangi pilot internal. Sebelum menyatakan proposal lengkap, setiap item harus diimplementasikan dan teruji atau secara eksplisit dikeluarkan dari scope.

- [ ] Hubungkan ClickHouse ke lifecycle API/Judge, request/scan logging, identitas key yang di-hash, flush berkala/shutdown, retention dan test integrasi; kehilangan log saat error harus terukur.
- [ ] Jika melayani banyak pelanggan, hubungkan tabel key dengan middleware, scope admin/lookup, rotasi/revoke, kuota dan audit perubahan.
- [ ] Evaluasi neighborhood analysis dan validasi false positive sebelum dipakai scoring produksi.
- [ ] Distribusi MMDB multi-node: satu pipeline pusat, artefak berversi/checksum, transfer sementara, validasi, atomic rename, reload, health, rollback dan cache sesuai versi.
- [ ] Optimasi streaming/agregasi compiler dan paralelisasi batch bila benchmark menunjukkan bottleneck.
- [ ] Kubernetes hanya dipersiapkan jika kebutuhan orkestrasi nyata sudah ada; satu VPS memakai Compose.

## Bukti dan status awal

- Audit lokal sebelumnya: `go test ./...` dan `go vet ./...` lulus; ini belum menguji layanan produksi.
- Akses baca `git ls-remote origin HEAD` berhasil pada 11 Oktober 2026; remote HEAD menunjuk `3be39cea874a2f544e94692b577e780dfbd27f8c`. Akses push dan hasil CI remote belum diverifikasi.
- Dokumen phase ini merupakan rencana pelaksanaan, bukan perbaikan runtime yang sudah selesai.
- Belum ada koneksi atau deployment ke VPS; pengguna akan menyediakan akses ketika dibutuhkan.


## Progres 12 Oktober 2026

Branch lokal `fix/production-readiness` dibuat tanpa membuang perubahan yang sudah ada. Remote GitHub dapat dibaca dan akun aktif memiliki permission ADMIN pada repository; belum ada workflow/run di remote. Workflow lokal belum dipush. Review baseline, commit/push dan CI remote tetap pending.

Implementasi phase 1: `ADMIN_API_KEY` terpisah (kosong menonaktifkan admin); lookup rate limit setelah auth per hash key; health/metrics di luar limiter; `TRUSTED_PROXIES` eksplisit dengan `X-Real-IP` dan validasi IP. Nginx menimpa forwarding header; listener dashboard/Judge/Prometheus/exporter lama bind loopback dan script tidak membuka portnya melalui UFW. Cache management publik ditolak; reload proxy dibatasi loopback. Key lookup lama tidak lagi mengakses cache/reload.

Compiler menolak publish dataset kosong dan mempertahankan snapshot lama. Daemon Compose memakai skip initial ingest/compile setelah memeriksa snapshot bisa dibuka; init tetap melakukan bootstrap. Test mencakup missing snapshot, empty compilation, key admin/lookup, operational endpoint ketika kuota habis, proxy dipercaya/tidak dipercaya, dan override environment. Pengujian restart dan integrasi container nyata tetap phase 3/5; pengujian ini tidak menjamin freshness snapshot.

Validasi pada Go host: unit test, race suite, vet, build, `docker compose config --quiet` dengan secret dummy, `bash -n scripts/setup-domain.sh`, dan `git diff --check` lulus. Go 1.25.3 dan vulnerability scan diperiksa terpisah; hasil final dicatat setelah pemeriksaan selesai. Akses VPS belum diperlukan.


Pemeriksaan keamanan baseline Go 1.25.3 mendeteksi 30 vulnerability ID yang terjangkau pada standard library dan `golang.org/x/text`. Toolchain proyek/Docker dinaikkan ke Go 1.27.2 (rilis stabil yang diverifikasi dari [Go downloads](https://go.dev/dl/?mode=json)); `x/text` diperbarui ke v0.41.0 dan dependency `x/sync` ke v0.22.0. CI memakai govulncheck v1.9.0. Pemindaian ulang dengan toolchain tersebut exit 0, tanpa vulnerability terjangkau. Referensi perbaikan x/text: [GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629). Ini hasil static reachability scan, bukan security audit deployment penuh.


Hasil akhir validasi phase 1 pada Go 1.27.2: `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`, dan govulncheck v1.9.0 lulus. Seluruh checklist implementasi lokal phase 1 selesai; acceptance di container/VPS dan security review produksi tetap phase berikutnya. Phase 0 belum selesai karena review baseline dan commit/push/CI remote masih pending.


## Progres phase 2 — 12 Oktober 2026

Baseline terdahulu termasuk perbaikan phase 1 disimpan pada commit `77a6254`. Data/lock MMDB, artefak build, environment runtime dan private key tidak diikutkan. Branch `fix/production-readiness` menjadi branch review; belum ada perubahan pada main atau deployment VPS.

Scoring kini memakai konfigurasi source override, decay, cap, ASN bonus dan risk threshold (`risky`). GeoLite2 memberi nomor/org ASN; PostgreSQL `asn_info` memberi klasifikasi yang dikenal. Mapping berlaku hanya bila record ASN mencakup seluruh prefix. Sampel bawaan bukan database klasifikasi semua ASN.

Snapshot memakai identitas SHA-256 reputasi/GeoIP/ASN, dan metadata riwayat sukses source dari PostgreSQL. Freshness dinilai per jadwal feed; default compile 15m, polling reload 5m, maksimum umur artefak 1h. Ini anggaran konfigurasi, bukan bukti SLA produksi: durasi antre/storage/compile perlu diukur di phase 6. Legacy MMDB tanpa metadata feed mengungkap status unknown. File yang corrupt atau kehilangan GeoIP/ASN yang sebelumnya berhasil dimuat ditolak saat reload. Satu request membaca generation yang konsisten; cache diberi namespace versi dan timeout, dengan jeda retry satu detik saat dependency gagal.

API dan Judge memakai polling versi/checksum. TODO notifikasi compiler ditutup dengan desain polling yang sama untuk satu VPS; latency publikasi menunggu sampai interval polling ditambah waktu validasi file. Distribusi lintas server tetap phase 7. Metrik ingest/compile/reload/freshness dan scan kini benar-benar di-update; endpoint pipeline tersedia pada jaringan Compose tanpa port publik.

Pengujian integrasi dengan PostgreSQL 17 dan Redis 7 sudah membuktikan ingest/store/history, scoring dan ASN, whitelist, IPv6, cache hit, versi setelah reload, feed outage, kegagalan storage, data stale meski MMDB baru, Redis terputus, serta compiler kosong yang mempertahankan file lama. Uji ini menemukan dan memperbaiki pembentukan prefix IPv6 `.../128/128` serta tag MMDB ASN yang tidak terbaca. Test memakai fixture lokal; feed eksternal, akurasi klasifikasi dan deployment nyata belum diverifikasi.

Tindak lanjut kualitas data: storage saat ini tetap menyimpan reputasi historis dengan decay; penghapusan IP dari feed bukan penghapusan otomatis dari master store. Tentukan kebijakan expiry/retention dan uji refresh parsial di phase 3 sebelum mengklaim sinkronisasi dataset penuh. Riwayat feed juga memerlukan retention operasional. Source yang berubah URL/nama perlu bootstrap ulang agar history tersedia.


Validasi lokal akhir phase 2: `go test -race ./...`, `scripts/test-integration.sh` (PostgreSQL 17/Redis 7), `go vet ./...`, `go build ./cmd/...`, validasi Compose, dan govulncheck v1.9.0 lulus. Scanner melaporkan 0 vulnerability terjangkau; ada temuan pada package/module yang tidak terjangkau kode sekarang. Feed internet dan beban VPS belum diuji.


### GitHub dan validasi ARM64

Phase 0 selesai: perubahan baseline dan phase 2 sudah di-commit/push ke `fix/production-readiness`; [draft PR #1](https://github.com/afuzapratama/BEON-IPQuality/pull/1) tersedia. Main belum diubah. CI GitHub untuk revision runtime `33ae82a52d65edca5264e7fd945cf371054516b4` berhasil, termasuk job verify dan integration: [hasil CI](https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38181345959). Event push juga lulus pada run `38181308893`.

Cross-build `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o /tmp/beon-phase3-arm64/ ./cmd/...` menghasilkan api/ingestor/compiler/judge sebagai ELF AArch64 statically linked dengan Go 1.27.2. Manifest base image `golang:1.27.2-alpine` dan `alpine:3.22` mencantumkan linux/arm64. Ini verifikasi build dan manifest, bukan eksekusi aplikasi/container pada CPU ARM atau image release yang sudah dipublish.

Pekerjaan berikutnya adalah sisa phase 3: kebijakan expiry/retention, kontrak API, image release, Compose produksi, migrasi upgrade, backup/restore/rollback dan runbook VPS. Akses VPS baru diperlukan pada phase 4 setelah paket deployment siap.


## Phase 3 — persiapan release dan operasi

Refresh source sekarang memakai COPY/staging/upsert/expiry dalam satu transaksi PostgreSQL. Snapshot yang mengecil langsung menonaktifkan IP hilang; empty/malformed/failed source ditolak dan transaksi gagal tidak mengubah dataset sebelumnya. TTL source dua freshness budget, expired history 30 hari, fetch history 30 hari dengan latest attempt/success tetap tersimpan. Retention cleanup harian dan scheduled refresh tidak saling overlap. Test PostgreSQL/Redis mencakup refresh mengecil, reappearance, kegagalan storage serta prune history/expired records.

Compose produksi menarik image berversi/digest, memakai bind mount direktori proyek untuk MMDB, readonly filesystem API/ingestor, resource/pool caps yang dapat diubah, restart policy, log rotation dan service migrator berledger/checksum. Cap contoh belum menentukan ukuran VPS; nilai akhir menunggu spesifikasi dan peak RAM data nyata.

Native geoipupdate memakai path file kredensial pilihan pengguna melalui `GEOIP_CONFIG_FILE`, output melalui `MMDB_HOST_DIR`, default `/opt/beon-ipquality/data/mmdb` yang dipasang sebagai `/app/data/mmdb`. Template systemd/env tersedia; mock executable membuktikan path dengan spasi, output routing, serta file kredensial tidak diubah. Package installation, file path nyata, MaxMind download dan timer server baru dijalankan pada phase 4/5.

Bootstrap Compose, rerun migrasi, penolakan checksum berubah, pg_dump bundle, verifikasi MMDB offline dan restore database terisolasi lulus pada fixture container lokal. Readiness PostgreSQL diubah menjadi TCP supaya temporary init server tidak salah dianggap ready. Nginx HTTPS template lulus nginx -t dengan sertifikat fixture. OpenAPI 3.1 lulus validator 0.9.0. Proposal aktif diselaraskan dengan implementasi; proposal 2025 dipindah ke arsip dengan label historis.

Release automation memverifikasi CI terlebih dahulu, membangun archive amd64/arm64 hanya dari support files yang tracked (tanpa credential/MMDB runtime), mempublish image multiarch empat service ke GHCR, memverifikasi manifest dan CLI ARM melalui emulasi, dan menghasilkan immutable deployment-lock serta checksum. Tag prerelease dipakai untuk pilot; main/stable release lama tidak diubah. Hasil publish/CI dan digest akan dicatat setelah workflow berjalan.

Sisa gate phase 3 adalah menjalankan dan memverifikasi workflow release pada revision final. Setelah itu akses VPS diperlukan untuk phase 4: IP/port SSH, username, path SSH key, sudo/OS/resource, domain bila ada, serta path file MaxMind (AccountID/LicenseKey). Offsite backup destination dan akses read:packages bila GHCR memerlukan login juga ditentukan saat itu.


### Phase 3 selesai — release candidate terverifikasi

[Release v1.1.0-rc.1](https://github.com/afuzapratama/BEON-IPQuality/releases/tag/v1.1.0-rc.1) terbit dari revision runtime `614f8c7e5f57a0dd5f263334a2bdc5d09cced9a1`. [Workflow release](https://github.com/afuzapratama/BEON-IPQuality/actions/runs/38184993152) lulus: tiga job verify, binary amd64/arm64, empat image multiarch dengan verifikasi platform dan CLI ARM melalui QEMU, serta publication assets. CI branch juga lulus pada run `38184554513`.

Arsip yang dipublish diunduh ulang; SHA256SUMS, empat ELF binary di tiap arsitektur, dan executable bit updater diverifikasi. Image index API terbaca dari registry tanpa login dan memuat amd64/arm64. Docker service digest dan dependency PostgreSQL/Redis dipin dalam `deployments/releases/v1.1.0-rc.1.env` (salinan deployment-lock release). Executable bit script dicatat eksplisit di Git agar checkout Linux benar; workspace lokal tidak melacak perubahan mode secara otomatis.

Paket phase 3 siap untuk pilot, bukan bukti produksi telah berjalan. Main/draft PR tetap dalam proses review; release candidate dipakai sebagai revision tetap. Native ARM VPS, feed nyata, file/kredensial MaxMind, DNS/HTTPS, offsite backup dan performa/drill data nyata diuji pada phase 4–6.

**Gate phase 4 sekarang tercapai.** Informasi server diperlukan: IP, port SSH, username, path file SSH key yang dapat diakses agent, sudo/OS/vCPU/RAM/disk, domain bila tersedia, dan path file MaxMind (AccountID/LicenseKey). Agent akan menerapkan path kredensial serta direktori MMDB proyek pada server, memasang native updater/timer dan monitoring ringan setelah informasi itu tersedia. Secret tidak perlu dimasukkan ke repository.

### 12 Oktober 2026 — VPS ARM dan temuan pilot

Lihat [catatan pilot](VPS_ARM_PILOT_2026-10-12.md). Akses host, Docker, direktori, reverse proxy existing, Glances loopback dan GeoLite2 native/timer telah diperiksa atau dipasang. Feed FireHOL Anonymous membutuhkan perbaikan bounded-memory sebelum diaktifkan pada host 4 GB; eDROP/SSL blacklist kosong dikecualikan dari pilot. Ingest 22 kelompok feed berhasil. Compiler reserved-network diperbaiki dan mendapat uji regresi; deployment release baru serta gate API/backup masih berjalan.

### 12 Oktober 2026 — pilot aktif

Release v1.1.0-rc.2 dan seluruh CI sukses; domain HTTPS berjalan, native GeoIP
timer dan monitoring aktif, API health/auth/single/batch/whitelist/reload/cache
fallback/limiter teruji. Duplikasi URL datacenter/VPN dibersihkan pada konfigurasi
pilot; 20 kelompok/32 source aktif. Backup akhir serta restore terisolasi lulus,
timer backup aktif. Restart service lulus; reboot host dan rollback seluruh pipeline
belum diuji. API rc.1 teruji membaca snapshot akhir tanpa mengganggu layanan live.
Offsite/alert, dataset penuh, load-test dan akurasi scoring tetap phase 6.
