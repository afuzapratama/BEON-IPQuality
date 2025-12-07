#!/bin/bash
#===============================================================================
# BEON-IPQuality Ubuntu VPS Installation Script
# 
# One-Line Install:
#   curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
#
# Or with options:
#   curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash -s -- --db-password "yourpass"
#
# Tested on: Ubuntu 22.04 LTS, Ubuntu 24.04 LTS
# Requirements: 2GB+ RAM, 20GB+ Storage
#===============================================================================

set -e

# Version
SCRIPT_VERSION="1.0.0"
GITHUB_REPO="afuzapratama/BEON-IPQuality"
GITHUB_BRANCH="main"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration defaults
INSTALL_DIR="/opt/beon-ipquality"
DATA_DIR="/var/lib/beon-ipquality"
LOG_DIR="/var/log/beon-ipquality"
USER="beon"
GROUP="beon"
DB_PASSWORD=""
API_KEY=""
SKIP_DEPS=false

#===============================================================================
# Helper Functions
#===============================================================================
print_banner() {
    echo -e "${CYAN}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                                                                   ║"
    echo "║   ██████╗ ███████╗ ██████╗ ███╗   ██╗                            ║"
    echo "║   ██╔══██╗██╔════╝██╔═══██╗████╗  ██║                            ║"
    echo "║   ██████╔╝█████╗  ██║   ██║██╔██╗ ██║                            ║"
    echo "║   ██╔══██╗██╔══╝  ██║   ██║██║╚██╗██║                            ║"
    echo "║   ██████╔╝███████╗╚██████╔╝██║ ╚████║                            ║"
    echo "║   ╚═════╝ ╚══════╝ ╚═════╝ ╚═╝  ╚═══╝                            ║"
    echo "║                                                                   ║"
    echo "║   IP Quality & Reputation System                                  ║"
    echo "║   Version: ${SCRIPT_VERSION}                                               ║"
    echo "║                                                                   ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

print_status() { echo -e "${BLUE}[*]${NC} $1"; }
print_success() { echo -e "${GREEN}[✓]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[!]${NC} $1"; }
print_error() { echo -e "${RED}[✗]${NC} $1"; }

print_step() {
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  STEP $1: $2${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

generate_password() {
    < /dev/urandom tr -dc 'A-Za-z0-9' | head -c 24
}

generate_api_key() {
    < /dev/urandom tr -dc 'A-Za-z0-9' | head -c 32
}

#===============================================================================
# Parse Arguments
#===============================================================================
usage() {
    cat << EOF
BEON-IPQuality Ubuntu VPS Installer v${SCRIPT_VERSION}

Usage: $0 [OPTIONS]

Options:
    --db-password PASSWORD    Set PostgreSQL password (auto-generated if not set)
    --api-key KEY             Set API key (auto-generated if not set)
    --skip-deps               Skip installing system dependencies
    --branch BRANCH           GitHub branch to use (default: main)
    -h, --help                Show this help message

Examples:
    # One-line install (recommended)
    curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-ubuntu.sh | sudo bash

    # With custom password
    curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-ubuntu.sh | sudo bash -s -- --db-password "MySecurePass123"

    # Local execution
    sudo ./install-ubuntu.sh --db-password "MySecurePass123" --api-key "myapikey123456789012345678901234"

EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --db-password)
            DB_PASSWORD="$2"
            shift 2
            ;;
        --api-key)
            API_KEY="$2"
            shift 2
            ;;
        --skip-deps)
            SKIP_DEPS=true
            shift
            ;;
        --branch)
            GITHUB_BRANCH="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            print_error "Unknown option: $1"
            usage
            ;;
    esac
done

#===============================================================================
# Pre-flight Checks
#===============================================================================
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

check_os() {
    if [[ ! -f /etc/os-release ]]; then
        print_error "Cannot detect OS. This script requires Ubuntu."
        exit 1
    fi
    
    source /etc/os-release
    if [[ "$ID" != "ubuntu" ]]; then
        print_error "This script is designed for Ubuntu. Detected: $ID"
        exit 1
    fi
    
    print_success "Detected: Ubuntu $VERSION_ID"
}

