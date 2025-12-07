# 🚀 BEON-IPQuality Ubuntu VPS Deployment Guide

## Quick Start (One-Line Install)

```bash
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
```

That's it! The script will:
- ✅ Install Go 1.23, PostgreSQL 17, Redis 7, Nginx
- ✅ Clone the repository from GitHub
- ✅ Build all binaries
- ✅ Configure database, firewall, and services
- ✅ Generate secure credentials automatically

---

## Prerequisites

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| **OS** | Ubuntu 22.04 LTS | Ubuntu 24.04 LTS |
| **RAM** | 2GB | 4GB+ |
| **Storage** | 20GB SSD | 40GB+ SSD |
| **CPU** | 1 core | 2+ cores |

---

## Installation Options

### Option 1: One-Line Install (Recommended)

```bash
# Basic installation (auto-generates all credentials)
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
```

### Option 2: Custom Credentials

```bash
# With custom database password
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash -s -- --db-password "YourSecurePassword123"

# With custom API key
curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash -s -- --api-key "your32characterapikey1234567890"
```

### Option 3: Manual Download

```bash
# Download script first
wget https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh

# Review the script
less install-ubuntu.sh

# Run with options
sudo bash install-ubuntu.sh --db-password "YourPassword" --api-key "YourAPIKey"
```

---

## Post-Installation Steps

After installation completes, follow these steps:

### Step 1: Setup MaxMind GeoIP

