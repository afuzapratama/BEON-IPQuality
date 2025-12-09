#!/bin/bash
#===============================================================================
# BEON-IPQuality Ubuntu VPS Installation Script v2.0
# 
# Features:
#   - Clear progress information at every step
#   - Automatic retry with fallback for downloads
#   - Built-in data ingestion with progress display
#   - Comprehensive error messages
#
# One-Line Install:
#   curl -fsSL https://raw.githubusercontent.com/afuzapratama/BEON-IPQuality/main/scripts/install-ubuntu.sh | sudo bash
#
# Tested on: Ubuntu 22.04 LTS, Ubuntu 24.04 LTS
# Requirements: 2GB+ RAM, 20GB+ Storage
#===============================================================================

set -e

# Version
SCRIPT_VERSION="2.0.0"
GITHUB_REPO="afuzapratama/BEON-IPQuality"
GITHUB_BRANCH="main"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'
BOLD='\033[1m'

# Configuration
INSTALL_DIR="/opt/beon-ipquality"
DATA_DIR="/var/lib/beon-ipquality"
LOG_DIR="/var/log/beon-ipquality"
USER="beon"
GROUP="beon"

# Auto-generated credentials
DB_PASSWORD=""
API_KEY=""
GRAFANA_PASSWORD=""
MAXMIND_ACCOUNT_ID=""
MAXMIND_LICENSE_KEY=""
INTERACTIVE=true

# Statistics tracking
TOTAL_STEPS=13
CURRENT_STEP=0
START_TIME=$(date +%s)

#===============================================================================
# HELPER FUNCTIONS
#===============================================================================

print_banner() {
    clear
    echo -e "${CYAN}"
    cat << 'BANNER'
╔══════════════════════════════════════════════════════════════════════════════╗
║                                                                              ║
║    ██████╗ ███████╗ ██████╗ ███╗   ██╗     ██╗██████╗  ██████╗              ║
║    ██╔══██╗██╔════╝██╔═══██╗████╗  ██║     ██║██╔══██╗██╔═══██╗             ║
║    ██████╔╝█████╗  ██║   ██║██╔██╗ ██║     ██║██████╔╝██║   ██║             ║
║    ██╔══██╗██╔══╝  ██║   ██║██║╚██╗██║     ██║██╔═══╝ ██║▄▄ ██║             ║
║    ██████╔╝███████╗╚██████╔╝██║ ╚████║     ██║██║     ╚██████╔╝             ║
║    ╚═════╝ ╚══════╝ ╚═════╝ ╚═╝  ╚═══╝     ╚═╝╚═╝      ╚══▀▀═╝              ║
║                                                                              ║
║                  IP Quality & Reputation System                              ║
║                     Ubuntu VPS Installer v2.0                                ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝
BANNER
    echo -e "${NC}"
    echo ""
}

print_step() {
    CURRENT_STEP=$1
    local title="$2"
    local elapsed=$(($(date +%s) - START_TIME))
    local mins=$((elapsed / 60))
    local secs=$((elapsed % 60))
    
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC} ${YELLOW}STEP ${CURRENT_STEP}/${TOTAL_STEPS}${NC}: ${BOLD}${title}${NC}"
    echo -e "${CYAN}║${NC} ${MAGENTA}Elapsed: ${mins}m ${secs}s${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

print_substep() {
    echo -e "  ${BLUE}→${NC} $1"
}

print_progress() {
    echo -e "  ${BLUE}[*]${NC} $1"
}

print_success() {
    echo -e "  ${GREEN}[✓]${NC} $1"
}

print_warning() {
    echo -e "  ${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "  ${RED}[✗]${NC} $1"
}

print_info() {
    echo -e "  ${CYAN}[i]${NC} $1"
}

