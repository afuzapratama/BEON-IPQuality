# 🛡️ BEON-IPQuality

**High-Performance IP Reputation & Proxy Detection System**

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## 📖 Overview

BEON-IPQuality adalah sistem reputasi IP dan deteksi proxy berkinerja tinggi. Sistem ini mampu mendeteksi:

- 🧅 **Tor Exit Nodes** - Jaringan anonimisasi Tor
- 🔒 **VPN/Proxy** - VPN komersial dan proxy publik
- 🏢 **Datacenter IPs** - IP dari penyedia hosting/cloud
- 🤖 **Botnet C2** - Server Command & Control malware
- 🚫 **Blacklisted IPs** - IP dari berbagai threat intelligence feeds

## ✨ Features

- ⚡ **Ultra-fast queries** (< 1ms latency)
- 📊 **Risk scoring** (0-100) dengan kategorisasi
- 🔄 **Auto-update** threat feeds setiap 4 jam
- 🌐 **REST API** dengan API key authentication
- 📈 **1.6M+ IP entries** dari 21 threat feeds
- 🔒 **Rate limiting** built-in

---

## 🚀 Quick Start Installation

### One-Line Install (Ubuntu 22.04/24.04 LTS)

```bash
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
```

### What Gets Installed

| Component | Version | Purpose |
|-----------|---------|---------|
| PostgreSQL | 17 | Primary database |
| Redis | 7 | Caching layer |
| Nginx | Latest | Reverse proxy |
| Go | 1.25 | Runtime (for compilation) |

### Installation Process (~2 minutes)

The installer will:

1. ✅ Update system packages
2. ✅ Install Go 1.25.3
3. ✅ Install & configure PostgreSQL 17
4. ✅ Install & configure Redis 7
5. ✅ Install & configure Nginx
6. ✅ Create `beon` system user
7. ✅ Clone repository & download pre-built binaries
8. ✅ Run database migrations (8 tables)
9. ✅ Create configuration files
10. ✅ Setup systemd services
11. ✅ Configure firewall (UFW)
12. ✅ **Ingest 1.6M+ threat IPs** from 21 feeds

### Interactive Prompts

During installation, you'll be asked for MaxMind credentials (optional):

```
┌─────────────────────────────────────────────────────────────────┐
│  MaxMind GeoLite2 Configuration (Optional - for GeoIP features)│
├─────────────────────────────────────────────────────────────────┤
│  Get your FREE Account ID & License Key at:                    │
│  https://www.maxmind.com/en/geolite2/signup                    │
└─────────────────────────────────────────────────────────────────┘

Enter MaxMind Account ID (or 'skip'): 
```

> 💡 Type `skip` if you don't have MaxMind credentials.

---

## 📋 Post-Installation

After installation completes, you'll see:

```
╔══════════════════════════════════════════════════════════════════╗
║              🎉 INSTALLATION COMPLETE! 🎉                        ║
║              Total time: ~2 minutes                              ║
╚══════════════════════════════════════════════════════════════════╝

🔑 YOUR API KEY (SAVE THIS!)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  NrA3pia8Bb2TATLPxlTp2NJWgahwnGPb

📊 SERVICE STATUS
━━━━━━━━━━━━━━━━━
  PostgreSQL: active
  Redis:      active
  Nginx:      active
  Database:   1,636,197 IP entries
```

### Step 1: Start API Server

```bash
sudo systemctl start beon-api
sudo systemctl enable beon-api
```

### Step 2: Verify Services

```bash
# Check all services are running
sudo systemctl status beon-api
sudo systemctl status postgresql
sudo systemctl status redis
sudo systemctl status nginx
```

### Step 3: Test the API

```bash
# Health check (no auth required)
curl http://localhost/health
```

**Expected response:**
```json
{"status":"healthy","version":"1.0.0","uptime":"10.5s","timestamp":"2025-12-09T06:07:59Z"}
```

---

## 🧪 API Testing Guide

### ⚠️ Important: API Endpoint Format

The IP address goes in the **URL path**, NOT as a query parameter:

