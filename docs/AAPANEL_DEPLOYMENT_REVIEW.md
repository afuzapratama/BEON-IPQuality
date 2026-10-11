# Review dan deployment Ubuntu + aaPanel

Review: 12 September 2026, berdasarkan working tree lokal termasuk perubahan yang belum di-commit. Dokumen ini merupakan hasil audit dan panduan; belum ada deployment ke VPS atau perubahan kode runtime.

## Kesimpulan arsitektur

Pertahankan alur feed → PostgreSQL → compiler → MMDB → API. API saat ini tidak membuka koneksi PostgreSQL untuk lookup; Redis opsional di kode. Compose standar masih mewajibkan Redis. Judge, GeoIP updater, dan monitoring sudah dipisahkan melalui profil opsional.

Pilih pola berdasarkan kebutuhan:

| Kebutuhan | Pola yang disarankan |
| --- | --- |
| Banyak website milik sendiri, dataset sama | Satu API pusat yang dipanggil backend website |
| Banyak node untuk kapasitas/region, dataset sama | Satu pipeline ingest/compiler, distribusi MMDB ke node API |
| Pelanggan atau dataset harus terisolasi | Stack terpisah, project name, port, kredensial dan volume berbeda |

Distribusi MMDB lintas server belum diimplementasikan. Targetnya: artefak berversi, checksum, transfer ke file sementara, validasi, atomic rename pada filesystem yang sama, reload, health check, dan rollback. Mount direktori MMDB agar penggantian file terbaca container. Jangan menggandakan ingestor/compiler untuk setiap replica API. Satu pipeline pusat juga memerlukan backup dan pemulihan; node API dapat tetap memakai snapshot terakhir saat pipeline terganggu, dengan pemantauan umur data.

## Temuan prioritas

1. **Installer native tidak cocok langsung untuk server aaPanel.** `scripts/install-ubuntu.sh` menulis `/etc/nginx/sites-available`, menghapus default site, me-restart Nginx, serta mengaktifkan UFW. `scripts/setup-domain.sh` juga mengelola Nginx, sertifikat, dan port monitoring/judge. Gunakan Compose dan Nginx yang dikelola aaPanel untuk jalur ini.
2. **Rate limit di belakang proxy perlu diperbaiki sebelum trafik besar.** `cmd/api/main.go` memakai `c.IP()` tetapi belum mengatur trusted proxy/ProxyHeader. Klien yang melewati proxy yang sama bisa berbagi kuota 1.000 request/menit. Limiter juga mencakup health dan metrics. Perbaikannya: percayai hanya proxy yang diketahui, baca IP yang ditetapkan proxy tersebut, pisahkan health/metrics dari limiter, dan uji spoofed headers. Mengirim `X-Forwarded-For` dari Nginx saja belum memperbaiki kode ini.
3. **API key belum dipisahkan berdasarkan peran/pelanggan.** Satu key mengakses lookup sekaligus reload dan clear cache. Batasi route publik ke lookup; jangan menaruh master key di JavaScript browser. Untuk layanan banyak pelanggan perlu key per pelanggan, rotasi, pencabutan dan kuota. Jangan menganggap tabel API key di database berarti fitur tersebut sudah terhubung ke middleware.
4. **Bootstrap bisa sukses tanpa menghasilkan MMDB.** `internal/compiler/compiler.go` mengembalikan nil ketika tidak ada reputasi maupun whitelist. Compose menganggap compiler-init selesai sukses, tetapi API production gagal membuka file yang belum ada. Perlu status gagal eksplisit pada bootstrap kosong dan pemeriksaan artefak yang dihasilkan.
5. **Startup mengulang pekerjaan.** Setelah init ingest/compile, service ingestor dan compiler reguler menjalankan pekerjaan awal lagi. Pisahkan bootstrap dari scheduler atau tambahkan mekanisme skip awal yang memeriksa snapshot valid.
6. **Compiler menampung dataset di RAM.** Data query, map agregasi, entry MMDB dan tree penulis hidup dalam satu proses. Ukur peak RSS pada dataset nyata; lanjutkan optimasi streaming/agregasi bila terbukti menjadi bottleneck. Pool PostgreSQL default 100 maksimum/10 minimum per proses perlu disesuaikan dengan jumlah worker dan batas server, bukan dinaikkan otomatis.
7. **Redis belum tentu mempercepat lookup lokal.** Bandingkan p50/p95/p99, throughput dan error rate dengan/tanpa Redis. Cache sekarang melakukan JSON/network I/O, batch diproses berurutan, dan operasi cache memakai background context. Prioritaskan batas waktu cache yang pendek dan fallback saat Redis lambat. Jangan berbagi Redis DB antardataset karena prefix saat ini sama-sama `ipq:`.
8. **Pembaruan perlu identitas versi data.** API reload tiap satu jam dan menghapus cache meskipun file belum berubah. Usulkan reload hanya saat versi berubah dan cache key mengandung versi MMDB agar rollout banyak node tidak mencampur hasil snapshot berbeda. Health saat ini memeriksa ketersediaan reader, bukan batas umur feed.
9. **Deploy massal masih build per server.** Compose menggunakan `build:`; workflow release menghasilkan binary archive, belum publish image container. Langkah berikutnya adalah CI membangun image berversi sekali dan server menarik tag/digest yang sama. Rapikan komentar TODO dan opsi konfigurasi yang belum terhubung; contoh compiler masih menggunakan `scoring.NewDefault()`.

