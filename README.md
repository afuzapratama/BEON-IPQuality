# 🛡️ BEON-IPQuality

**High-Performance IP Reputation & Proxy Detection System**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## 📖 Overview

BEON-IPQuality adalah sistem reputasi IP dan deteksi proksi berkinerja tinggi yang dibangun menggunakan **Golang**. Sistem ini mampu mendeteksi:

- 🧅 **Tor Exit Nodes** - Jaringan anonimisasi Tor
- 🔒 **VPN/Proxy** - VPN komersial dan proxy publik
- 🏢 **Datacenter IPs** - IP dari penyedia hosting/cloud
- 🤖 **Botnet C2** - Server Command & Control malware
- 🚫 **Blacklisted IPs** - IP dari berbagai blocklist

## ✨ Features

- ⚡ **Ultra-fast queries** (< 1ms latency)
- 📊 **Risk scoring** (0-100) dengan time decay
- 🔄 **Hot reload** tanpa downtime
- 🌐 **REST API** dengan rate limiting
- 📈 **Real-time analytics** dengan ClickHouse
- 🔍 **Active proxy detection** (Judge Node)

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    BEON-IPQuality System                     │
├─────────────────────────────────────────────────────────────┤
│  API Layer    : Go (Fiber) + MMDB Reader                    │
│  Cache        : In-Memory Radix Tree                        │
│  Master DB    : PostgreSQL + ip4r extension                 │
│  Analytics    : ClickHouse                                   │
│  Ingestor     : Go Goroutines + Cron Scheduler              │
│  Judge Node   : Go TCP/HTTP Scanner                          │
└─────────────────────────────────────────────────────────────┘
```

## 🚀 Quick Start

### One-Line Install (Ubuntu VPS) - Recommended

```bash
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
```

#### What Happens During Installation:

1. **Interactive Setup** - You'll be prompted for MaxMind Account ID & License Key
2. **Auto-Install** - Go 1.21+, PostgreSQL 17, Redis 7, Nginx, geoipupdate
3. **Auto-Generate** - All passwords & API keys generated automatically
4. **Auto-Configure** - All services configured and ready to use
5. **Auto-Download** - GeoIP databases downloaded if credentials provided

#### Interactive Prompts:

```
┌─────────────────────────────────────────────────────────────────┐
│  MaxMind GeoLite2 Configuration (Required for GeoIP features)  │
├─────────────────────────────────────────────────────────────────┤
│  Get your FREE Account ID & License Key at:                    │
│  https://www.maxmind.com/en/geolite2/signup                    │
└─────────────────────────────────────────────────────────────────┘

Enter MaxMind Account ID (6-digit number, or 'skip'): 123456
Enter MaxMind License Key: abcdef1234567890
```

> 💡 **Tip**: Type `skip` if you don't have MaxMind credentials yet. You can configure later.

#### After Installation:

```
╔══════════════════════════════════════════════════════════════════╗
║          BEON-IPQuality Installation Complete! 🎉               ║
╚══════════════════════════════════════════════════════════════════╝

🔑 API MASTER KEY (SAVE THIS!)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  a7Bk9mNpQrStUvWxYz12345678901234

  ⚠️  All credentials saved to: /opt/beon-ipquality/credentials.txt
  ⚠️  Environment config at:    /opt/beon-ipquality/.env
```

### Installation Options

```bash
# Interactive install (recommended) - will prompt for MaxMind credentials
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash

# With MaxMind credentials (skip prompt)
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | \
  sudo bash -s -- --maxmind-account "123456" --maxmind-key "YOUR_LICENSE_KEY"

# Fully automated (no prompts at all)
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | \
  sudo bash -s -- --maxmind-account "123456" --maxmind-key "YOUR_KEY" --non-interactive

# Skip MaxMind setup (configure later)
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | \
  sudo bash -s -- --non-interactive
```

### Post-Install Steps

After installation completes, follow these steps **in order**:

```bash
# 1. Verify database tables exist (should show ~5+ tables)
sudo -u postgres psql -d ipquality -c "\dt"

# 2. Run initial data ingestion (downloads threat feeds - takes 2-5 minutes)
sudo -u beon /opt/beon-ipquality/bin/ingestor \
  -config /opt/beon-ipquality/configs/config.yaml \
  -feeds /opt/beon-ipquality/configs/feeds.yaml 2>&1 | tee /tmp/ingestor.log

# 3. Check data was ingested
sudo -u postgres psql -d ipquality -c "SELECT COUNT(*) FROM ip_entries;"

