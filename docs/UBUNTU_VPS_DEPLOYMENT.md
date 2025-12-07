# BEON-IPQuality Ubuntu VPS Deployment Guide

## Prerequisites

- **OS**: Ubuntu 22.04 LTS or 24.04 LTS
- **RAM**: Minimum 2GB (4GB+ recommended)
- **Storage**: 20GB+ SSD
- **CPU**: 2+ cores recommended
- **Root access** or sudo privileges

## Quick Installation

### One-Line Install

```bash
# Clone repository
git clone https://github.com/your-org/BEON-IPQuality.git /tmp/BEON-IPQuality

# Run installer
sudo bash /tmp/BEON-IPQuality/scripts/install-ubuntu.sh
```

### Manual Installation

1. **Update System**
```bash
sudo apt update && sudo apt upgrade -y
```

2. **Install Go 1.23**
```bash
wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
source ~/.profile
```

3. **Install PostgreSQL 17**
```bash
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget -qO- https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg
sudo apt update
sudo apt install -y postgresql-17
```

4. **Install Redis 7**
```bash
curl -fsSL https://packages.redis.io/gpg | sudo gpg --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/redis.list
sudo apt update
sudo apt install -y redis-server
```

5. **Build BEON-IPQuality**
```bash
cd /opt
git clone https://github.com/your-org/BEON-IPQuality.git
cd BEON-IPQuality

go build -o bin/api ./cmd/api
go build -o bin/judge ./cmd/judge
go build -o bin/ingestor ./cmd/ingestor
go build -o bin/compiler ./cmd/compiler
```

## Post-Installation Configuration

### 1. Database Setup

```bash
# Access PostgreSQL
sudo -u postgres psql

# Create user and database
CREATE USER beon WITH PASSWORD 'your_secure_password';
CREATE DATABASE ipquality OWNER beon;
GRANT ALL PRIVILEGES ON DATABASE ipquality TO beon;
\c ipquality
CREATE EXTENSION IF NOT EXISTS pg_trgm;
\q
```

### 2. Configuration File

Edit `/opt/beon-ipquality/configs/config.yaml`:

```yaml
server:
  host: "127.0.0.1"
  port: 8080

database:
  host: "localhost"
  port: 5432
  user: "beon"
  password: "your_secure_password"
  name: "ipquality"

redis:
  host: "localhost"
  port: 6379

api:
  key: "your-32-char-minimum-api-key-here"
  rate_limit: 1000
```

### 3. MaxMind GeoIP

