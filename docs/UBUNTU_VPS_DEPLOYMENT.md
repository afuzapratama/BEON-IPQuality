# Deployment Ubuntu VPS

Cara yang direkomendasikan adalah Docker Compose seperti di `README.md`. Service internal tidak dipublikasikan, API bind ke loopback, dan Nginx/Caddy dapat menangani TLS di depannya.

## Compose

```bash
git clone https://github.com/afuzapratama/BEON-IPQuality.git
cd BEON-IPQuality
cp .env.example .env
# Isi POSTGRES_PASSWORD, REDIS_PASSWORD, dan API_KEY (minimal 32 karakter).
docker compose up --build -d
docker compose ps
curl http://127.0.0.1:8080/health
```

Gunakan firewall dan reverse proxy HTTPS sebelum membuka API ke internet. Jangan mempublikasikan PostgreSQL, Redis, ClickHouse, atau Judge.

## Installer native

Installer tersedia untuk Ubuntu 22.04/24.04:

```bash
git clone https://github.com/afuzapratama/BEON-IPQuality.git
cd BEON-IPQuality
git log -1 --oneline
sudo bash scripts/install-ubuntu.sh
```

Tinjau commit dan script lokal sebelum menjalankannya sebagai root. Installer membangun binary dari source checkout, melakukan migrasi idempoten, dan tidak lagi menghapus database pada instalasi ulang. Backup PostgreSQL tetap disarankan sebelum upgrade.

## Pemeriksaan operasional

```bash
sudo systemctl status beon-api postgresql redis-server nginx
sudo journalctl -u beon-api -f
curl http://127.0.0.1/health
curl -H "X-API-Key: <key>" http://127.0.0.1/api/v1/check/8.8.8.8
```

Health 503 berarti MMDB belum ada/tidak bisa dibaca. Jalankan ingestor dan compiler, lalu reload API atau tunggu interval reload yang dikonfigurasi.
