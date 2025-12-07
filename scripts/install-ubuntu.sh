#!/bin/bash
#===============================================================================
# BEON-IPQuality Ubuntu VPS Installation Script
# Tested on: Ubuntu 22.04 LTS, Ubuntu 24.04 LTS
# Requirements: 2GB+ RAM, 20GB+ Storage
#===============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/opt/beon-ipquality"
DATA_DIR="/var/lib/beon-ipquality"
LOG_DIR="/var/log/beon-ipquality"
USER="beon"
GROUP="beon"

# Print with color
print_status() { echo -e "${BLUE}[*]${NC} $1"; }
print_success() { echo -e "${GREEN}[✓]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[!]${NC} $1"; }
print_error() { echo -e "${RED}[✗]${NC} $1"; }

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   print_error "This script must be run as root"
   exit 1
fi

print_status "Starting BEON-IPQuality installation..."

#===============================================================================
# STEP 1: System Update & Base Packages
#===============================================================================
print_status "Updating system packages..."
apt-get update -qq
apt-get upgrade -y -qq

print_status "Installing base dependencies..."
apt-get install -y -qq \
    curl wget git \
    software-properties-common \
    apt-transport-https \
    ca-certificates \
    gnupg lsb-release \
    build-essential \
    ufw fail2ban

print_success "Base packages installed"

#===============================================================================
# STEP 2: Install Go 1.23
#===============================================================================
print_status "Installing Go 1.23..."
GO_VERSION="1.23.4"
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
rm -rf /usr/local/go
tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
rm "go${GO_VERSION}.linux-amd64.tar.gz"

# Set up Go environment
cat >> /etc/profile.d/go.sh << 'EOF'
export PATH=$PATH:/usr/local/go/bin
export GOPATH=/opt/go
export PATH=$PATH:$GOPATH/bin
EOF
source /etc/profile.d/go.sh

print_success "Go ${GO_VERSION} installed"

#===============================================================================
# STEP 3: Install PostgreSQL 17
#===============================================================================
print_status "Installing PostgreSQL 17..."
sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget -qO- https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg
apt-get update -qq
apt-get install -y -qq postgresql-17 postgresql-contrib-17

# Start PostgreSQL
systemctl start postgresql
systemctl enable postgresql

# Create database and user
sudo -u postgres psql << EOF
CREATE USER beon WITH PASSWORD 'changeme_secure_password';
CREATE DATABASE ipquality OWNER beon;
GRANT ALL PRIVILEGES ON DATABASE ipquality TO beon;
\c ipquality
CREATE EXTENSION IF NOT EXISTS pg_trgm;
EOF

print_success "PostgreSQL 17 installed and configured"

#===============================================================================
# STEP 4: Install Redis 7
#===============================================================================
print_status "Installing Redis 7..."
curl -fsSL https://packages.redis.io/gpg | gpg --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $(lsb_release -cs) main" > /etc/apt/sources.list.d/redis.list
apt-get update -qq
apt-get install -y -qq redis-server

# Configure Redis
sed -i 's/^supervised no/supervised systemd/' /etc/redis/redis.conf
sed -i 's/^# maxmemory .*/maxmemory 256mb/' /etc/redis/redis.conf
sed -i 's/^# maxmemory-policy .*/maxmemory-policy allkeys-lru/' /etc/redis/redis.conf

systemctl restart redis-server
systemctl enable redis-server

print_success "Redis 7 installed and configured"

#===============================================================================
# STEP 5: Install Nginx
#===============================================================================
print_status "Installing Nginx..."
apt-get install -y -qq nginx

# Create Nginx config
cat > /etc/nginx/sites-available/beon-ipquality << 'EOF'
upstream ipquality_api {
    server 127.0.0.1:8080;
    keepalive 64;
}

# Rate limiting zone
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;