Get a free license key from [MaxMind](https://www.maxmind.com/en/geolite2/signup):

```bash
export MAXMIND_LICENSE_KEY='your_license_key'
/opt/beon-ipquality/scripts/update-geoip.sh
```

### 4. Initial Data Load

```bash
# Run ingestor to fetch threat feeds
sudo -u beon /opt/beon-ipquality/bin/ingestor

# Compile MMDB database
sudo -u beon /opt/beon-ipquality/bin/compiler
```

### 5. Start Services

```bash
# Start API server
sudo systemctl start beon-api
sudo systemctl enable beon-api

# Verify it's running
sudo systemctl status beon-api
curl http://localhost:8080/health
```

## SSL Configuration with Let's Encrypt

```bash
# Install Certbot
sudo apt install certbot python3-certbot-nginx

# Get certificate (replace with your domain)
sudo certbot --nginx -d api.yourdomain.com

# Auto-renewal is configured automatically
```

## Architecture Overview

```
Internet
    │
    ▼
┌─────────────────────────────────────────────────────┐
│                    Ubuntu VPS                        │
│                                                      │
│  ┌─────────────────────────────────────────────────┐ │
│  │            Nginx (Port 80/443)                   │ │
│  │  - SSL termination                               │ │
│  │  - Rate limiting                                 │ │
│  │  - Reverse proxy                                 │ │
│  └──────────────────────┬──────────────────────────┘ │
│                         │                             │
│                         ▼                             │
│  ┌─────────────────────────────────────────────────┐ │
│  │         BEON API (127.0.0.1:8080)                │ │
│  │  - IP reputation checks                          │ │
│  │  - MMDB lookups                                  │ │
│  │  - Redis caching                                 │ │
│  └──────────────────────┬──────────────────────────┘ │
│                         │                             │
│           ┌─────────────┴─────────────┐               │
│           ▼                           ▼               │
│  ┌─────────────────┐       ┌─────────────────┐       │
│  │   PostgreSQL    │       │      Redis      │       │
│  │   (Port 5432)   │       │   (Port 6379)   │       │
│  └─────────────────┘       └─────────────────┘       │
│                                                      │
│  ┌─────────────────────────────────────────────────┐ │
│  │              Data Files                          │ │
│  │  /var/lib/beon-ipquality/mmdb/ipquality.mmdb    │ │
│  │  /var/lib/beon-ipquality/geoip/GeoLite2-*.mmdb  │ │
│  └─────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

## Service Management

```bash
# API Server
sudo systemctl start beon-api
sudo systemctl stop beon-api
sudo systemctl restart beon-api
sudo systemctl status beon-api

# Judge Node (optional)
sudo systemctl start beon-judge
sudo systemctl status beon-judge

# View logs
sudo journalctl -u beon-api -f
tail -f /var/log/beon-ipquality/api.log
```

## Monitoring

### Health Check
```bash
curl http://localhost:8080/health
```

### Prometheus Metrics
```bash
curl http://localhost:9100/metrics
```

### System Resources
```bash
htop
free -h
df -h
```

## Security Hardening

### 1. Firewall (UFW)
```bash
sudo ufw status
# Should show: SSH, HTTP, HTTPS allowed
```

### 2. Fail2ban
```bash
sudo fail2ban-client status
sudo fail2ban-client status nginx-limit-req
```

### 3. SSH Hardening
```bash
# Edit /etc/ssh/sshd_config
PermitRootLogin no
PasswordAuthentication no  # Use SSH keys only
MaxAuthTries 3
```

### 4. Automatic Updates
```bash
sudo apt install unattended-upgrades
sudo dpkg-reconfigure -plow unattended-upgrades
```

## Performance Tuning

### 1. System Limits

Edit `/etc/security/limits.conf`:
```
beon soft nofile 65535
beon hard nofile 65535
```

### 2. Sysctl Tuning

Edit `/etc/sysctl.d/99-beon.conf`:
```
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_tw_reuse = 1
vm.swappiness = 10
```

Apply: `sudo sysctl --system`

### 3. PostgreSQL Tuning

Edit `/etc/postgresql/17/main/postgresql.conf`:
```
shared_buffers = 512MB
effective_cache_size = 1536MB
maintenance_work_mem = 128MB
work_mem = 32MB
max_connections = 100
```

### 4. Redis Tuning

Edit `/etc/redis/redis.conf`:
```
maxmemory 512mb
maxmemory-policy allkeys-lru
```

## Backup Strategy

### Database Backup
```bash
# Manual backup
pg_dump -U beon ipquality > backup.sql

# Automated daily backup (add to cron)
0 2 * * * pg_dump -U beon ipquality | gzip > /backup/ipquality_$(date +\%Y\%m\%d).sql.gz
```

### MMDB Backup
```bash
cp /var/lib/beon-ipquality/mmdb/ipquality.mmdb /backup/
```

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
# Test PostgreSQL connection
psql -h localhost -U beon -d ipquality -c "SELECT 1"

# Check PostgreSQL status
sudo systemctl status postgresql
```

### High Memory Usage
```bash
# Check what's using memory
ps aux --sort=-%mem | head -10

# Redis memory
redis-cli INFO memory

# PostgreSQL connections
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity;"
```

### Slow Queries
```bash
# Check API latency
curl -w "@curl-format.txt" http://localhost:8080/api/v1/check/8.8.8.8

# Enable PostgreSQL slow query logging
# Add to postgresql.conf:
# log_min_duration_statement = 100
```

## Scaling Options

### Vertical Scaling
- Upgrade VPS RAM/CPU
- Add more PostgreSQL shared_buffers
- Increase Redis maxmemory

### Horizontal Scaling
- Add read replicas for PostgreSQL
- Use Redis Cluster
- Load balancer with multiple API instances

### CDN Integration
- Cloudflare in front for DDoS protection
- Cache responses at edge (be careful with cache invalidation)

## Cost Estimation

| Provider | Specs | Est. Monthly Cost |
|----------|-------|-------------------|
| DigitalOcean | 2GB RAM, 1 vCPU | $12 |
| Linode | 2GB RAM, 1 vCPU | $12 |
| Vultr | 2GB RAM, 1 vCPU | $10 |
| Hetzner | 4GB RAM, 2 vCPU | €5 (~$5.50) |
| AWS EC2 t3.small | 2GB RAM, 2 vCPU | ~$15 |

**Recommended**: Start with 2GB RAM, scale to 4-8GB based on traffic.