check_resources() {
    local mem_gb=$(free -g | awk '/^Mem:/{print $2}')
    local disk_gb=$(df -BG / | awk 'NR==2 {print $4}' | tr -d 'G')
    
    if [[ $mem_gb -lt 1 ]]; then
        print_warning "Low memory detected: ${mem_gb}GB (recommended: 2GB+)"
    else
        print_success "Memory: ${mem_gb}GB"
    fi
    
    if [[ $disk_gb -lt 10 ]]; then
        print_warning "Low disk space: ${disk_gb}GB (recommended: 20GB+)"
    else
        print_success "Disk space: ${disk_gb}GB available"
    fi
}

#===============================================================================
# MAIN INSTALLATION
#===============================================================================
main() {
    print_banner
    
    # Pre-flight
    check_root
    check_os
    check_resources
    
    # Generate credentials if not provided
    if [[ -z "$DB_PASSWORD" ]]; then
        DB_PASSWORD=$(generate_password)
        print_status "Generated database password"
    fi
    
    if [[ -z "$API_KEY" ]]; then
        API_KEY=$(generate_api_key)
        print_status "Generated API key"
    fi
    
    echo ""
    echo -e "${YELLOW}Installation will begin with these settings:${NC}"
    echo -e "  Install directory: ${CYAN}$INSTALL_DIR${NC}"
    echo -e "  Data directory:    ${CYAN}$DATA_DIR${NC}"
    echo -e "  Log directory:     ${CYAN}$LOG_DIR${NC}"
    echo -e "  Service user:      ${CYAN}$USER${NC}"
    echo ""

    #===========================================================================
    # STEP 1: System Update & Base Packages
    #===========================================================================
    if [[ "$SKIP_DEPS" = false ]]; then
        print_step "1/12" "System Update & Base Packages"
        
        print_status "Updating system packages..."
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get upgrade -y -qq
        
        print_status "Installing base dependencies..."
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
            curl wget git \
            software-properties-common \
            apt-transport-https \
            ca-certificates \
            gnupg lsb-release \
            build-essential \
            ufw fail2ban \
            jq htop
        
        print_success "Base packages installed"
    else
        print_step "1/12" "System Update (Skipped)"
        print_warning "Skipping dependency installation as requested"
    fi

    #===========================================================================
    # STEP 2: Install Go 1.23
    #===========================================================================
    print_step "2/12" "Installing Go 1.23"
    
    GO_VERSION="1.23.4"
    if command -v go &> /dev/null && go version | grep -q "go1.23"; then
        print_success "Go 1.23 already installed"
    else
        print_status "Downloading Go ${GO_VERSION}..."
        wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O /tmp/go.tar.gz
        rm -rf /usr/local/go
        tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
        
        # Set up Go environment
        cat > /etc/profile.d/go.sh << 'GOENV'
export PATH=$PATH:/usr/local/go/bin
export GOPATH=/opt/go
export PATH=$PATH:$GOPATH/bin
GOENV
        
        export PATH=$PATH:/usr/local/go/bin
        export GOPATH=/opt/go
        export PATH=$PATH:$GOPATH/bin
        
        print_success "Go ${GO_VERSION} installed"
    fi

    #===========================================================================
    # STEP 3: Install PostgreSQL 17
    #===========================================================================
    print_step "3/12" "Installing PostgreSQL 17"
    
    if command -v psql &> /dev/null && psql --version | grep -q "17"; then
        print_success "PostgreSQL 17 already installed"
    else
        print_status "Adding PostgreSQL repository..."
        sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
        wget -qO- https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg 2>/dev/null || true
        
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql-17 postgresql-contrib-17
        
        systemctl start postgresql
        systemctl enable postgresql
        
        print_success "PostgreSQL 17 installed"
    fi
    
    # Create database and user
    print_status "Configuring database..."
    sudo -u postgres psql -c "SELECT 1 FROM pg_user WHERE usename = 'beon'" | grep -q 1 || \
    sudo -u postgres psql << EOSQL
CREATE USER beon WITH PASSWORD '${DB_PASSWORD}';
CREATE DATABASE ipquality OWNER beon;
GRANT ALL PRIVILEGES ON DATABASE ipquality TO beon;
\c ipquality
CREATE EXTENSION IF NOT EXISTS pg_trgm;
EOSQL
    
    print_success "PostgreSQL configured"

    #===========================================================================
    # STEP 4: Install Redis 7
    #===========================================================================
    print_step "4/12" "Installing Redis 7"
    
    if command -v redis-server &> /dev/null; then
        print_success "Redis already installed"
    else
        print_status "Adding Redis repository..."
        curl -fsSL https://packages.redis.io/gpg | gpg --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg 2>/dev/null || true
        echo "deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $(lsb_release -cs) main" > /etc/apt/sources.list.d/redis.list
        
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq redis-server
        
        # Configure Redis
        sed -i 's/^supervised no/supervised systemd/' /etc/redis/redis.conf
        sed -i 's/^# maxmemory .*/maxmemory 256mb/' /etc/redis/redis.conf
        sed -i 's/^# maxmemory-policy .*/maxmemory-policy allkeys-lru/' /etc/redis/redis.conf
        
        systemctl restart redis-server
        systemctl enable redis-server
        
        print_success "Redis 7 installed"
    fi

    #===========================================================================
    # STEP 5: Install Nginx
    #===========================================================================
    print_step "5/12" "Installing Nginx"
    
    if command -v nginx &> /dev/null; then
        print_success "Nginx already installed"
    else
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx
        print_success "Nginx installed"
    fi
    
    # Basic Nginx config (will be replaced by setup-domain.sh)
    cat > /etc/nginx/sites-available/beon-ipquality << 'NGINXCONF'
upstream ipquality_api {
    server 127.0.0.1:8080;
    keepalive 64;
}

limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;

server {
    listen 80 default_server;
    server_name _;

    location / {
        proxy_pass http://ipquality_api;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
        
        limit_req zone=api_limit burst=200 nodelay;
        
        proxy_connect_timeout 5s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }
    
    location /health {
        proxy_pass http://ipquality_api/health;
        access_log off;
    }
}
NGINXCONF
    
    ln -sf /etc/nginx/sites-available/beon-ipquality /etc/nginx/sites-enabled/
    rm -f /etc/nginx/sites-enabled/default
    
    nginx -t && systemctl restart nginx
    systemctl enable nginx
    
    print_success "Nginx configured"

    #===========================================================================
    # STEP 6: Create User and Directories
    #===========================================================================
    print_step "6/12" "Creating User & Directories"
    
    # Create user
    id -u $USER &>/dev/null || useradd -r -s /bin/false $USER
    
    # Create directories
    mkdir -p $INSTALL_DIR/{bin,configs,scripts,docs}
    mkdir -p $DATA_DIR/{mmdb,geoip}
    mkdir -p $LOG_DIR
    
    print_success "Directories created"

    #===========================================================================
    # STEP 7: Clone Repository
    #===========================================================================
    print_step "7/12" "Cloning BEON-IPQuality Repository"
    
    print_status "Cloning from GitHub (${GITHUB_REPO})..."
    
    # Clone to temp directory
    rm -rf /tmp/BEON-IPQuality
    git clone --depth 1 --branch $GITHUB_BRANCH "https://github.com/${GITHUB_REPO}.git" /tmp/BEON-IPQuality
    
    print_success "Repository cloned"

    #===========================================================================
    # STEP 8: Build Binaries
    #===========================================================================
    print_step "8/12" "Building Binaries"
    
    cd /tmp/BEON-IPQuality
    
    # Ensure Go is available
    export PATH=$PATH:/usr/local/go/bin
    export GOPATH=/opt/go
    
    print_status "Building API server..."
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/api ./cmd/api
    
    print_status "Building Judge node..."
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/judge ./cmd/judge
    
    print_status "Building Ingestor..."
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/ingestor ./cmd/ingestor
    
    print_status "Building Compiler..."
    go build -ldflags="-w -s" -o $INSTALL_DIR/bin/compiler ./cmd/compiler
    
    # Copy configs and scripts
    cp -r configs/* $INSTALL_DIR/configs/ 2>/dev/null || true
    cp -r scripts/* $INSTALL_DIR/scripts/ 2>/dev/null || true
    cp -r docs/* $INSTALL_DIR/docs/ 2>/dev/null || true
    cp -r migrations $INSTALL_DIR/ 2>/dev/null || true
    
    # Make scripts executable
    chmod +x $INSTALL_DIR/scripts/*.sh 2>/dev/null || true
    chmod +x $INSTALL_DIR/bin/*
    
    print_success "Binaries built successfully"

    #===========================================================================
    # STEP 9: Create Configuration
    #===========================================================================
    print_step "9/12" "Creating Configuration"
    
    cat > $INSTALL_DIR/configs/config.yaml << CONFIGYAML
# BEON-IPQuality Configuration
# Generated: $(date)

server:
  host: "127.0.0.1"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

database:
  host: "localhost"
  port: 5432
  user: "beon"
  password: "${DB_PASSWORD}"
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
  path: "${DATA_DIR}/mmdb/ipquality.mmdb"
  geoip_city: "${DATA_DIR}/geoip/GeoLite2-City.mmdb"
  geoip_asn: "${DATA_DIR}/geoip/GeoLite2-ASN.mmdb"

cache:
  ttl: 300
  max_size: 100000

api:
  key: "${API_KEY}"
  rate_limit: 1000
  rate_limit_window: 60

logging:
  level: "info"
  format: "json"
  output: "${LOG_DIR}/api.log"

judge:
  enabled: false
  port: 8081
  workers: 10
  timeout: 5s
CONFIGYAML
    
    # Set permissions
    chown -R $USER:$GROUP $INSTALL_DIR
    chown -R $USER:$GROUP $DATA_DIR
    chown -R $USER:$GROUP $LOG_DIR
    chmod 640 $INSTALL_DIR/configs/config.yaml
    
    print_success "Configuration created"

    #===========================================================================
    # STEP 10: Create Systemd Services
    #===========================================================================
    print_step "10/12" "Creating Systemd Services"
    
    # API Service
    cat > /etc/systemd/system/beon-api.service << SVCAPI
[Unit]
Description=BEON-IPQuality API Server
After=network.target postgresql.service redis-server.service
Wants=postgresql.service redis-server.service

[Service]
Type=simple
User=${USER}
Group=${GROUP}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/bin/api -config ${INSTALL_DIR}/configs/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535

Environment=GOMAXPROCS=0

StandardOutput=append:${LOG_DIR}/api.log
StandardError=append:${LOG_DIR}/api-error.log

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${LOG_DIR}

[Install]
WantedBy=multi-user.target
SVCAPI

    # Judge Service
    cat > /etc/systemd/system/beon-judge.service << SVCJUDGE
[Unit]
Description=BEON-IPQuality Judge Node
After=network.target beon-api.service

[Service]
Type=simple
User=${USER}
Group=${GROUP}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/bin/judge -config ${INSTALL_DIR}/configs/config.yaml
Restart=always
RestartSec=10
LimitNOFILE=65535

StandardOutput=append:${LOG_DIR}/judge.log
StandardError=append:${LOG_DIR}/judge-error.log

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${LOG_DIR}

[Install]
WantedBy=multi-user.target
SVCJUDGE
    
    systemctl daemon-reload
    print_success "Systemd services created"

    #===========================================================================
    # STEP 11: Configure Firewall & Security
    #===========================================================================
    print_step "11/12" "Configuring Firewall & Security"
    
    # UFW
    ufw default deny incoming
    ufw default allow outgoing
    ufw allow ssh
    ufw allow 80/tcp
    ufw allow 443/tcp
    echo "y" | ufw enable
    
    print_success "Firewall configured"
    
    # Fail2ban
    cat > /etc/fail2ban/jail.local << 'F2BCONF'
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
F2BCONF
    
    systemctl restart fail2ban
    systemctl enable fail2ban
    
    print_success "Fail2ban configured"

    #===========================================================================
    # STEP 12: Setup Cron Jobs & Log Rotation
    #===========================================================================
    print_step "12/12" "Setting Up Cron Jobs & Log Rotation"
    
    # Cron jobs
    cat > /etc/cron.d/beon-ipquality << CRONJOBS
# BEON-IPQuality Automated Tasks

# Update threat feeds every 4 hours
0 */4 * * * ${USER} ${INSTALL_DIR}/bin/ingestor -config ${INSTALL_DIR}/configs/config.yaml >> ${LOG_DIR}/ingestor.log 2>&1

# Recompile MMDB every 4 hours (15 min after ingestor)
15 */4 * * * ${USER} ${INSTALL_DIR}/bin/compiler -config ${INSTALL_DIR}/configs/config.yaml >> ${LOG_DIR}/compiler.log 2>&1

# Hot reload API after MMDB update
20 */4 * * * root curl -s -X POST http://127.0.0.1:8080/api/v1/reload >> ${LOG_DIR}/reload.log 2>&1

# Weekly GeoIP update (Sunday 3:00 AM)
0 3 * * 0 ${USER} ${INSTALL_DIR}/scripts/update-geoip.sh >> ${LOG_DIR}/geoip-update.log 2>&1
CRONJOBS
    
    # Log rotation
    cat > /etc/logrotate.d/beon-ipquality << LOGROTATE
${LOG_DIR}/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0640 ${USER} ${GROUP}
    sharedscripts
    postrotate
        systemctl reload beon-api > /dev/null 2>&1 || true
    endscript
}
LOGROTATE
    
    print_success "Cron jobs and log rotation configured"

    #===========================================================================
    # CLEANUP
    #===========================================================================
    print_status "Cleaning up..."
    rm -rf /tmp/BEON-IPQuality
    apt-get autoremove -y -qq
    apt-get clean
    
    #===========================================================================
    # SAVE CREDENTIALS
    #===========================================================================
    cat > $INSTALL_DIR/.credentials << CREDS
