# 🌐 Domain Configuration Guide

## Overview

Panduan ini menjelaskan cara mengkonfigurasi domain untuk BEON-IPQuality di Ubuntu VPS.

### Access Pattern

```
┌─────────────────────────────────────────────────────────────────┐
│                        DOMAIN ACCESS                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  api.hutang.io          →  Main API (port 80/443)              │
│  api.hutang.io:3000     →  Grafana Dashboard                   │
│  api.hutang.io:8081     →  Judge Node                          │
│  api.hutang.io:9090     →  Prometheus                          │
│  api.hutang.io:9100     →  Metrics Exporter                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Quick Setup

### 1. Prerequisites

- Domain sudah terdaftar (contoh: hutang.io)
- DNS A record mengarah ke IP VPS
- BEON-IPQuality sudah terinstall (`install-ubuntu.sh`)

### 2. Configure DNS

Di DNS provider kamu (Cloudflare, Namecheap, dll):

```
Type    Name    Value           TTL
A       api     YOUR_VPS_IP     300
```

Contoh: `api.hutang.io → 192.168.1.100`

### 3. Run Domain Setup Script

```bash
# Dengan SSL (recommended untuk production)
sudo ./scripts/setup-domain.sh \
    --domain api.hutang.io \
    --email admin@hutang.io

# Tanpa SSL (development/testing)
sudo ./scripts/setup-domain.sh \
    --domain api.hutang.io \
    --skip-ssl
```

### 4. Verify Setup

```bash
# Test API
curl http://api.hutang.io/health

# Test IP check
curl "http://api.hutang.io/api/v1/check?ip=8.8.8.8"

# Test Grafana
curl http://api.hutang.io:3000
```

---

## Manual Configuration

Jika kamu ingin konfigurasi manual:

### 1. Nginx Configuration

```bash
sudo nano /etc/nginx/sites-available/beon-ipquality
```

```nginx
# Rate limiting
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;

# Upstream
upstream ipquality_api {
    server 127.0.0.1:8080;
    keepalive 64;
}

# Main API Server
server {
    listen 80;
    server_name api.hutang.io;  # GANTI DOMAIN

    location / {
        proxy_pass http://ipquality_api;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Connection "";
        
        limit_req zone=api_limit burst=200 nodelay;
    }

    location /health {
        proxy_pass http://ipquality_api/health;
        access_log off;
    }
}