server {
    listen 80;
    server_name _;
    
    # Redirect HTTP to HTTPS (uncomment when SSL is configured)
    # return 301 https://$server_name$request_uri;

    location / {
        proxy_pass http://ipquality_api;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
        
        # Rate limiting
        limit_req zone=api_limit burst=200 nodelay;
        
        # Timeouts
        proxy_connect_timeout 5s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
        
        # Buffering
        proxy_buffering on;
        proxy_buffer_size 4k;
        proxy_buffers 8 4k;
    }
    
    location /health {
        proxy_pass http://ipquality_api/health;
        access_log off;
    }
    
    location /metrics {
        # Restrict metrics to internal access
        allow 127.0.0.1;
        deny all;
        proxy_pass http://127.0.0.1:9100/metrics;
    }
}

# HTTPS server (uncomment and configure SSL)
# server {
#     listen 443 ssl http2;
#     server_name your-domain.com;
#     
#     ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
#     ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
#     ssl_protocols TLSv1.2 TLSv1.3;
#     ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
#     ssl_prefer_server_ciphers off;
#     
#     # ... same location blocks as above
# }
EOF

ln -sf /etc/nginx/sites-available/beon-ipquality /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

nginx -t && systemctl restart nginx
systemctl enable nginx

print_success "Nginx installed and configured"

#===============================================================================
# STEP 6: Create User and Directories
#===============================================================================
print_status "Creating service user and directories..."

# Create user
id -u $USER &>/dev/null || useradd -r -s /bin/false $USER

# Create directories
mkdir -p $INSTALL_DIR/{bin,configs}
mkdir -p $DATA_DIR/{mmdb,geoip}
mkdir -p $LOG_DIR

# Set permissions
chown -R $USER:$GROUP $INSTALL_DIR
chown -R $USER:$GROUP $DATA_DIR
chown -R $USER:$GROUP $LOG_DIR

print_success "Directories created"

#===============================================================================
# STEP 7: Clone and Build BEON-IPQuality
#===============================================================================
print_status "Building BEON-IPQuality..."

# Clone repository (or copy from current directory)
cd /tmp
if [ -d "BEON-IPQuality" ]; then
    rm -rf BEON-IPQuality
fi

# Option 1: Clone from GitHub (replace with your repo)
# git clone https://github.com/your-org/BEON-IPQuality.git

# Option 2: Copy from local (if running on same machine)
print_warning "Please ensure BEON-IPQuality source code is at /tmp/BEON-IPQuality"
print_warning "Or modify this script to clone from your repository"