# BEON-IPQuality Credentials
# Generated: $(date)
# KEEP THIS FILE SECURE!

DATABASE_PASSWORD=${DB_PASSWORD}
API_KEY=${API_KEY}
CREDS
    chmod 600 $INSTALL_DIR/.credentials
    chown root:root $INSTALL_DIR/.credentials
    
    #===========================================================================
    # FINAL SUMMARY
    #===========================================================================
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║          BEON-IPQuality Installation Complete! 🎉               ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  CREDENTIALS (SAVE THESE!)${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  Database Password: ${GREEN}${DB_PASSWORD}${NC}"
    echo -e "  API Key:           ${GREEN}${API_KEY}${NC}"
    echo ""
    echo -e "  ${YELLOW}Credentials saved to: ${INSTALL_DIR}/.credentials${NC}"
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  NEXT STEPS${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${GREEN}1.${NC} Setup MaxMind GeoIP (get free key at maxmind.com):"
    echo -e "     ${CYAN}cp ${INSTALL_DIR}/configs/GeoIP.conf.example ${INSTALL_DIR}/configs/GeoIP.conf${NC}"
    echo -e "     ${CYAN}nano ${INSTALL_DIR}/configs/GeoIP.conf${NC}"
    echo -e "     ${CYAN}${INSTALL_DIR}/scripts/update-geoip.sh${NC}"
    echo ""
    echo -e "  ${GREEN}2.${NC} Run initial data ingestion:"
    echo -e "     ${CYAN}sudo -u beon ${INSTALL_DIR}/bin/ingestor -config ${INSTALL_DIR}/configs/config.yaml${NC}"
    echo ""
    echo -e "  ${GREEN}3.${NC} Compile MMDB database:"
    echo -e "     ${CYAN}sudo -u beon ${INSTALL_DIR}/bin/compiler -config ${INSTALL_DIR}/configs/config.yaml${NC}"
    echo ""
    echo -e "  ${GREEN}4.${NC} Start the API server:"
    echo -e "     ${CYAN}sudo systemctl start beon-api${NC}"
    echo -e "     ${CYAN}sudo systemctl enable beon-api${NC}"
    echo ""
    echo -e "  ${GREEN}5.${NC} Configure your domain (RECOMMENDED):"
    echo -e "     ${CYAN}sudo ${INSTALL_DIR}/scripts/setup-domain.sh --domain api.yourdomain.com --email you@email.com${NC}"
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  SERVICE STATUS${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  PostgreSQL: $(systemctl is-active postgresql)"
    echo -e "  Redis:      $(systemctl is-active redis-server)"
    echo -e "  Nginx:      $(systemctl is-active nginx)"
    echo -e "  API:        $(systemctl is-active beon-api 2>/dev/null || echo 'not started')"
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  DIRECTORIES${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  Install:  ${INSTALL_DIR}"
    echo -e "  Data:     ${DATA_DIR}"
    echo -e "  Logs:     ${LOG_DIR}"
    echo -e "  Config:   ${INSTALL_DIR}/configs/config.yaml"
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  QUICK TEST${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  After starting the API, test with:"
    echo -e "  ${CYAN}curl http://localhost/health${NC}"
    echo -e "  ${CYAN}curl \"http://localhost/api/v1/check?ip=8.8.8.8\"${NC}"
    echo ""
    echo -e "${GREEN}Installation complete! 🚀${NC}"
    echo ""
}

# Run main function
main "$@"
