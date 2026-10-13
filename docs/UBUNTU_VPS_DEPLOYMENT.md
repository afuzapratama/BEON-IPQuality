# Deployment Ubuntu VPS

Untuk VPS ARM64 langsung tanpa aaPanel, gunakan [panduan VPS ARM64](VPS_ARM_DEPLOYMENT.md) dan [backup/restore/rollback](BACKUP_RESTORE_ROLLBACK.md). Compose produksi menarik image release berdasarkan digest, sedangkan Nginx dan geoipupdate berjalan pada host. Root proyek dan path file kredensial MaxMind disesuaikan setelah akses server tersedia.

Installer native lama `scripts/install-ubuntu.sh` mengelola Nginx dan firewall host; jalur ini tidak dipakai oleh phase perbaikan VPS saat ini. Source/config/secrets perlu ditinjau sebelum instalasi pada server existing. Status pekerjaan dan bukti validasi ada pada [tracker phase](PHASE_PERBAIKAN_DAN_VPS_ARM.md).