Get a free license key from [MaxMind](https://www.maxmind.com/en/geolite2/signup):

```bash
# Copy example config
cp /opt/beon-ipquality/configs/GeoIP.conf.example /opt/beon-ipquality/configs/GeoIP.conf

# Edit with your credentials
nano /opt/beon-ipquality/configs/GeoIP.conf
# Change: AccountID YOUR_ACCOUNT_ID
# Change: LicenseKey YOUR_LICENSE_KEY

# Download GeoIP databases
/opt/beon-ipquality/scripts/update-geoip.sh
```

### Step 2: Run Initial Data Ingestion

```bash
# Fetch threat intelligence feeds
sudo -u beon /opt/beon-ipquality/bin/ingestor -config /opt/beon-ipquality/configs/config.yaml
```

### Step 3: Compile MMDB Database

```bash
# Compile IP reputation database
sudo -u beon /opt/beon-ipquality/bin/compiler -config /opt/beon-ipquality/configs/config.yaml
```

### Step 4: Start the API

```bash
# Start and enable the API service
sudo systemctl start beon-api
sudo systemctl enable beon-api

# Check status
sudo systemctl status beon-api

# Test the API
curl http://localhost/health
curl "http://localhost/api/v1/check?ip=8.8.8.8"
```

### Step 5: Configure Domain (Recommended)

```bash
# With SSL (production)
sudo /opt/beon-ipquality/scripts/setup-domain.sh --domain api.yourdomain.com --email you@email.com

# Without SSL (development)
sudo /opt/beon-ipquality/scripts/setup-domain.sh --domain api.yourdomain.com --skip-ssl
```

---

## Service Access URLs

After domain configuration, access your services at:

| Service | URL |
|---------|-----|
| **Main API** | `https://api.yourdomain.com` |
| **Health Check** | `https://api.yourdomain.com/health` |
| **Grafana** | `http://api.yourdomain.com:3000` |
| **Judge Node** | `http://api.yourdomain.com:8081` |
| **Prometheus** | `http://api.yourdomain.com:9090` |
| **Metrics** | `http://api.yourdomain.com:9100/metrics` |

---

## Architecture

```
                         Internet
                             │
                             ▼
                    ┌────────────────┐
                    │   Cloudflare   │  (Optional DDoS protection)
                    └───────┬────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Ubuntu VPS                                │
│                                                                  │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │              Nginx (Port 80/443)                          │  │
│   │  • SSL termination                                        │  │
│   │  • Rate limiting (100 req/s)                              │  │
│   │  • Reverse proxy                                          │  │
│   └────────────────────────┬─────────────────────────────────┘  │
│                            │                                     │
│                            ▼                                     │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │           BEON API Server (127.0.0.1:8080)                │  │
│   │  • IP reputation checks (<1ms)                            │  │
│   │  • MMDB lookups                                           │  │
│   │  • Risk scoring                                           │  │
│   └────────────────────────┬─────────────────────────────────┘  │
│                            │                                     │
│              ┌─────────────┴─────────────┐                       │
│              ▼                           ▼                       │
│   ┌──────────────────┐       ┌──────────────────┐               │
│   │   PostgreSQL 17  │       │     Redis 7      │               │
│   │   (Port 5432)    │       │   (Port 6379)    │               │
│   │   • IP data      │       │   • Cache        │               │
│   │   • Threat feeds │       │   • Rate limits  │               │
│   └──────────────────┘       └──────────────────┘               │
│                                                                  │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │                    Data Files                             │  │
│   │  /var/lib/beon-ipquality/mmdb/ipquality.mmdb             │  │
│   │  /var/lib/beon-ipquality/geoip/GeoLite2-*.mmdb           │  │
│   └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Service Management

```bash
# API Server
sudo systemctl start beon-api      # Start
sudo systemctl stop beon-api       # Stop
sudo systemctl restart beon-api    # Restart
sudo systemctl status beon-api     # Status
sudo systemctl enable beon-api     # Enable on boot

# Judge Node (optional)
sudo systemctl start beon-judge
sudo systemctl status beon-judge

# View logs
sudo journalctl -u beon-api -f                    # Live API logs
tail -f /var/log/beon-ipquality/api.log          # API log file
tail -f /var/log/beon-ipquality/ingestor.log     # Ingestor log
```

---

## Directory Structure

```
/opt/beon-ipquality/
├── bin/
│   ├── api          # API server binary
│   ├── judge        # Judge node binary
│   ├── ingestor     # Feed ingestor binary
│   └── compiler     # MMDB compiler binary
├── configs/
│   ├── config.yaml           # Main configuration
│   ├── feeds.yaml            # Threat feed sources
│   └── GeoIP.conf.example    # MaxMind config template
├── scripts/
│   ├── setup-domain.sh       # Domain configuration
│   ├── update-geoip.sh       # GeoIP updater
│   └── auto-update.sh        # Auto update script
├── docs/
│   └── *.md                  # Documentation
└── .credentials              # Generated credentials (root only)

/var/lib/beon-ipquality/
├── mmdb/
│   └── ipquality.mmdb        # Compiled IP database
└── geoip/
    ├── GeoLite2-City.mmdb    # MaxMind City DB
    └── GeoLite2-ASN.mmdb     # MaxMind ASN DB

/var/log/beon-ipquality/
├── api.log
├── api-error.log
├── ingestor.log
├── compiler.log
└── judge.log
```

---

## Configuration

### Main Config (`/opt/beon-ipquality/configs/config.yaml`)

```yaml
server:
  host: "127.0.0.1"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

database:
  host: "localhost"
  port: 5432
  user: "beon"
  password: "YOUR_DB_PASSWORD"
  name: "ipquality"

redis:
  host: "localhost"
  port: 6379

api:
  key: "YOUR_API_KEY"
  rate_limit: 1000
```

### View Credentials

```bash
# View saved credentials
sudo cat /opt/beon-ipquality/.credentials
```

---

## Security

### Firewall (UFW)

```bash
# Check status
sudo ufw status

# Default rules installed:
# - SSH (22)
# - HTTP (80)
# - HTTPS (443)
```

### Fail2ban

```bash
# Check status
sudo fail2ban-client status
sudo fail2ban-client status nginx-limit-req

# View banned IPs
sudo fail2ban-client status sshd
```

### SSL Certificate Renewal

```bash
# Test renewal
sudo certbot renew --dry-run

# Certificates auto-renew via cron
```

---

## Monitoring

### Health Check

```bash
curl http://localhost/health
```

### Prometheus Metrics

```bash
curl http://localhost:9100/metrics
```

### System Resources

```bash
htop                    # Process monitor
free -h                 # Memory usage
df -h                   # Disk usage
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check logs
sudo journalctl -u beon-api --no-pager -n 50

# Check config syntax
/opt/beon-ipquality/bin/api -config /opt/beon-ipquality/configs/config.yaml -validate
```

### Database Connection Issues

```bash
# Test PostgreSQL
psql -h localhost -U beon -d ipquality -c "SELECT 1"

# Check PostgreSQL status
sudo systemctl status postgresql
```

### High Memory Usage

```bash
# Check processes
ps aux --sort=-%mem | head -10

# Redis memory
redis-cli INFO memory
```

---

## Updating

### Update BEON-IPQuality

```bash
# Run the update script
/opt/beon-ipquality/scripts/auto-update.sh
```

### Update Threat Feeds Manually

```bash
sudo -u beon /opt/beon-ipquality/bin/ingestor -config /opt/beon-ipquality/configs/config.yaml
sudo -u beon /opt/beon-ipquality/bin/compiler -config /opt/beon-ipquality/configs/config.yaml
curl -X POST http://localhost:8080/api/v1/reload
```

---

## Cost Estimation

| Provider | Specs | Monthly Cost |
|----------|-------|--------------|
| **Hetzner** | 4GB RAM, 2 vCPU | €5 (~$5.50) |
| **Vultr** | 2GB RAM, 1 vCPU | $10 |
| **DigitalOcean** | 2GB RAM, 1 vCPU | $12 |
| **Linode** | 2GB RAM, 1 vCPU | $12 |
| **AWS EC2 t3.small** | 2GB RAM, 2 vCPU | ~$15 |

**Recommendation**: Start with 2GB RAM, scale to 4-8GB based on traffic.

---

## Support

- **Documentation**: `/opt/beon-ipquality/docs/`
- **GitHub Issues**: [github.com/afuzapratama/BEON-IPQuality/issues](https://github.com/afuzapratama/BEON-IPQuality/issues)