## Setup satu instance dengan Compose yang ada

Asumsi: aaPanel memakai Nginx pada host Ubuntu yang sama dengan Docker. Kalau Nginx ada di container/server lain, `127.0.0.1` menunjuk lingkungan tersebut dan konfigurasi jaringan perlu disesuaikan.

### 1. Persiapan

Siapkan domain, checkout source yang sudah ditinjau, dan Docker Engine beserta plugin Compose. Periksa dahulu agar tidak memasang ulang Docker yang sudah dipakai workload lain:

```bash
docker version
docker compose version
```

Jika belum terpasang, ikuti [instalasi resmi Docker untuk Ubuntu](https://docs.docker.com/engine/install/ubuntu/) dan [plugin Compose](https://docs.docker.com/compose/install/linux/). Gunakan akun yang memang mempunyai akses Docker atau tambahkan `sudo` pada perintah Docker. Tidak perlu Go di host untuk build melalui Dockerfile.

Letakkan checkout misalnya `/opt/beon-ipquality/client01`, di luar document root publik. Salin working tree yang sudah ditinjau atau commit terlebih dahulu: clone repository hanya membawa perubahan yang sudah di-commit dan tersedia di remote.

### 2. Konfigurasi instance

Di terminal Bash pada server, untuk checkout baru yang belum memiliki `.env`:

```bash
cd /opt/beon-ipquality/client01
cp -n .env.example .env
chmod 600 .env
nano .env
```

Isi `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, dan `API_KEY` dengan tiga nilai berbeda. Generate masing-masing menggunakan `openssl rand -hex 32`. Simpan key sebagai secret; jangan masukkan ke repository.

Pastikan juga:

```dotenv
COMPOSE_PROJECT_NAME=beon-client01
API_BIND_ADDRESS=127.0.0.1
API_PORT=18080
LOG_LEVEL=info
```

`COMPOSE_PROJECT_NAME` mengoverride `name: beon-ipquality` pada file Compose; alternatifnya konsisten gunakan `docker compose -p beon-client01 ...`. Jangan berganti nama setelah volume produksi dibuat karena akan menunjuk stack/volume lain. Lihat [prioritas project name Docker](https://docs.docker.com/compose/how-tos/project-name/).

GeoLite2 opsional untuk metadata lokasi/ASN; lookup reputasi membutuhkan `reputation.mmdb` dari compiler. Untuk awal biarkan profil opsional tidak aktif.

### 3. Jalankan dan periksa

```bash
docker compose config --quiet
docker compose up --build -d
docker compose ps -a
docker compose logs --tail=100 ingestor-init compiler-init api
curl -i http://127.0.0.1:18080/health
```

Startup pertama menunggu PostgreSQL, ingest feed, dan kompilasi. `ingestor-init` dan `compiler-init` berstatus `Exited (0)` setelah selesai merupakan kondisi yang diharapkan. API harus memberikan health 200 sebelum dihubungkan ke domain. Jika gagal, periksa log; jangan menganggap `up -d` saja membuktikan deployment berhasil.

Uji lookup tanpa menulis key literal di shell history:

```bash
read -r -s -p 'API key: ' BEON_TEST_KEY
echo
curl -i -H "X-API-Key: ${BEON_TEST_KEY}" \
  http://127.0.0.1:18080/api/v1/check/8.8.8.8
unset BEON_TEST_KEY
```

### 4. Domain dan HTTPS di aaPanel

1. Arahkan DNS A domain, misalnya `ipq.example.com`, ke IP publik server. Tambahkan AAAA hanya jika IPv6 memang siap.
2. Buka Website → Proxy Project pada aaPanel Nginx. Buat domain tersebut dan proxy `/` ke `http://127.0.0.1:18080`. Pada versi lain, gunakan menu Reverse Proxy dalam pengaturan website.
3. Matikan proxy cache untuk API. Pertahankan header `X-API-Key` dari permintaan, jangan menanam master key di proxy publik.
4. Melalui pengaturan SSL website, terbitkan sertifikat Let's Encrypt, lalu aktifkan pengalihan HTTPS. Pastikan metode validasi dan pembaruan sertifikat berfungsi.

Pengaturan menu, proxy lokal dan SSL mengacu pada [dokumentasi resmi Proxy Project aaPanel](https://www.aapanel.com/docs/Function/proxy.html).

Untuk domain API publik, batasi proxy pada path lookup. Contoh di bawah **menggantikan** `location /` proxy yang dibuat panel dan ditambahkan dalam blok `server` site; jangan menambahkan dua `location /`. Pertahankan konfigurasi sertifikat serta location validasi ACME milik panel. Terapkan pembatasan pada semua server block yang melayani domain dan uji konfigurasi dari aaPanel sebelum reload:

```nginx
location ^~ /api/v1/check/ {
    proxy_pass http://127.0.0.1:18080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_connect_timeout 5s;
    proxy_read_timeout 30s;
    proxy_cache off;
}

location / {
    return 404;
}
```

Contoh mengasumsikan klien langsung mengakses Nginx, tanpa CDN tambahan. `/health`, `/metrics`, `/api/v1/reload`, dan cache management tetap diakses lewat loopback/SSH. Header IP di atas belum menyelesaikan limiter aplikasi yang dijelaskan pada temuan nomor 2.

Uji ulang lookup melalui HTTPS dengan key (200), tanpa key (401), serta `/api/v1/reload` dan `/metrics` publik (404). Port API tetap loopback. Untuk akses publik aplikasi cukup HTTP/HTTPS; pertahankan akses SSH dan port panel sesuai konfigurasi yang sudah ada. Jangan menjalankan installer native atau script domain lama pada jalur ini.

### 5. Operasional

```bash
docker compose ps -a
docker compose logs --tail=100 api compiler ingestor
docker stats --no-stream
```

Compose menjalankan scheduler ingestor/compiler sendiri; jangan menambahkan cron aaPanel yang menjalankan pekerjaan sama. Backup PostgreSQL, `.env`, konfigurasi feed/scoring, serta snapshot MMDB terakhir yang valid. Simpan salinan di luar VPS dan uji restore. Atur rotasi log Docker agar disk tidak dipenuhi access log.

Migrasi SQL pada `/docker-entrypoint-initdb.d` hanya diterapkan saat volume PostgreSQL baru diinisialisasi. Perubahan schema berikutnya membutuhkan prosedur migrasi tersendiri. Untuk update aplikasi, gunakan revision yang sudah diuji, backup sebelum perubahan schema, lalu build/recreate dan uji health serta lookup. Siapkan revision dan MMDB sebelumnya untuk rollback; rollback binary tidak otomatis mengembalikan schema database. Jangan memakai `docker compose down -v` untuk update karena menghapus volume data.

## Banyak instance terisolasi

Ulangi setup pada direktori lain dengan secret unik:

| Direktori | COMPOSE_PROJECT_NAME | API_PORT | Domain contoh |
| --- | --- | --- | --- |
| `/opt/beon-ipquality/client01` | `beon-client01` | `18080` | `ipq-a.example.com` |
| `/opt/beon-ipquality/client02` | `beon-client02` | `18081` | `ipq-b.example.com` |

Compose membedakan volume dan network berdasarkan project. Port database/Redis tidak dipublikasikan oleh Compose ini, sehingga nomor port internal boleh sama. Bila profil monitoring/judge diaktifkan pada host yang sama, port publik loopback masing-masing juga harus dibedakan. Set secret monitoring sendiri sebelum mengaktifkan profil. Untuk GeoIP, tambahkan `MAXMIND_ACCOUNT_ID` dan key valid; account ID belum dicantumkan di `.env.example` sekarang.

Setiap stack penuh menggandakan database, Redis dan pipeline. Untuk banyak website dengan dataset sama, gunakan satu API pusat dahulu. Ukuran VPS dan jumlah instance yang aman harus ditentukan dari dataset, interval compile, peak RAM, disk, serta trafik nyata; belum ada benchmark yang mendukung angka kapasitas tertentu.

## Validasi review

- `go test ./...`: lulus pada environment lokal; tidak membuktikan integrasi PostgreSQL/Redis maupun akses feed nyata.
- `go vet ./...`: lulus.
- `go build ./cmd/...`: lulus.
- `docker compose config --quiet`: lulus menggunakan secret dummy khusus validasi.
- Deployment container penuh, penerapan Nginx aaPanel, load test, dan restore backup belum dijalankan.

Urutan tindak lanjut: benahi limiter/proxy dan bootstrap kosong, pilot satu instance, ukur beban, lalu otomatisasi image release dan distribusi MMDB sesuai kebutuhan jumlah server.