# Grafana - Port 3000
server {
    listen 3000;
    server_name api.hutang.io;

    location / {
        proxy_pass http://127.0.0.1:3001;  # Grafana internal
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}

# Judge Node - Port 8081
server {
    listen 8081;
    server_name api.hutang.io;

    location / {
        proxy_pass http://127.0.0.1:8082;  # Judge internal
        proxy_http_version 1.1;
    }
}

# Prometheus - Port 9090
server {
    listen 9090;
    server_name api.hutang.io;

    location / {
        proxy_pass http://127.0.0.1:9091;  # Prometheus internal
        proxy_http_version 1.1;
    }
}
```

### 2. Enable Site

```bash
sudo ln -sf /etc/nginx/sites-available/beon-ipquality /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
```

### 3. Configure Firewall

```bash
sudo ufw allow 80/tcp    # HTTP API
sudo ufw allow 443/tcp   # HTTPS API
sudo ufw allow 3000/tcp  # Grafana
sudo ufw allow 8081/tcp  # Judge Node
sudo ufw allow 9090/tcp  # Prometheus
sudo ufw allow 9100/tcp  # Metrics
sudo ufw reload
```

### 4. SSL with Let's Encrypt

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d api.hutang.io
```

---

## Port Mapping Reference

| Service | External Port | Internal Port | URL Example |
|---------|--------------|---------------|-------------|
| **API** | 80 (HTTP) | 8080 | `http://api.hutang.io/` |
| **API** | 443 (HTTPS) | 8080 | `https://api.hutang.io/` |
| **Grafana** | 3000 | 3001 | `http://api.hutang.io:3000` |
| **Judge** | 8081 | 8082 | `http://api.hutang.io:8081` |
| **Prometheus** | 9090 | 9091 | `http://api.hutang.io:9090` |
| **Metrics** | 9100 | 9101 | `http://api.hutang.io:9100/metrics` |
| **PostgreSQL** | - | 5432 | Internal only |
| **Redis** | - | 6379 | Internal only |
| **ClickHouse** | - | 8123 | Internal only |

> **Note:** PostgreSQL, Redis, dan ClickHouse TIDAK boleh diekspos ke publik!

---

## Architecture

```
                    Internet
                        │
                        ▼
              ┌─────────────────┐
              │   Cloudflare    │  (Optional CDN/DDoS)
              │   DNS Proxy     │
              └────────┬────────┘
                       │
                       ▼
              ┌─────────────────┐
              │  Ubuntu VPS     │
              │  (Your Server)  │
              └────────┬────────┘
                       │
         ┌─────────────┼─────────────┐
         ▼             ▼             ▼
    Port 80/443   Port 3000    Port 8081
         │             │             │
         ▼             ▼             ▼
    ┌─────────┐  ┌─────────┐  ┌─────────┐
    │  Nginx  │  │  Nginx  │  │  Nginx  │
    │ Reverse │  │ Reverse │  │ Reverse │
    │  Proxy  │  │  Proxy  │  │  Proxy  │
    └────┬────┘  └────┬────┘  └────┬────┘
         │            │            │
         ▼            ▼            ▼
    ┌─────────┐  ┌─────────┐  ┌─────────┐
    │   API   │  │ Grafana │  │  Judge  │
    │  :8080  │  │  :3001  │  │  :8082  │
    └─────────┘  └─────────┘  └─────────┘
```

---

## Security Recommendations

### 1. Restrict Admin Endpoints

Edit Nginx config untuk membatasi akses:

```nginx
# Restrict internal endpoints
location /api/v1/reload {
    allow 127.0.0.1;
    allow YOUR_ADMIN_IP;
    deny all;
    proxy_pass http://ipquality_api;
}

# Restrict Prometheus
server {
    listen 9090;
    server_name api.hutang.io;
    
    allow 127.0.0.1;
    allow YOUR_ADMIN_IP;
    deny all;

    location / {
        proxy_pass http://127.0.0.1:9091;
    }
}
```

### 2. Add Basic Auth for Dashboards

```bash
# Create password file
sudo apt install apache2-utils
sudo htpasswd -c /etc/nginx/.htpasswd admin

# Add to Nginx
server {
    listen 3000;
    server_name api.hutang.io;
    
    auth_basic "Grafana Login";
    auth_basic_user_file /etc/nginx/.htpasswd;
    
    location / {
        proxy_pass http://127.0.0.1:3001;
    }
}
```

### 3. Use Cloudflare (Recommended)

1. Add domain to Cloudflare
2. Enable "Proxied" for A record
3. Set SSL mode to "Full (strict)"
4. Enable rate limiting rules
5. Configure DDoS protection

---

## Troubleshooting

### DNS Not Resolving

```bash
# Check DNS propagation
dig api.hutang.io
nslookup api.hutang.io

# Should return your VPS IP
```

### Nginx Configuration Error

```bash
# Test config
sudo nginx -t

# Check error log
sudo tail -f /var/log/nginx/error.log
```

### Port Already in Use

```bash
# Check what's using the port
sudo lsof -i :3000
sudo netstat -tlnp | grep 3000

# Kill process if needed
sudo kill -9 <PID>
```

### SSL Certificate Issues

```bash
# Renew certificate
sudo certbot renew --dry-run

# Force renewal
sudo certbot renew --force-renewal

# Check certificate
sudo certbot certificates
```

### API Not Responding

```bash
# Check API service
sudo systemctl status beon-api
sudo journalctl -u beon-api -f

# Check if API is listening
curl http://127.0.0.1:8080/health
```

---

## API Usage Examples

### Basic IP Check

```bash
# HTTP
curl "http://api.hutang.io/api/v1/check?ip=8.8.8.8"

# HTTPS
curl "https://api.hutang.io/api/v1/check?ip=8.8.8.8"
```

### Response Example

```json
{
  "success": true,
  "data": {
    "ip": "8.8.8.8",
    "risk_score": 0.05,
    "is_proxy": false,
    "is_vpn": false,
    "is_tor": false,
    "is_datacenter": true,
    "is_bot": false,
    "threat_level": "low",
    "country": "US",
    "asn": 15169,
    "org": "Google LLC",
    "latency_ms": 0.42
  }
}
```

### Batch Check

```bash
curl -X POST "https://api.hutang.io/api/v1/batch" \
     -H "Content-Type: application/json" \
     -d '{
       "ips": ["8.8.8.8", "1.1.1.1", "185.220.100.240"]
     }'
```

### With API Key (if configured)

```bash
curl "https://api.hutang.io/api/v1/check?ip=8.8.8.8" \
     -H "X-API-Key: your-api-key"
```

---

## Integration Examples

### JavaScript/Node.js

```javascript
const response = await fetch('https://api.hutang.io/api/v1/check?ip=8.8.8.8');
const data = await response.json();

if (data.data.risk_score > 0.7) {
    console.log('High risk IP detected!');
}
```

### Python

```python
import requests

response = requests.get('https://api.hutang.io/api/v1/check', 
                        params={'ip': '8.8.8.8'})
data = response.json()

if data['data']['is_proxy']:
    print('Proxy detected!')
```

### PHP

```php
$ip = $_SERVER['REMOTE_ADDR'];
$response = file_get_contents("https://api.hutang.io/api/v1/check?ip={$ip}");
$data = json_decode($response, true);

if ($data['data']['is_tor']) {
    // Block Tor users
    exit('Access denied');
}
```

---

## Monitoring Dashboard

Access Grafana at `http://api.hutang.io:3000`:

1. Default login: `admin` / `admin`
2. Add Prometheus data source: `http://localhost:9091`
3. Import BEON-IPQuality dashboard

### Key Metrics to Monitor

- `ipquality_requests_total` - Total requests
- `ipquality_request_duration_seconds` - Latency
- `ipquality_cache_hits_total` - Cache performance
- `ipquality_threat_detections_total` - Threats detected

---

## Questions?

Check the main documentation:
- [Ubuntu Deployment Guide](UBUNTU_VPS_DEPLOYMENT.md)
- [Railway Deployment Guide](RAILWAY_DEPLOYMENT.md)
- [API Reference](API.md)
