# Backup, restore dan rollback

Tooling memakai Compose produksi, `.env` host dan `deployment-lock.env`. Semua operasi terikat project Compose yang dipilih. Backup lokal tidak mengubah service atau database. Restore-check membuat PostgreSQL sementara tanpa host port/volume, lalu menghapus container tersebut; production tetap tidak disentuh.

## Backup

```bash
cd /opt/beon-ipquality
sudo env BACKUP_DIR=/opt/beon-backups ./scripts/backup.sh
sudo ./scripts/restore-check.sh /opt/beon-backups/<bundle>
```

Akun operasi perlu membaca `.env` dan direktori MMDB milik UID 10001; contoh memakai sudo. Bundle private (umask 077) mencakup pg_dump custom format, config/support files, snapshot MMDB, runtime environment, image lock, checksum dan marker COMPLETE. `GEOIP_CONFIG_FILE=/path/GeoIP.conf` pada invocation backup menambahkan copy private file MaxMind; tanpa variabel itu file MaxMind tidak ikut. Copy credential ini berada dalam backup private, bukan repo/image. Bundle yang terputus sebelum marker COMPLETE tidak dipakai untuk restore.

Dump memakai snapshot transaksi PostgreSQL. MMDB disalin sebagai complete atomic file; publication dapat lebih lama/baru daripada dump. Setelah restore, recompilation dari database menentukan dataset yang authoritative. Cache Redis disposable dan tidak dibackup. GeoLite2 dapat diunduh lagi menggunakan credential yang benar.

Jadwal awal: backup harian, sebelum setiap migrasi/upgrade, dan setelah perubahan kredensial/config. Simpan minimal 7 backup harian lokal dan 30 hari offsite; pertahankan backup pre-migration sampai release stabil serta setidaknya dua revision image/MMDB untuk rollback. Jangan prune satu-satunya backup yang lolos restore-check atau bundle yang belum dipastikan tersalin offsite.

Offsite destination dan metode enkripsi dipilih bersama pengguna pada phase 4. Bundle mengandung secret; encrypt dengan age/GPG atau storage encryption yang tervalidasi sebelum transfer. Tidak ada upload otomatis ke akun/service eksternal dari repo ini. Catat waktu backup, checksum offsite, hasil restore dan waktu pemulihan; uji rutin, bukan hanya mengecek file ada.

## Restore terisolasi

`restore-check.sh` memeriksa SHA256SUMS, memvalidasi MMDB melalui image compiler dari lock, menjalankan pg_restore `--exit-on-error`, dan mengecek ledger migrasi serta tabel reputasi di PostgreSQL sementara. Image compiler harus dapat dipull/read melalui registry atau sudah tersedia lokal. Backup lama dapat valid secara integrity tetapi stale; itu tidak dijadikan alasan mengabaikan readiness policy.

Sebelum restore production, lakukan restore-check, pilih target DB/volume baru, dan backup kondisi saat ini. Restore ke volume baru; tidak menimpa volume live selama validasi. Terapkan credential environment, restore pg_dump dengan `--no-owner --no-acl`, pastikan owner/grants sesuai role aplikasi. Pastikan versi PostgreSQL sesuai major 17 atau target upgrade yang sudah diuji.

Support files dapat dipulihkan dari config.tar.gz; inspect archive sebelum extraction dan jangan overwrite `.env`/credential server tanpa tujuan jelas. Ekstrak MMDB ke direktori staging dengan filesystem yang sama, validasi menggunakan compiler `-verify-mmdb`, lalu atomic rename file. Isi GeoLite2/credential pada target sesuai path yang dipilih. Cache generation otomatis berubah ketika snapshot berbeda.

Jalankan compiler dari DB restore, periksa health/lookup/auth/whitelist/IPv6 dan versi snapshot, lalu arahkan service ke volume target. Prosedur cutover/akses sudo disesuaikan dengan VPS yang sesungguhnya pada phase 5. Jangan memakai snapshot stale untuk mengklaim API sehat.

## Rollback

1. Simpan diagnosis, image lock dan data saat ini; jangan hapus volume live.
2. Jika schema kompatibel, pasang deployment-lock release sebelumnya dan recreate API/worker setelah pemeriksaan migration ledger.
3. Jika schema tidak kompatibel, restore backup pre-migration ke DB/volume baru yang tervalidasi lalu pilih volume tersebut secara eksplisit. Menurunkan versi image saja tidak membatalkan schema.
4. Bila snapshot baru bermasalah, validasi snapshot sebelumnya dan swap MMDB secara atomik. GeoLite2 lama bila perlu juga dipulihkan; reader generation mencakup ketiga file. Polling reload mengambil versi; admin reload mempercepat pemeriksaan.
5. Periksa health, data freshness, lookup, SSL dan metrik. Jika data lama melewati freshness, refresh pipeline sebelum menyatakan pemulihan berhasil.

RPO/RTO adalah hasil drill pada data nyata, bukan janji dari script. Phase 3 menguji backup dan restore pada fixture container; restore/cutover VPS, offsite transfer dan waktu pemulihan produksi diuji pada phase 5/6.

## Operasional phase 6

Bunny S3 dan Telegram telah diaktifkan pada pilot. Baca
[hasil phase 6](PHASE_6_SCORING_BACKUP_CAPACITY.md) untuk policy, retention, manifest,
serta bukti offsite download/decrypt/isolated restore. Bundle final juga memiliki
`operations.env` privat dan `backup.recipient` public key, keduanya masuk checksum.
Private key age hanya berada di komputer pengguna/lokasi recovery terpisah.

Pada pemulihan VPS, sesudah mendekripsi bundle:

```sh
install -d -m 0700 /etc/beon-ipquality
install -m 0600 /private/bundle/operations.env /etc/beon-ipquality/operations.env
install -m 0644 /private/bundle/backup.recipient /etc/beon-ipquality/backup.recipient
# Pulihkan GeoIP.conf dan runtime.env sesuai runbook di atas.
```

Pertahankan public recipient yang cocok dengan recovery key; jangan membuat
recipient baru pada restore jika masih menggunakan kunci pemulihan lama.
Backup local verified minimal tujuh hari/tujuh points, offsite minimal 30 hari;
pruning hanya berjalan setelah offsite readback terbaru sukses. Bundle incomplete
atau belum terverifikasi tidak dihapus otomatis.

API rc.3 pada pilot menggunakan MMDB langsung (REDIS_ENABLED=false) berdasarkan
baseline; root server/env dan manifest backup menentukan konfigurasi restore,
bukan asumsi cache selalu aktif. Metadata health menyatakan scoring policy.
Restore-check memvalidasi data/schema; refresh feeds dan recompile wajib sebelum
menganggap snapshot pulihan yang sudah tua siap melayani request.