# 4. Compile MMDB database (creates fast-lookup binary file)
sudo -u beon /opt/beon-ipquality/bin/compiler \
  -config /opt/beon-ipquality/configs/config.yaml 2>&1 | tee /tmp/compiler.log

# 5. Start the API server
sudo systemctl start beon-api
sudo systemctl enable beon-api

# 6. Test the API
curl http://localhost:8080/health
curl -H "X-API-Key: YOUR_API_KEY" "http://localhost:8080/api/v1/check?ip=8.8.8.8"

# 7. (Optional) Configure domain with SSL
sudo /opt/beon-ipquality/scripts/setup-domain.sh --domain api.yourdomain.com --email you@email.com
```

> ⚠️ **Important**: If step 2 (ingestor) shows no output for more than 30 seconds, check if database tables exist (step 1). If tables are missing, run the migration manually:
> ```bash
> sudo -u postgres psql -d ipquality -f /opt/beon-ipquality/migrations/001_initial_schema.sql
> ```

### Quick Fix (For Existing Installations)

If you already installed and need to configure MaxMind credentials:

```bash
# 1. Set your MaxMind credentials
sudo tee /opt/beon-ipquality/configs/GeoIP.conf << 'EOF'
AccountID YOUR_ACCOUNT_ID_HERE
LicenseKey YOUR_LICENSE_KEY_HERE
EditionIDs GeoLite2-ASN GeoLite2-City GeoLite2-Country
DatabaseDirectory /opt/beon-ipquality/data/mmdb
EOF

# 2. Download GeoIP databases
sudo -u beon geoipupdate -f /opt/beon-ipquality/configs/GeoIP.conf \
  -d /opt/beon-ipquality/data/mmdb -v

# 3. Run ingestor with correct flags
sudo -u beon /opt/beon-ipquality/bin/ingestor \
  -config /opt/beon-ipquality/configs/config.yaml \
  -feeds /opt/beon-ipquality/configs/feeds.yaml
```

### 🔧 Troubleshooting

#### Error: `'api.rate_limit' expected type 'int'`

This happens when config.yaml format doesn't match the Go struct. Fix by updating config:

```bash
# Backup old config
sudo cp /opt/beon-ipquality/configs/config.yaml /opt/beon-ipquality/configs/config.yaml.bak

# Get your DB password
DB_PASS=$(grep POSTGRES_PASSWORD /opt/beon-ipquality/credentials.txt | cut -d'=' -f2)

# Create correct config format
sudo tee /opt/beon-ipquality/configs/config.yaml << EOF
server:
  host: "127.0.0.1"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

environment: production

logging:
  level: "info"
  format: "json"
  output: "file"
  file_path: "/var/log/beon-ipquality/api.log"

database:
  postgres:
    host: "localhost"
    port: 5432
    database: "ipquality"
    username: "beon"
    password: "$DB_PASS"
    ssl_mode: "disable"
    max_connections: 50
    min_connections: 10
    max_conn_lifetime: 1h
    max_conn_idle_time: 30m

redis:
  enabled: true
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  pool_size: 100

mmdb:
  reputation_path: "/var/lib/beon-ipquality/mmdb/reputation.mmdb"
  geolite2_city_path: "/var/lib/beon-ipquality/mmdb/GeoLite2-City.mmdb"
  geolite2_asn_path: "/var/lib/beon-ipquality/mmdb/GeoLite2-ASN.mmdb"
  output_path: "/var/lib/beon-ipquality/mmdb/reputation.mmdb"
  reload_interval: 1h

scoring:
  decay_lambda: 0.01
  max_score: 100
  risk_threshold: 50

ingestor:
  batch_size: 1000
  workers: 4

api:
  auth_enabled: true
  rate_limit: 1000
  rate_limit_window: 60s
  batch_enabled: true
  batch_max_size: 100

judge:
  enabled: false
  port: 8081

metrics:
  enabled: true

health:
  enabled: true
EOF

# Set permissions
sudo chown beon:beon /opt/beon-ipquality/configs/config.yaml
sudo chmod 640 /opt/beon-ipquality/configs/config.yaml

# Test again
sudo -u beon /opt/beon-ipquality/bin/ingestor \
  -config /opt/beon-ipquality/configs/config.yaml \
  -feeds /opt/beon-ipquality/configs/feeds.yaml
```

#### Error: `no PostgreSQL user name specified`

Database credentials not configured correctly. Check config has nested `database.postgres`:

```yaml
database:
  postgres:           # ← Must be nested!
    host: "localhost"
    username: "beon"  # ← Not 'user'
    password: "xxx"
    database: "ipquality"  # ← Not 'name'