```bash
# ✅ CORRECT - IP in path
curl "http://localhost/api/v1/check/8.8.8.8"

# ❌ WRONG - IP as query parameter
curl "http://localhost/api/v1/check?ip=8.8.8.8"
```

### Authentication

All API endpoints (except `/health` and `/metrics`) require an API key header:

```
X-API-Key: YOUR_API_KEY
```

---

### 1. Health Check

**Endpoint:** `GET /health`  
**Auth Required:** No

```bash
curl http://localhost/health
```

**Response:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "1h30m45s",
  "timestamp": "2025-12-09T06:07:59Z",
  "services": {
    "api": "healthy",
    "database": "healthy",
    "mmdb": "healthy"
  }
}
```

---

### 2. Check Single IP

**Endpoint:** `GET /api/v1/check/:ip`  
**Auth Required:** Yes

```bash
# Replace YOUR_API_KEY with your actual API key
curl -H "X-API-Key: YOUR_API_KEY" "http://localhost/api/v1/check/8.8.8.8"
```

**Example - Check Google DNS (clean IP):**
```bash
curl -H "X-API-Key: NrA3pia8Bb2TATLPxlTp2NJWgahwnGPb" \
  "http://localhost/api/v1/check/8.8.8.8"
```

**Response (Clean IP):**
```json
{
  "ip": "8.8.8.8",
  "score": 0,
  "risk_level": "low",
  "threats": [],
  "is_proxy": false,
  "is_vpn": false,
  "is_tor": false,
  "is_datacenter": false,
  "cached": false,
  "query_time_ms": 0.45
}
```

**Example - Check Known Malicious IP:**
```bash
curl -H "X-API-Key: NrA3pia8Bb2TATLPxlTp2NJWgahwnGPb" \
  "http://localhost/api/v1/check/185.220.101.1"
```

**Response (Malicious IP):**
```json
{
  "ip": "185.220.101.1",
  "score": 85,
  "risk_level": "high",
  "threats": ["tor_exit", "proxy", "anonymizer"],
  "is_proxy": true,
  "is_vpn": false,
  "is_tor": true,
  "is_datacenter": true,
  "first_seen": "2024-01-15T00:00:00Z",
  "last_seen": "2025-12-09T00:00:00Z",
  "source_count": 5,
  "cached": false,
  "query_time_ms": 0.32
}
```

---

### 3. Batch Check (Multiple IPs)

**Endpoint:** `POST /api/v1/batch`  
**Auth Required:** Yes  
**Max IPs:** 100 per request

```bash
curl -X POST \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"ips": ["8.8.8.8", "185.220.101.1", "1.1.1.1"]}' \
  "http://localhost/api/v1/batch"
```

**Response:**
```json
{
  "results": [
    {"ip": "8.8.8.8", "score": 0, "risk_level": "low"},
    {"ip": "185.220.101.1", "score": 85, "risk_level": "high"},
    {"ip": "1.1.1.1", "score": 0, "risk_level": "low"}
  ],
  "total_count": 3,
  "total_time_ms": 1.25
}
```

---

### 4. Get Statistics

**Endpoint:** `GET /api/v1/stats`  
**Auth Required:** Yes

```bash
curl -H "X-API-Key: YOUR_API_KEY" "http://localhost/api/v1/stats"
```

---

### 5. Prometheus Metrics

**Endpoint:** `GET /metrics`  
**Auth Required:** No

```bash
curl http://localhost/metrics
```

---

## 🌐 External Access (From Internet)

### Access from Your Computer

Replace `YOUR_VPS_IP` with your VPS IP address:

```bash
curl -H "X-API-Key: YOUR_API_KEY" "http://YOUR_VPS_IP/api/v1/check/8.8.8.8"
```

### Real Example

```bash
curl -H "X-API-Key: NrA3pia8Bb2TATLPxlTp2NJWgahwnGPb" \
  "http://45.143.166.221/api/v1/check/8.8.8.8"