# For now, assume source is available
if [ -d "/tmp/BEON-IPQuality" ]; then
    cd /tmp/BEON-IPQuality
    
    # Build binaries
    source /etc/profile.d/go.sh
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/api ./cmd/api
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/judge ./cmd/judge
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/ingestor ./cmd/ingestor
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/compiler ./cmd/compiler
    
    # Copy configs
    cp -r configs/* $INSTALL_DIR/configs/
    
    print_success "BEON-IPQuality built"
else
    print_error "Source code not found at /tmp/BEON-IPQuality"
    print_warning "Please clone the repository and re-run this script"
fi

#===============================================================================
# STEP 8: Create Configuration File
#===============================================================================
print_status "Creating configuration..."

cat > $INSTALL_DIR/configs/config.yaml << EOF
server:
  host: "127.0.0.1"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

database:
  host: "localhost"
  port: 5432
  user: "beon"
  password: "changeme_secure_password"
  name: "ipquality"
  sslmode: "disable"
  max_conns: 50
  max_idle_conns: 10

redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  pool_size: 100

mmdb:
  custom_path: "$DATA_DIR/mmdb/ipquality.mmdb"
  geoip_city: "$DATA_DIR/geoip/GeoLite2-City.mmdb"
  geoip_asn: "$DATA_DIR/geoip/GeoLite2-ASN.mmdb"
  geoip_country: "$DATA_DIR/geoip/GeoLite2-Country.mmdb"

cache:
  ttl: 300
  max_size: 100000

api:
  key: "your-secure-api-key-change-this"
  rate_limit: 1000
  rate_limit_window: 60

logging:
  level: "info"
  format: "json"
  output: "$LOG_DIR/api.log"
EOF

chown $USER:$GROUP $INSTALL_DIR/configs/config.yaml
chmod 640 $INSTALL_DIR/configs/config.yaml

print_success "Configuration created"

#===============================================================================
# STEP 9: Create Systemd Services
#===============================================================================
print_status "Creating systemd services..."

# API Service
cat > /etc/systemd/system/beon-api.service << EOF
[Unit]
Description=BEON-IPQuality API Server
After=network.target postgresql.service redis-server.service
Wants=postgresql.service redis-server.service

[Service]
Type=simple
User=$USER
Group=$GROUP
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/bin/api -config $INSTALL_DIR/configs/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535

# Environment
Environment=GOMAXPROCS=0

# Logging
StandardOutput=append:$LOG_DIR/api.log
StandardError=append:$LOG_DIR/api-error.log

# Security
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $LOG_DIR

[Install]
WantedBy=multi-user.target
EOF

# Judge Service
cat > /etc/systemd/system/beon-judge.service << EOF
[Unit]
Description=BEON-IPQuality Judge Node
After=network.target beon-api.service

[Service]
Type=simple
User=$USER
Group=$GROUP
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/bin/judge -config $INSTALL_DIR/configs/config.yaml
Restart=always
RestartSec=10
LimitNOFILE=65535

# Logging
StandardOutput=append:$LOG_DIR/judge.log
StandardError=append:$LOG_DIR/judge-error.log

# Security
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $LOG_DIR

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
systemctl daemon-reload

print_success "Systemd services created"

#===============================================================================
# STEP 10: Configure Firewall
#===============================================================================
print_status "Configuring firewall..."

ufw default deny incoming
ufw default allow outgoing
ufw allow ssh
ufw allow 80/tcp
ufw allow 443/tcp

# Allow internal services only from localhost
# PostgreSQL and Redis are bound to localhost by default

echo "y" | ufw enable

print_success "Firewall configured"

#===============================================================================
# STEP 11: Configure Fail2ban
#===============================================================================
print_status "Configuring Fail2ban..."

cat > /etc/fail2ban/jail.local << 'EOF'
[DEFAULT]
bantime = 1h
findtime = 10m
maxretry = 5

[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log

[nginx-limit-req]
enabled = true
port = http,https
filter = nginx-limit-req
logpath = /var/log/nginx/error.log
maxretry = 10
findtime = 1m
bantime = 10m
EOF

systemctl restart fail2ban
systemctl enable fail2ban

print_success "Fail2ban configured"

#===============================================================================
# STEP 12: Setup Log Rotation
#===============================================================================
print_status "Configuring log rotation..."

cat > /etc/logrotate.d/beon-ipquality << EOF
$LOG_DIR/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 640 $USER $GROUP
    sharedscripts
    postrotate
        systemctl reload beon-api > /dev/null 2>&1 || true
    endscript
}
EOF

print_success "Log rotation configured"

#===============================================================================
# STEP 13: Setup Cron Jobs
#===============================================================================
print_status "Setting up cron jobs..."

cat > /etc/cron.d/beon-ipquality << EOF
# BEON-IPQuality Cron Jobs

# Update threat feeds every 4 hours
0 */4 * * * $USER $INSTALL_DIR/bin/ingestor -config $INSTALL_DIR/configs/config.yaml >> $LOG_DIR/ingestor.log 2>&1

# Recompile MMDB every 4 hours (after ingestor)
15 */4 * * * $USER $INSTALL_DIR/bin/compiler -config $INSTALL_DIR/configs/config.yaml >> $LOG_DIR/compiler.log 2>&1

# Reload API after MMDB update
20 */4 * * * root curl -s -X POST http://127.0.0.1:8080/api/v1/reload >> $LOG_DIR/reload.log 2>&1

# Weekly GeoIP update (Sunday 3:00 AM)
0 3 * * 0 $USER $INSTALL_DIR/scripts/update-geoip.sh >> $LOG_DIR/geoip-update.log 2>&1
EOF

print_success "Cron jobs configured"

#===============================================================================
# STEP 14: Create Update Script
#===============================================================================
print_status "Creating update scripts..."

mkdir -p $INSTALL_DIR/scripts

cat > $INSTALL_DIR/scripts/update-geoip.sh << 'EOF'
#!/bin/bash
# GeoIP Update Script
# Requires: MAXMIND_LICENSE_KEY environment variable