```

#### Error: `permission denied ./configs/feeds.yaml`

Using relative path instead of absolute. Always use full path:

```bash
# Wrong ❌
sudo -u beon /opt/beon-ipquality/bin/ingestor -config ./configs/config.yaml

# Correct ✅
sudo -u beon /opt/beon-ipquality/bin/ingestor \
  -config /opt/beon-ipquality/configs/config.yaml \
  -feeds /opt/beon-ipquality/configs/feeds.yaml
```

#### Clean Reinstall

If you need to start fresh:

```bash
# Stop and remove everything
sudo systemctl stop beon-api beon-judge 2>/dev/null
sudo rm -f /etc/systemd/system/beon-*.service
sudo systemctl daemon-reload
sudo rm -rf /opt/beon-ipquality /var/lib/beon-ipquality /var/log/beon-ipquality
sudo userdel -r beon 2>/dev/null
sudo rm -f /etc/cron.d/beon-ipquality
sudo -u postgres psql -c "DROP DATABASE IF EXISTS ipquality;" 2>/dev/null
sudo -u postgres psql -c "DROP USER IF EXISTS beon;" 2>/dev/null

# Fresh install
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
```

### View Your Credentials

```bash
# All passwords are saved here
sudo cat /opt/beon-ipquality/credentials.txt
```

### Get MaxMind Credentials (Free)

1. Register at [maxmind.com/en/geolite2/signup](https://www.maxmind.com/en/geolite2/signup)
2. After login, your **Account ID** is shown on the dashboard (6-digit number)
3. Go to **Account → Manage License Keys → Generate new license key**
4. Use both Account ID and License Key during installation

> 📝 **Note**: You need BOTH Account ID and License Key. The installer will prompt for both.

### Manual Installation

```bash
# Clone repository
git clone https://github.com/afuzapratama/BEON-IPQuality.git
cd BEON-IPQuality

# Install dependencies
go mod download

# Build binaries
make build

# Start services
make run-api
```

### Using Docker

```bash
# Build and run all services
docker-compose up -d

# View logs
docker-compose logs -f
```

## 📡 API Usage

### Check Single IP

```bash
curl -X GET "http://localhost:8080/api/v1/check/185.220.101.42" \
  -H "X-API-Key: your-api-key"
```

### Response

```json
{
  "ip": "185.220.101.42",
  "score": 87,
  "risk_level": "high",
  "proxy": true,
  "vpn": false,
  "tor": true,
  "datacenter": true,
  "threats": [
    {
      "type": "tor_exit",
      "source": "torproject",
      "confidence": 1.0,
      "last_seen": "2025-12-07T10:30:00Z"
    }
  ],
  "geo": {
    "country": "DE",
    "city": "Frankfurt",
    "asn": 24940,
    "org": "Hetzner Online GmbH"
  },
  "query_time_ms": 0.45
}
```

### Batch Check

```bash
curl -X POST "http://localhost:8080/api/v1/check/batch" \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"ips": ["1.2.3.4", "5.6.7.8"]}'
```

## 📂 Project Structure

```
BEON-IPQuality/
├── cmd/                    # Application entry points
│   ├── api/                # API server
│   ├── ingestor/           # Data ingestion service
│   ├── compiler/           # MMDB compiler
│   └── judge/              # Active proxy scanner
├── internal/               # Private application code
│   ├── api/                # API handlers & middleware
│   ├── config/             # Configuration
│   ├── database/           # Database connections
│   ├── ingestor/           # Ingestion logic
│   ├── mmdb/               # MMDB operations
│   ├── scoring/            # Risk scoring engine
│   └── judge/              # Proxy detection
├── pkg/                    # Public packages
├── migrations/             # Database migrations
├── configs/                # Configuration files
├── deployments/            # Deployment configs
└── docs/                   # Documentation
```

## 🔧 Configuration

```yaml
# configs/config.yaml
server:
  port: 8080
  read_timeout: 5s
  write_timeout: 10s

database:
  postgres:
    host: localhost
    port: 5432
    database: beon_ipquality
    username: postgres
    password: secret

mmdb:
  path: ./data/mmdb/reputation.mmdb
  reload_interval: 1h

scoring:
  decay_lambda: 0.01
  weights:
    spamhaus: 95
    feodo: 90
    firehol: 85
    tor: 70
    datacenter: 50
    proxy_list: 40