```

---

## 📁 Important Files & Locations

| File | Path | Description |
|------|------|-------------|
| **Credentials** | `/opt/beon-ipquality/credentials.txt` | API key, DB password |
| **Config** | `/opt/beon-ipquality/configs/config.yaml` | Main configuration |
| **Feeds Config** | `/opt/beon-ipquality/configs/feeds.yaml` | Threat feed sources |
| **Binaries** | `/opt/beon-ipquality/bin/` | api, judge, ingestor, compiler |
| **Logs** | `/var/log/beon-ipquality/` | Application logs |
| **Data** | `/var/lib/beon-ipquality/` | MMDB files, cache |

### View Your Credentials

```bash
sudo cat /opt/beon-ipquality/credentials.txt
```

### View Logs

```bash
# Real-time API logs
sudo journalctl -u beon-api -f

# Or log file
sudo tail -f /var/log/beon-ipquality/api.log
```

---

## 🔧 Management Commands

### Service Management

```bash
# Start API
sudo systemctl start beon-api

# Stop API
sudo systemctl stop beon-api

# Restart API
sudo systemctl restart beon-api

# Check status
sudo systemctl status beon-api

# Enable auto-start on boot
sudo systemctl enable beon-api
```

### Database Operations

```bash
# Check total IP count in database
sudo -u postgres psql -d ipquality -c "SELECT COUNT(*) FROM ip_reputation;"

# List all tables
sudo -u postgres psql -d ipquality -c "\dt"

# Query specific IP
sudo -u postgres psql -d ipquality -c \
  "SELECT * FROM ip_reputation WHERE ip_address = '185.220.101.1';"

# Check threat types distribution
sudo -u postgres psql -d ipquality -c \
  "SELECT threat_type, COUNT(*) FROM ip_reputation GROUP BY threat_type ORDER BY count DESC;"
```

### Manual Threat Feed Update

```bash
# Run ingestor manually (normally auto-runs every 4 hours)
sudo -u beon /opt/beon-ipquality/bin/ingestor --once --verbose
```

### Compile MMDB (Optional - for faster lookups)

```bash
sudo -u beon /opt/beon-ipquality/bin/compiler
```

---

## 🔄 Auto-Updates

Threat feeds are automatically updated every 4 hours via cron:

```bash
# View cron job
cat /etc/cron.d/beon-ingestor

# Check last ingestor run
sudo journalctl -u beon-ingestor --since "4 hours ago"
```

---

## 📊 Monitoring & Dashboards

### Grafana Dashboard (Optional)

To install Grafana for monitoring:

```bash
# Install Grafana
sudo apt-get install -y apt-transport-https software-properties-common
wget -q -O - https://apt.grafana.com/gpg.key | sudo apt-key add -
echo "deb https://apt.grafana.com stable main" | sudo tee /etc/apt/sources.list.d/grafana.list
sudo apt-get update && sudo apt-get install -y grafana

# Start Grafana
sudo systemctl start grafana-server
sudo systemctl enable grafana-server
```

**Access Grafana:**
- URL: `http://YOUR_VPS_IP:3000`
- Default Username: `admin`
- Default Password: `admin` (change on first login)
- Or use generated password from: `cat /opt/beon-ipquality/credentials.txt | grep GRAFANA`

**Configure Prometheus Data Source:**
1. Go to Configuration → Data Sources → Add data source
2. Select **Prometheus**
3. URL: `http://localhost:9090`
4. Click **Save & Test**

### Prometheus Metrics

Built-in metrics endpoint at `/metrics`:

```bash
curl http://localhost/metrics
```

Available metrics:
- `beon_api_requests_total` - Total API requests
- `beon_api_request_duration_seconds` - Request latency
- `beon_ip_lookups_total` - IP lookup count
- `beon_cache_hits_total` - Redis cache hits
- `beon_database_queries_total` - PostgreSQL queries

---

## 📊 Threat Feed Sources

BEON-IPQuality aggregates data from 21 threat intelligence feeds:

