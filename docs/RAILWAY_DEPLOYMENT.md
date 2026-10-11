# Railway deployment

Railway dapat membangun target `api` dari `Dockerfile` menggunakan `railway.json`. Deployment memerlukan PostgreSQL, Redis, dan MMDB persisten di `/app/data/mmdb/reputation.mmdb`.

API tidak menyertakan snapshot reputasi di image. Ini disengaja agar image tidak membawa data lama. Attach volume di `/app/data`, lalu hasilkan MMDB menggunakan service compiler atau pipeline data terpisah sebelum API dinyalakan. API production keluar saat startup bila MMDB belum tersedia/invalid; siapkan snapshot sebelum menyalakannya. Setelah proses berjalan, `/health` memberikan 503 bila snapshot/source stale atau reader unavailable.

Gunakan variabel dari `railway.env.example`. Yang wajib untuk API production:

```dotenv
APP_ENV=production
API_AUTH_ENABLED=true
API_KEY=<minimal-32-random-characters>
DATABASE_URL=<railway-postgres-url>
REDIS_URL=<railway-redis-url>
REDIS_ENABLED=true
MMDB_PATH=/app/data/mmdb/reputation.mmdb
```

Railway menyediakan `PORT`; aplikasi membacanya sebagai override `server.port`. Jangan simpan API key, URL database, atau URL Redis di repository.

Sesudah deploy, verifikasi:

```bash
curl https://<domain>/health
curl -H "X-API-Key: <key>" https://<domain>/api/v1/check/8.8.8.8
```

Jika health check gagal, periksa keberadaan dan permission MMDB terlebih dahulu. Jika API hidup tetapi cache tidak tersedia, cek `REDIS_URL`; kegagalan Redis menonaktifkan cache tetapi tidak mengubah hasil reputasi.