```

## 📊 Data Sources

| Source | Type | Update Frequency |
|--------|------|------------------|
| Tor Project | Exit Nodes | Hourly |
| Spamhaus DROP | Hijacked Netblocks | Daily |
| FireHOL | Multi-threat | Real-time |
| Abuse.ch Feodo | Botnet C2 | 5 minutes |
| MaxMind GeoLite2 | Geolocation | Weekly |

## 🧪 Testing

```bash
# Run unit tests
make test

# Run with coverage
make test-coverage

# Run integration tests
make test-integration
```

## 📈 Performance

| Metric | Value |
|--------|-------|
| API Latency (p50) | < 1ms |
| API Latency (p99) | < 5ms |
| Throughput | > 100,000 req/sec |
| Data Freshness | < 1 hour |

## 📜 License

MIT License - see [LICENSE](LICENSE) for details.

## 🤝 Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) first.

---

## 🙏 Credits & Attributions

### Data Sources & Threat Intelligence

This project uses data from the following sources. We are grateful for their contributions to internet security:

| Source | Description | License |
|--------|-------------|---------|
| [MaxMind GeoLite2](https://www.maxmind.com/en/geolite2/signup) | IP Geolocation databases | [GeoLite2 EULA](https://www.maxmind.com/en/geolite2/eula) |
| [Tor Project](https://www.torproject.org/) | Tor exit node list | Public Domain |
| [Spamhaus](https://www.spamhaus.org/) | DROP/EDROP blocklists | Free for non-commercial use |
| [Abuse.ch](https://abuse.ch/) | Feodo Tracker, SSL Blacklist | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |
| [FireHOL](https://github.com/firehol/blocklist-ipsets) | IP blocklist compilation | [GPL v3](https://www.gnu.org/licenses/gpl-3.0.html) |
| [IPsum](https://github.com/stamparm/ipsum) | Daily threat intelligence | [MIT](https://opensource.org/licenses/MIT) |
| [Emerging Threats](https://rules.emergingthreats.net/) | Compromised IP lists | Free for non-commercial use |
| [Blocklist.de](https://www.blocklist.de/) | Attack IP blocklist | Free |
| [CINS Army](https://cinsscore.com/) | CI Army bad IP list | Free |
| [Greensnow](https://greensnow.co/) | Blocklist | Free |

### Technologies & Libraries

| Technology | Description | License |
|------------|-------------|---------|
| [Go](https://golang.org/) | Programming language | [BSD 3-Clause](https://opensource.org/licenses/BSD-3-Clause) |
| [Fiber](https://github.com/gofiber/fiber) | Web framework | [MIT](https://opensource.org/licenses/MIT) |
| [MaxMind DB](https://github.com/maxmind/mmdbwriter) | MMDB writer library | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |
| [oschwald/maxminddb-golang](https://github.com/oschwald/maxminddb-golang) | MMDB reader | [ISC](https://opensource.org/licenses/ISC) |
| [PostgreSQL](https://www.postgresql.org/) | Database | [PostgreSQL License](https://www.postgresql.org/about/licence/) |
| [Redis](https://redis.io/) | Cache & rate limiting | [BSD 3-Clause](https://opensource.org/licenses/BSD-3-Clause) |
| [ClickHouse](https://clickhouse.com/) | Analytics database | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |
| [Prometheus](https://prometheus.io/) | Monitoring | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |
| [Grafana](https://grafana.com/) | Visualization | [AGPL v3](https://www.gnu.org/licenses/agpl-3.0.html) |
| [Nginx](https://nginx.org/) | Reverse proxy | [BSD 2-Clause](https://opensource.org/licenses/BSD-2-Clause) |

### Inspiration

This project was inspired by:
- [IPQualityScore](https://www.ipqualityscore.com/) - Commercial IP reputation service
- [Proxycheck.io](https://proxycheck.io/) - Proxy detection API
- [AbuseIPDB](https://www.abuseipdb.com/) - Community IP abuse database

### Special Thanks

- **MaxMind** for providing free GeoLite2 databases
- **Tor Project** for maintaining public exit node lists  
- **Abuse.ch** for their commitment to free threat intelligence
- **FireHOL** for aggregating multiple blocklists
- The open-source community for amazing tools and libraries

---

## ⚠️ Disclaimer

This software is provided "as is" without warranty. The threat intelligence data is sourced from third parties and may contain false positives. Always verify results and use responsibly.

**MaxMind Attribution**: This product includes GeoLite2 data created by MaxMind, available from [https://www.maxmind.com](https://www.maxmind.com).

---

**Built with ❤️ in Indonesia 🇮🇩**