| Feed | Type | Description |
|------|------|-------------|
| Firehol Level 1-4 | Aggregated | Multiple blocklists combined |
| Emerging Threats | Compromised | Known compromised IPs |
| Tor Exit Nodes | Anonymizer | Tor network exits |
| Spamhaus DROP | Spam | Spam networks |
| Abuse.ch | Malware | Malware/Botnet C2 |
| Blocklist.de | Brute Force | SSH/FTP attackers |
| DShield | Attacks | Active attackers |
| Talos Intelligence | Threats | Cisco threat intel |
| *And 13 more...* | Various | Multiple sources |

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    BEON-IPQuality System                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────┐    ┌─────────┐    ┌─────────────────────────┐ │
│  │  Nginx  │───▶│   API   │───▶│  PostgreSQL (1.6M IPs) │ │
│  │ :80/443 │    │  :8080  │    │       + Redis Cache     │ │
│  └─────────┘    └─────────┘    └─────────────────────────┘ │
│                      │                                      │
│                      ▼                                      │
│              ┌─────────────┐                                │
│              │    MMDB     │  (Optional compiled DB)        │
│              │  < 1ms      │                                │
│              └─────────────┘                                │
│                                                             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │  Ingestor (Cron every 4h)                               ││
│  │  - Fetches 21 threat feeds                              ││
│  │  - Updates 1.6M+ IP entries                             ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

---

## 📜 License

MIT License - see [LICENSE](LICENSE) file.

---

## 🙏 Credits & Attribution

### Threat Intelligence Feeds

Thanks to these amazing open-source threat intelligence providers:

| Provider | Website | License |
|----------|---------|---------|
| **FireHOL IP Lists** | [iplists.firehol.org](https://iplists.firehol.org/) | GPL |
| **Emerging Threats** | [rules.emergingthreats.net](https://rules.emergingthreats.net/) | BSD |
| **Tor Project** | [torproject.org](https://www.torproject.org/) | BSD |
| **Spamhaus** | [spamhaus.org](https://www.spamhaus.org/) | Spamhaus License |
| **Abuse.ch** | [abuse.ch](https://abuse.ch/) | CC0 |
| **Blocklist.de** | [blocklist.de](https://www.blocklist.de/) | GPL |
| **DShield** | [dshield.org](https://www.dshield.org/) | CC BY-NC-SA |
| **GreenSnow** | [greensnow.co](https://greensnow.co/) | Free |
| **CINS Score** | [cinsscore.com](https://cinsscore.com/) | Free |
| **Binary Defense** | [binarydefense.com](https://www.binarydefense.com/) | Free |
| **Stamparm** | [github.com/stamparm](https://github.com/stamparm/ipsum) | GPL |
| **IPSum** | [github.com/stamparm/ipsum](https://github.com/stamparm/ipsum) | GPL |

### GeoIP Data

- **MaxMind GeoLite2** - [maxmind.com](https://www.maxmind.com/) - CC BY-SA 4.0

### Open Source Libraries

| Library | Purpose | License |
|---------|---------|---------|
| [Fiber](https://gofiber.io/) | Web Framework | MIT |
| [pgx](https://github.com/jackc/pgx) | PostgreSQL Driver | MIT |
| [go-redis](https://github.com/redis/go-redis) | Redis Client | BSD-2 |
| [maxminddb-golang](https://github.com/oschwald/maxminddb-golang) | MMDB Reader | ISC |
| [mmdbwriter](https://github.com/maxmind/mmdbwriter) | MMDB Writer | Apache 2.0 |
| [viper](https://github.com/spf13/viper) | Configuration | MIT |
| [zap](https://github.com/uber-go/zap) | Logging | MIT |

### Inspired By

- [IPQualityScore](https://www.ipqualityscore.com/)
- [Proxycheck.io](https://proxycheck.io/)
- [IP2Location](https://www.ip2location.com/)

---

## 🤝 Contributing

Contributions welcome! Please open an issue or pull request.

---

## 👨‍💻 Author

**BEON Team**

- GitHub: [@afuzapratama](https://github.com/afuzapratama)

---

Made with ❤️ in Indonesia 🇮🇩
