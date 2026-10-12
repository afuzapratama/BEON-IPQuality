# Penyimpanan sementara NVMe Azure

Verifikasi 13 Oktober 2026: VPS ARM menggunakan `Standard_D4plds_v6`, 4 vCPU,
RAM ~7,7 GiB. Disk sistem persisten tetap 30 GB (~22 GB bebas). Disk tambahan
220 GB teridentifikasi sebagai `Microsoft NVMe Direct Disk v2`, melalui
`/dev/disk/azure/local/by-name/nvme-220G-1`.

Ini **temporary local NVMe**, bukan managed data disk persisten. Data dapat
hilang saat stop/deallocate, perpindahan host atau event lifecycle lain.
[Microsoft: temporary NVMe FAQ](https://learn.microsoft.com/en-us/azure/virtual-machines/enable-nvme-temp-faqs).
Database produksi, MMDB aktif, secrets, dan backup unik tidak ditempatkan di sini.

## Konfigurasi yang diterapkan

Disk tanpa filesystem/partisi/signature terdeteksi diformat ext4 dengan label
`BEON_SCRATCH`, mounted di `/mnt/beon-scratch`, sekitar 216 GiB usable. UUID saat
ini `2cc231ea-2ab8-43b1-937e-a3fe14bc2bfb`. Mount options:
`nofail,nodev,nosuid,noexec,x-systemd.device-timeout=5s`; filesystem memakai reserved
blocks 0%. Parent mount dan folder staging mode 0700. `/etc/fstab` disalin sebagai
`/etc/fstab.before-beon-scratch` sebelum penambahan. Verifikasi fstab dan baca/tulis
lulus. Konfigurasi boot ditambahkan; reboot belum diuji pada turn ini.

`nofail` memungkinkan OS boot jika disk tidak tersedia. Setelah disk sementara
di-reset Azure, UUID/isi bisa berubah; mount tidak dijamin berhasil dan tidak ada
auto-format. Periksa identitas/signature disk lagi sebelum inisialisasi ulang.
Jangan sekadar menulis ke direktori mount kosong pada disk sistem.

## Penggunaan staging

Gunakan root run baru di `/mnt/beon-scratch/staging/<run-id>`.
`scripts/stage-avastel-arm.py` menolak root scratch bila mount tidak tersedia,
agar staging tidak diam-diam memenuhi disk sistem. MMDB kandidat, clone dump,
source sementara dan PostgreSQL staging bind data berada di root run tersebut.
Docker image/binary tetap pada storage OS; `noexec` melarang eksekusi binary dari
scratch, termasuk binary kandidat bila disimpan di sana.

Container/network staging dibersihkan setelah run. PostgreSQL bind data dan
artifact privat pada direktori run dipertahankan untuk audit sampai dibersihkan
atau disk direklamasi; penghentian container tidak menghapus bind files.
Laporan agregat penting harus disalin ke lokal/repository. Scratch tidak dianggap
sebagai recovery point atau pengganti backup Bunny.

## Status layanan sesudah setup

API rc.3 healthy dan snapshot fresh pada post-check. Saat pemeriksaan sempat
terlihat snapshot stale dari sebelum reboot, sedangkan compiler sudah menulis
snapshot baru; reload admin memuat snapshot baru dan memulihkan health. Kejadian
ini terpisah dari setup filesystem, dan tetap perlu dicakup reboot/freshness drill.
Tidak ada database/MMDB produksi yang dipindahkan ke scratch. Limit compiler
produksi tetap 1,5 GiB; pengujian limit lebih besar merupakan langkah berikutnya.