GEOIP_DIR="/var/lib/beon-ipquality/geoip"
MAXMIND_LICENSE_KEY="${MAXMIND_LICENSE_KEY:-}"

if [ -z "$MAXMIND_LICENSE_KEY" ]; then
    echo "Error: MAXMIND_LICENSE_KEY not set"
    exit 1
fi

# Download databases
for db in GeoLite2-City GeoLite2-ASN GeoLite2-Country; do
    wget -q "https://download.maxmind.com/app/geoip_download?edition_id=${db}&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz" -O /tmp/${db}.tar.gz
    tar -xzf /tmp/${db}.tar.gz -C /tmp/
    mv /tmp/${db}_*/$(db).mmdb $GEOIP_DIR/
    rm -rf /tmp/${db}*
done

echo "GeoIP databases updated"
EOF

chmod +x $INSTALL_DIR/scripts/update-geoip.sh
chown $USER:$GROUP $INSTALL_DIR/scripts/update-geoip.sh

print_success "Update scripts created"

#===============================================================================
# FINAL: Summary
#===============================================================================
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║         BEON-IPQuality Installation Complete!               ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}Next Steps:${NC}"
echo ""
echo "1. Update the database password in:"
echo "   - /opt/beon-ipquality/configs/config.yaml"
echo "   - PostgreSQL: ALTER USER beon PASSWORD 'new_password';"
echo ""
echo "2. Set your API key in config.yaml"
echo ""
echo "3. Download MaxMind GeoIP databases:"
echo "   export MAXMIND_LICENSE_KEY='your-key'"
echo "   /opt/beon-ipquality/scripts/update-geoip.sh"
echo ""
echo "4. Run initial data ingestion:"
echo "   sudo -u beon /opt/beon-ipquality/bin/ingestor -config /opt/beon-ipquality/configs/config.yaml"
echo ""
echo "5. Compile MMDB:"
echo "   sudo -u beon /opt/beon-ipquality/bin/compiler -config /opt/beon-ipquality/configs/config.yaml"
echo ""
echo "6. Start services:"
echo "   systemctl start beon-api && systemctl enable beon-api"
echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}DOMAIN CONFIGURATION (IMPORTANT!)${NC}"
echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "7. Configure domain untuk akses API & services:"
echo ""
echo "   # Dengan SSL (recommended untuk production)"
echo "   sudo ./scripts/setup-domain.sh --domain api.yoursite.com --email you@email.com"
echo ""
echo "   # Tanpa SSL (development)"
echo "   sudo ./scripts/setup-domain.sh --domain api.yoursite.com --skip-ssl"
echo ""
echo "   Setelah domain dikonfigurasi, akses:"
echo ""
echo "   ┌────────────────────────────────────────────────────┐"
echo "   │ Service      │ URL                                │"
echo "   ├────────────────────────────────────────────────────┤"
echo "   │ API          │ http://api.yoursite.com            │"
echo "   │ API (HTTPS)  │ https://api.yoursite.com           │"
echo "   │ Grafana      │ http://api.yoursite.com:3000       │"
echo "   │ Judge Node   │ http://api.yoursite.com:8081       │"
echo "   │ Prometheus   │ http://api.yoursite.com:9090       │"
echo "   │ Metrics      │ http://api.yoursite.com:9100       │"
echo "   └────────────────────────────────────────────────────┘"
echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Service Status:"
echo "  PostgreSQL: $(systemctl is-active postgresql)"
echo "  Redis:      $(systemctl is-active redis-server)"
echo "  Nginx:      $(systemctl is-active nginx)"
echo ""
echo "Directories:"
echo "  Install: $INSTALL_DIR"
echo "  Data:    $DATA_DIR"
echo "  Logs:    $LOG_DIR"
echo "  Config:  $INSTALL_DIR/configs"
echo ""
echo "Documentation:"
echo "  - Domain Setup: $INSTALL_DIR/docs/DOMAIN_CONFIGURATION.md"
echo "  - Full Guide:   $INSTALL_DIR/docs/UBUNTU_VPS_DEPLOYMENT.md"
echo ""
print_success "Installation complete!"
