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

### One-Line Install (Ubuntu VPS)

```bash
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
```

This will automatically:
- Install Go 1.23, PostgreSQL 17, Redis 7, Nginx
- Clone and build the project
- Configure all services
- Generate secure credentials

### Post-Install Steps

```bash
# 1. Setup MaxMind GeoIP (get free key at maxmind.com)
cp /opt/beon-ipquality/configs/GeoIP.conf.example /opt/beon-ipquality/configs/GeoIP.conf
nano /opt/beon-ipquality/configs/GeoIP.conf
/opt/beon-ipquality/scripts/update-geoip.sh

# 2. Run initial data ingestion
sudo -u beon /opt/beon-ipquality/bin/ingestor -config /opt/beon-ipquality/configs/config.yaml

# 3. Compile MMDB
sudo -u beon /opt/beon-ipquality/bin/compiler -config /opt/beon-ipquality/configs/config.yaml

# 4. Start the API
sudo systemctl start beon-api && sudo systemctl enable beon-api

# 5. Configure domain (recommended)
sudo /opt/beon-ipquality/scripts/setup-domain.sh --domain api.yourdomain.com --email you@email.com
```

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

**Built with ❤️ using Go**