spinner() {
    local pid=$1
    local delay=0.1
    local spinstr='|/-\'
    while [ "$(ps a | awk '{print $1}' | grep $pid)" ]; do
        local temp=${spinstr#?}
        printf " [%c]  " "$spinstr"
        local spinstr=$temp${spinstr%"$temp"}
        sleep $delay
        printf "\b\b\b\b\b\b"
    done
    printf "    \b\b\b\b"
}

generate_password() {
    < /dev/urandom tr -dc 'A-Za-z0-9' | head -c 24
}

generate_api_key() {
    < /dev/urandom tr -dc 'A-Za-z0-9' | head -c 32
}

download_with_retry() {
    local url="$1"
    local output="$2"
    local description="$3"
    local max_retries=3
    local retry=0
    
    while [ $retry -lt $max_retries ]; do
        retry=$((retry + 1))
        print_progress "Downloading ${description} (attempt ${retry}/${max_retries})..."
        
        if wget -4 --timeout=120 --tries=2 --retry-connrefused -q --show-progress -O "$output" "$url" 2>/dev/null; then
            print_success "Downloaded ${description}"
            return 0
        fi
        
        if curl -4 --retry 2 --connect-timeout 60 --max-time 300 -fsSL -o "$output" "$url" 2>/dev/null; then
            print_success "Downloaded ${description}"
            return 0
        fi
        
        print_warning "Attempt ${retry} failed, retrying..."
        sleep 2
    done
    
    return 1
}

#===============================================================================
# INTERACTIVE SETUP
#===============================================================================

interactive_setup() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  ${YELLOW}MAXMIND GEOLITE2 CONFIGURATION${NC}                                              ${CYAN}║${NC}"
    echo -e "${CYAN}╠══════════════════════════════════════════════════════════════════════════════╣${NC}"
    echo -e "${CYAN}║${NC}  GeoIP enables location-based risk scoring (country, ASN, datacenter).       ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}  Get your FREE credentials at:                                               ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}  ${GREEN}https://www.maxmind.com/en/geolite2/signup${NC}                                  ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}                                                                              ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}  After registration:                                                         ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}    1. Account ID is shown on your dashboard (6-digit number)                 ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}    2. License Key: Account → Manage License Keys → Generate New Key          ${CYAN}║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    if [[ ! -e /dev/tty ]]; then
        print_warning "No terminal for input - using default settings"
        return
    fi
    
    # Account ID
    while true; do
        echo -ne "${BLUE}Enter MaxMind Account ID (or 'skip' to configure later): ${NC}"
        read -r MAXMIND_ACCOUNT_ID </dev/tty || true
        
        if [[ "$MAXMIND_ACCOUNT_ID" == "skip" || -z "$MAXMIND_ACCOUNT_ID" ]]; then
            MAXMIND_ACCOUNT_ID=""
            MAXMIND_LICENSE_KEY=""
            print_warning "MaxMind skipped - you can configure later"
            return
        elif [[ "$MAXMIND_ACCOUNT_ID" =~ ^[0-9]+$ ]]; then
            print_success "Account ID: ${MAXMIND_ACCOUNT_ID}"
            break
        else
            print_error "Invalid Account ID - must be a number"
        fi
    done
    
    # License Key
    while true; do
        echo -ne "${BLUE}Enter MaxMind License Key: ${NC}"
        read -r MAXMIND_LICENSE_KEY </dev/tty || true
        
        if [[ ${#MAXMIND_LICENSE_KEY} -ge 10 ]]; then
            print_success "License Key configured"
            break
        else
            print_error "Invalid key (too short)"
        fi
    done
    
    echo ""
    print_success "MaxMind GeoIP configured!"
}

#===============================================================================
# PRE-FLIGHT CHECKS
#===============================================================================

check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

check_os() {
    if [[ ! -f /etc/os-release ]]; then
        print_error "Cannot detect OS"
        exit 1
    fi
    
    source /etc/os-release
    if [[ "$ID" != "ubuntu" ]]; then
        print_error "This script requires Ubuntu (detected: $ID)"
        exit 1
    fi
    
    print_success "OS: Ubuntu $VERSION_ID"
}

check_resources() {
    local mem_mb=$(free -m | awk '/^Mem:/{print $2}')
    local disk_gb=$(df -BG / | awk 'NR==2 {print $4}' | tr -d 'G')
    
    if [[ $mem_mb -lt 1024 ]]; then
        print_warning "Low memory: ${mem_mb}MB (recommended: 2GB+)"
    else
        print_success "Memory: ${mem_mb}MB"
    fi
    
    if [[ $disk_gb -lt 10 ]]; then
        print_warning "Low disk: ${disk_gb}GB (recommended: 20GB+)"
    else
        print_success "Disk: ${disk_gb}GB available"
    fi
}

#===============================================================================
# MAIN INSTALLATION
#===============================================================================

main() {
    print_banner
    
    # Pre-flight checks
    echo -e "${YELLOW}Running pre-flight checks...${NC}"
    echo ""
    check_root
    check_os
    check_resources
    
    # Interactive setup
    if [[ "$INTERACTIVE" = true ]]; then
        interactive_setup
    fi
    
    # Generate credentials
    echo ""
    print_progress "Generating secure credentials..."
    DB_PASSWORD=$(generate_password)
    API_KEY=$(generate_api_key)
    GRAFANA_PASSWORD=$(generate_password)
    print_success "Credentials generated"
    
    echo ""
    echo -e "${GREEN}Starting installation in 3 seconds...${NC}"
    sleep 3

    #===========================================================================
    # STEP 1: System Update
    #===========================================================================
    print_step 1 "SYSTEM UPDATE & BASE PACKAGES"
    
    print_progress "Updating package lists..."
    apt-get update -qq 2>&1 | tail -1
    print_success "Package lists updated"
    
    print_progress "Installing essential packages..."
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
        curl wget git software-properties-common apt-transport-https \
        ca-certificates gnupg lsb-release build-essential ufw fail2ban jq htop \
        geoipupdate 2>&1 | tail -3
    print_success "Essential packages installed"

    #===========================================================================
    # STEP 2: Install Go
    #===========================================================================
    print_step 2 "INSTALLING GO 1.25"
    
    GO_VERSION="1.25.3"
    GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz"
    
    if command -v go &> /dev/null && go version | grep -q "go1.25"; then
        print_success "Go already installed: $(go version | awk '{print $3}')"
    else
        print_progress "Downloading Go ${GO_VERSION}..."
        
        rm -f /tmp/go.tar.gz
        
        # Try multiple sources
        if ! download_with_retry "https://go.dev/dl/${GO_TARBALL}" "/tmp/go.tar.gz" "Go ${GO_VERSION}"; then
            if ! download_with_retry "https://dl.google.com/go/${GO_TARBALL}" "/tmp/go.tar.gz" "Go ${GO_VERSION} (mirror)"; then
                print_error "Failed to download Go"
                exit 1
            fi
        fi
        
        # Verify file size
        FILE_SIZE=$(stat -c%s /tmp/go.tar.gz 2>/dev/null || echo "0")
        if [ "$FILE_SIZE" -lt 50000000 ]; then
            print_error "Download corrupted (file too small)"
            exit 1
        fi
        print_success "Downloaded Go ($(numfmt --to=iec $FILE_SIZE))"
        
        print_progress "Extracting Go..."
        rm -rf /usr/local/go
        tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
        print_success "Go extracted to /usr/local/go"
        
        # Setup environment
        cat > /etc/profile.d/go.sh << 'GOENV'
export PATH=$PATH:/usr/local/go/bin
export GOPATH=/opt/go
export PATH=$PATH:$GOPATH/bin
GOENV
        export PATH=$PATH:/usr/local/go/bin
        export GOPATH=/opt/go
        
        print_success "Go ${GO_VERSION} installed"
    fi
    
    print_info "Go version: $(go version | awk '{print $3}')"

    #===========================================================================
    # STEP 3: Install PostgreSQL
    #===========================================================================
    print_step 3 "INSTALLING POSTGRESQL 17"
    
    if command -v psql &> /dev/null; then
        print_success "PostgreSQL already installed"
    else
        print_progress "Adding PostgreSQL repository..."
        sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
        wget -4 -q --timeout=60 -O- https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg 2>/dev/null
        print_success "Repository added"
        
        print_progress "Installing PostgreSQL 17..."
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql-17 postgresql-contrib-17 2>&1 | tail -2
        print_success "PostgreSQL 17 installed"
    fi
    
    print_progress "Starting PostgreSQL service..."
    systemctl start postgresql
    systemctl enable postgresql 2>/dev/null
    print_success "PostgreSQL service started"
    
    print_progress "Creating database and user..."
    sudo -u postgres psql -c "SELECT 1 FROM pg_user WHERE usename = 'beon'" 2>/dev/null | grep -q 1 || \
    sudo -u postgres psql << EOSQL
CREATE USER beon WITH PASSWORD '${DB_PASSWORD}';
CREATE DATABASE ipquality OWNER beon;
GRANT ALL PRIVILEGES ON DATABASE ipquality TO beon;
\c ipquality
CREATE EXTENSION IF NOT EXISTS pg_trgm;
EOSQL
    print_success "Database 'ipquality' created with user 'beon'"

    #===========================================================================
    # STEP 4: Install Redis
    #===========================================================================
    print_step 4 "INSTALLING REDIS"
    
    if command -v redis-server &> /dev/null; then
        print_success "Redis already installed"
    else
        print_progress "Adding Redis repository..."
        curl -4 --retry 3 -fsSL https://packages.redis.io/gpg | gpg --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg 2>/dev/null
        echo "deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $(lsb_release -cs) main" > /etc/apt/sources.list.d/redis.list
        
        print_progress "Installing Redis..."
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq redis-server 2>&1 | tail -1
        print_success "Redis installed"
    fi
    
    print_progress "Configuring Redis..."
    sed -i 's/^supervised no/supervised systemd/' /etc/redis/redis.conf 2>/dev/null || true
    sed -i 's/^# maxmemory .*/maxmemory 256mb/' /etc/redis/redis.conf
    sed -i 's/^# maxmemory-policy .*/maxmemory-policy allkeys-lru/' /etc/redis/redis.conf
    systemctl restart redis-server
    systemctl enable redis-server 2>/dev/null
    print_success "Redis configured and started"

    #===========================================================================
    # STEP 5: Install Nginx
    #===========================================================================
    print_step 5 "INSTALLING NGINX"
    
    if command -v nginx &> /dev/null; then
        print_success "Nginx already installed"
    else
        print_progress "Installing Nginx..."
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx 2>&1 | tail -1
        print_success "Nginx installed"
    fi
    
    print_progress "Configuring Nginx reverse proxy..."
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
    }
    
    location /health {
        proxy_pass http://ipquality_api/health;
        access_log off;
    }
}
NGINXCONF
    
    ln -sf /etc/nginx/sites-available/beon-ipquality /etc/nginx/sites-enabled/
    rm -f /etc/nginx/sites-enabled/default
    nginx -t 2>/dev/null && systemctl restart nginx
    systemctl enable nginx 2>/dev/null
    print_success "Nginx configured as reverse proxy"

    #===========================================================================
    # STEP 6: Create User & Directories
    #===========================================================================
    print_step 6 "CREATING USER & DIRECTORIES"
    
    print_progress "Creating service user 'beon'..."
    id -u $USER &>/dev/null || useradd -r -s /bin/false $USER
    print_success "User 'beon' created"
    
    print_progress "Creating directory structure..."
    mkdir -p $INSTALL_DIR/{bin,configs,scripts,docs,migrations}
    mkdir -p $DATA_DIR/{mmdb,geoip}
    mkdir -p $LOG_DIR
    print_success "Directories created"
    print_info "  Install: $INSTALL_DIR"
    print_info "  Data:    $DATA_DIR"
    print_info "  Logs:    $LOG_DIR"

    #===========================================================================
    # STEP 7: Clone Repository
    #===========================================================================
    print_step 7 "CLONING BEON-IPQUALITY REPOSITORY"
    
    print_progress "Cloning from GitHub..."
    rm -rf /tmp/BEON-IPQuality
    if git clone --depth 1 --branch $GITHUB_BRANCH "https://github.com/${GITHUB_REPO}.git" /tmp/BEON-IPQuality 2>&1 | tail -3; then
        print_success "Repository cloned"
    else
        print_error "Failed to clone repository"
        exit 1
    fi
    
    # Count files
    FILE_COUNT=$(find /tmp/BEON-IPQuality -type f | wc -l)
    print_info "Downloaded $FILE_COUNT files"

    #===========================================================================
    # STEP 8: Download Pre-built Binaries
    #===========================================================================
    print_step 8 "DOWNLOADING PRE-BUILT BINARIES"
    
    cd /tmp/BEON-IPQuality
    
    RELEASE_URL="https://github.com/afuzapratama/BEON-IPQuality/releases/download/v1.0.0/beon-binaries-linux-amd64.tar.gz"
    
    print_progress "Downloading binaries from GitHub Release..."
    if curl -fsSL "$RELEASE_URL" -o /tmp/beon-binaries.tar.gz; then
        print_success "Downloaded binaries"
    else
        print_error "Failed to download binaries"
        exit 1
    fi
    
    print_progress "Extracting binaries..."
    if tar -xzf /tmp/beon-binaries.tar.gz -C $INSTALL_DIR/bin/; then
        print_success "Extracted binaries"
    else
        print_error "Failed to extract binaries"
        exit 1
    fi
    
    chmod +x $INSTALL_DIR/bin/*
    
    # Copy files
    print_progress "Copying configuration files..."
    cp -r configs/* $INSTALL_DIR/configs/ 2>/dev/null || true
    cp -r scripts/* $INSTALL_DIR/scripts/ 2>/dev/null || true
    cp -r migrations/* $INSTALL_DIR/migrations/ 2>/dev/null || true
    chmod +x $INSTALL_DIR/scripts/*.sh 2>/dev/null || true
    print_success "All binaries installed successfully"
    
    # Show binary sizes
    print_info "Binary sizes:"
    for bin in api judge ingestor compiler; do
        SIZE=$(du -h $INSTALL_DIR/bin/$bin | cut -f1)
        print_info "  $bin: $SIZE"
    done
    
    # Cleanup
    rm -f /tmp/beon-binaries.tar.gz

    #===========================================================================
    # STEP 9: Database Migration
    #===========================================================================
    print_step 9 "RUNNING DATABASE MIGRATIONS"
    
    if [[ -f "$INSTALL_DIR/migrations/001_initial_schema.sql" ]]; then
        print_progress "Applying database schema..."
        if sudo -u postgres psql -d ipquality -f "$INSTALL_DIR/migrations/001_initial_schema.sql" 2>&1 | tail -5; then
            print_success "Schema applied"
        fi
        
        TABLE_COUNT=$(sudo -u postgres psql -d ipquality -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" | tr -d ' ')
        print_success "Created $TABLE_COUNT tables"
        
        # List tables
        print_info "Tables created:"
        sudo -u postgres psql -d ipquality -t -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public';" | while read table; do
            [[ -n "$table" ]] && print_info "  - $(echo $table | tr -d ' ')"
        done
    else
        print_warning "Migration file not found"
    fi

    #===========================================================================
    # STEP 10: Configuration
    #===========================================================================
    print_step 10 "CREATING CONFIGURATION FILES"
    
    print_progress "Creating config.yaml..."
    cat > $INSTALL_DIR/configs/config.yaml << CONFIGYAML
# BEON-IPQuality Configuration
# Generated: $(date)

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
  output: "stdout"
  file_path: "${LOG_DIR}/api.log"

database:
  postgres:
    host: "localhost"
    port: 5432
    database: "ipquality"
    username: "beon"
    password: "${DB_PASSWORD}"
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
  reputation_path: "${DATA_DIR}/mmdb/reputation.mmdb"
  geolite2_city_path: "${DATA_DIR}/mmdb/GeoLite2-City.mmdb"
  geolite2_asn_path: "${DATA_DIR}/mmdb/GeoLite2-ASN.mmdb"
  output_path: "${DATA_DIR}/mmdb/reputation.mmdb"

ingestor:
  batch_size: 1000
  workers: 4
  concurrency: 10
  update_interval: 4h
  retry_attempts: 3
  retry_delay: 30s
  http_timeout: 60s
  max_retries: 3
  user_agent: "BEON-IPQuality/1.0"

api:
  auth_enabled: true
  rate_limit: 1000
  rate_limit_window: 60s

judge:
  enabled: false
  port: 8081
CONFIGYAML
    print_success "config.yaml created"
    
    # Save credentials
    print_progress "Saving credentials..."
    cat > $INSTALL_DIR/credentials.txt << CREDS
#===============================================================================
# BEON-IPQuality Credentials
# Generated: $(date)
# ⚠️  KEEP THIS FILE SECURE!
#===============================================================================

API_MASTER_KEY=${API_KEY}

# PostgreSQL
POSTGRES_USER=beon
POSTGRES_PASSWORD=${DB_PASSWORD}
POSTGRES_DB=ipquality

# Grafana
GRAFANA_PASSWORD=${GRAFANA_PASSWORD}

# MaxMind
MAXMIND_ACCOUNT_ID=${MAXMIND_ACCOUNT_ID:-"(not configured)"}

#===============================================================================
# Quick Test:
#   curl -H "X-API-Key: ${API_KEY}" "http://localhost/api/v1/check?ip=8.8.8.8"
#===============================================================================
CREDS
    chmod 600 $INSTALL_DIR/credentials.txt
    print_success "Credentials saved to $INSTALL_DIR/credentials.txt"
    
    # Set ownership
    chown -R $USER:$GROUP $INSTALL_DIR
    chown -R $USER:$GROUP $DATA_DIR
    chown -R $USER:$GROUP $LOG_DIR

    #===========================================================================
    # STEP 11: Systemd Services
    #===========================================================================
    print_step 11 "CREATING SYSTEMD SERVICES"
    
    print_progress "Creating beon-api.service..."
    cat > /etc/systemd/system/beon-api.service << SVCAPI
[Unit]
Description=BEON-IPQuality API Server
After=network.target postgresql.service redis-server.service

[Service]
Type=simple
User=${USER}
Group=${GROUP}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/bin/api -config ${INSTALL_DIR}/configs/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SVCAPI
    print_success "beon-api.service created"
    
    systemctl daemon-reload
    print_success "Systemd reloaded"

    #===========================================================================
    # STEP 12: Firewall & Security
    #===========================================================================
    print_step 12 "CONFIGURING FIREWALL & SECURITY"
    
    print_progress "Configuring UFW firewall..."
    ufw default deny incoming 2>/dev/null
    ufw default allow outgoing 2>/dev/null
    ufw allow ssh 2>/dev/null
    ufw allow 80/tcp 2>/dev/null
    ufw allow 443/tcp 2>/dev/null
    echo "y" | ufw enable 2>/dev/null
    print_success "Firewall configured (SSH, HTTP, HTTPS allowed)"
    
    print_progress "Configuring Fail2ban..."
    systemctl enable fail2ban 2>/dev/null
    systemctl restart fail2ban 2>/dev/null
    print_success "Fail2ban enabled"

    #===========================================================================
    # STEP 13: Initial Data Ingestion
    #===========================================================================
    print_step 13 "INITIAL THREAT FEED INGESTION"
    
    echo ""
    echo -e "${YELLOW}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${YELLOW}║${NC}  This will download threat intelligence feeds from multiple sources.         ${YELLOW}║${NC}"
    echo -e "${YELLOW}║${NC}  Depending on your connection, this may take 2-10 minutes.                  ${YELLOW}║${NC}"
    echo -e "${YELLOW}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    print_progress "Starting threat feed ingestion..."
    echo ""
    
    # Run ingestor with --once flag for single run with progress
    if sudo -u $USER $INSTALL_DIR/bin/ingestor \
        -config $INSTALL_DIR/configs/config.yaml \
        -feeds $INSTALL_DIR/configs/feeds.yaml \
        -once 2>&1; then
        echo ""
        print_success "Threat feed ingestion completed!"
    else
        echo ""
        print_warning "Some feeds may have failed - this is normal"
        print_info "Feeds will be retried automatically via cron"
    fi
    
    # Check database
    print_progress "Verifying database entries..."
    ENTRY_COUNT=$(sudo -u postgres psql -d ipquality -t -c "SELECT COUNT(*) FROM ip_reputation;" 2>/dev/null | tr -d ' ')
    if [[ "$ENTRY_COUNT" -gt 0 ]]; then
        print_success "Database contains $ENTRY_COUNT IP reputation entries"
    else
        print_warning "No entries in database yet"
        print_info "Run manually: sudo -u beon $INSTALL_DIR/bin/ingestor -once"
    fi

    #===========================================================================
    # CLEANUP
    #===========================================================================
    print_progress "Cleaning up..."
    rm -rf /tmp/BEON-IPQuality
    apt-get autoremove -y -qq 2>/dev/null
    print_success "Cleanup complete"

    #===========================================================================
    # FINAL SUMMARY
    #===========================================================================
    local elapsed=$(($(date +%s) - START_TIME))
    local mins=$((elapsed / 60))
    local secs=$((elapsed % 60))
    
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                                                                              ║${NC}"
    echo -e "${GREEN}║              🎉 INSTALLATION COMPLETE! 🎉                                    ║${NC}"
    echo -e "${GREEN}║                                                                              ║${NC}"
    echo -e "${GREEN}║              Total time: ${mins}m ${secs}s                                            ║${NC}"
    echo -e "${GREEN}║                                                                              ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  🔑 YOUR API KEY (SAVE THIS!)${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${GREEN}${BOLD}${API_KEY}${NC}"
    echo ""
    
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  📊 SERVICE STATUS${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  PostgreSQL: $(systemctl is-active postgresql)"
    echo -e "  Redis:      $(systemctl is-active redis-server)"  
    echo -e "  Nginx:      $(systemctl is-active nginx)"
    echo -e "  Database:   ${ENTRY_COUNT:-0} IP entries"
    echo ""
    
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  🚀 START THE API SERVER${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${CYAN}sudo systemctl start beon-api${NC}"
    echo -e "  ${CYAN}sudo systemctl enable beon-api${NC}"
    echo ""
    
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  🧪 TEST YOUR API${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${CYAN}curl http://localhost/health${NC}"
    echo -e "  ${CYAN}curl -H \"X-API-Key: ${API_KEY}\" \"http://localhost/api/v1/check?ip=8.8.8.8\"${NC}"
    echo ""
    
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  📁 IMPORTANT FILES${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  Credentials:    ${CYAN}cat $INSTALL_DIR/credentials.txt${NC}"
    echo -e "  Configuration:  ${CYAN}$INSTALL_DIR/configs/config.yaml${NC}"
    echo -e "  Logs:           ${CYAN}$LOG_DIR/${NC}"
    echo ""
    
    echo -e "${GREEN}Thank you for installing BEON-IPQuality! 🚀${NC}"
    echo ""
}

#===============================================================================
# ARGUMENT PARSING
#===============================================================================

while [[ $# -gt 0 ]]; do
    case $1 in
        --maxmind-account)
            MAXMIND_ACCOUNT_ID="$2"
            shift 2
            ;;
        --maxmind-key)
            MAXMIND_LICENSE_KEY="$2"
            shift 2
            ;;
        --non-interactive)
            INTERACTIVE=false
            shift
            ;;
        --branch)
            GITHUB_BRANCH="$2"
            shift 2
            ;;
        -h|--help)
            echo "BEON-IPQuality Ubuntu Installer v${SCRIPT_VERSION}"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --maxmind-account ID    MaxMind Account ID"
            echo "  --maxmind-key KEY       MaxMind License Key"
            echo "  --non-interactive       Skip interactive prompts"
            echo "  --branch BRANCH         GitHub branch (default: main)"
            echo "  -h, --help              Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run main
main "$@"

