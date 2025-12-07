#!/bin/bash
#===============================================================================
# BEON-IPQuality Domain Configuration Script
# Author: BEON System
# Description: Setup domain dan SSL untuk Ubuntu VPS
#===============================================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

print_header() {
    echo -e "\n${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC} ${YELLOW}$1${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}\n"
}

print_step() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_info() {
    echo -e "${BLUE}[i]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

#===============================================================================
# Configuration Variables
#===============================================================================
DOMAIN=""
EMAIL=""
SKIP_SSL=false
INSTALL_DIR="/opt/beon-ipquality"
NGINX_CONF_DIR="/etc/nginx"

# Service ports (internal)
API_INTERNAL_PORT=8080
JUDGE_INTERNAL_PORT=8082
GRAFANA_INTERNAL_PORT=3001
PROMETHEUS_INTERNAL_PORT=9091
METRICS_INTERNAL_PORT=9101

# External ports (akses publik)
API_EXTERNAL_PORT=80
GRAFANA_EXTERNAL_PORT=3000
JUDGE_EXTERNAL_PORT=8081
PROMETHEUS_EXTERNAL_PORT=9090
METRICS_EXTERNAL_PORT=9100

#===============================================================================
# Parse Arguments
#===============================================================================
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

BEON-IPQuality Domain Configuration Script

Options:
    -d, --domain DOMAIN      Domain name (required), e.g., api.hutang.io
    -e, --email EMAIL        Email for Let's Encrypt (required for SSL)
    --skip-ssl               Skip SSL setup
    -h, --help               Show this help message

Examples:
    # Setup domain dengan SSL
    $0 --domain api.hutang.io --email admin@hutang.io
    
    # Setup domain tanpa SSL (development)
    $0 --domain api.hutang.io --skip-ssl

EOF
    exit 1
}

while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--domain)
            DOMAIN="$2"
            shift 2
            ;;
        -e|--email)
            EMAIL="$2"
            shift 2
            ;;
        --skip-ssl)
            SKIP_SSL=true
            shift
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

# Validate required arguments
if [ -z "$DOMAIN" ]; then
    print_error "Domain is required!"
    usage
fi

if [ "$SKIP_SSL" = false ] && [ -z "$EMAIL" ]; then
    print_error "Email is required for SSL setup!"
    usage
fi

#===============================================================================
# Check Root
#===============================================================================
if [ "$EUID" -ne 0 ]; then
    print_error "Please run as root (sudo)"
    exit 1
fi

print_header "BEON-IPQuality Domain Configuration"
echo -e "${CYAN}Domain:${NC} $DOMAIN"
echo -e "${CYAN}Email:${NC} ${EMAIL:-N/A}"
echo -e "${CYAN}SSL:${NC} $([ "$SKIP_SSL" = true ] && echo "Skipped" || echo "Enabled")"
echo ""

#===============================================================================
# Create Nginx Configuration
#===============================================================================
print_header "Generating Nginx Configuration"

# Backup existing config
if [ -f "${NGINX_CONF_DIR}/sites-available/beon-ipquality" ]; then
    cp "${NGINX_CONF_DIR}/sites-available/beon-ipquality" "${NGINX_CONF_DIR}/sites-available/beon-ipquality.bak.$(date +%Y%m%d%H%M%S)"
    print_step "Backed up existing configuration"
fi

# Generate main config
cat > "${NGINX_CONF_DIR}/sites-available/beon-ipquality" << EOF
#===============================================================================
# BEON-IPQuality Nginx Configuration
# Domain: ${DOMAIN}
# Generated: $(date)
#===============================================================================

# Rate limiting zones
limit_req_zone \$binary_remote_addr zone=api_limit:10m rate=100r/s;
limit_req_zone \$binary_remote_addr zone=dashboard_limit:10m rate=30r/s;

# Upstream definitions
upstream ipquality_api {
    server 127.0.0.1:${API_INTERNAL_PORT};
    keepalive 64;
}

upstream ipquality_judge {
    server 127.0.0.1:${JUDGE_INTERNAL_PORT};
    keepalive 16;
}

upstream grafana {
    server 127.0.0.1:${GRAFANA_INTERNAL_PORT};
    keepalive 16;
}

upstream prometheus {
    server 127.0.0.1:${PROMETHEUS_INTERNAL_PORT};
    keepalive 8;
}

#===============================================================================
# MAIN API SERVER - ${DOMAIN} (Port 80)
#===============================================================================
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    # Logging
    access_log /var/log/nginx/beon-api-access.log;
    error_log /var/log/nginx/beon-api-error.log;

    # Health check (no rate limit)
    location /health {
        proxy_pass http://ipquality_api/health;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        access_log off;
    }

    # API Endpoints
    location / {
        proxy_pass http://ipquality_api;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Connection "";
        
        # Rate limiting
        limit_req zone=api_limit burst=200 nodelay;
        limit_req_status 429;
        
        # Timeouts
        proxy_connect_timeout 5s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
        
        # Buffer settings for low latency
        proxy_buffering on;
        proxy_buffer_size 4k;
        proxy_buffers 8 4k;

        # CORS Headers
        add_header Access-Control-Allow-Origin "*" always;
        add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Authorization, Content-Type, X-API-Key" always;

        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }

    # Internal endpoints - restricted
    location /api/v1/reload {
        allow 127.0.0.1;
        allow 10.0.0.0/8;
        allow 192.168.0.0/16;
        deny all;
        proxy_pass http://ipquality_api;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }

    # Metrics endpoint - restricted
    location /metrics {
        allow 127.0.0.1;
        allow 10.0.0.0/8;
        allow 192.168.0.0/16;
        # Tambahkan IP monitoring server kamu di sini
        # allow YOUR_PROMETHEUS_IP;
        deny all;
        proxy_pass http://ipquality_api/metrics;
        proxy_http_version 1.1;
    }
}

#===============================================================================
# GRAFANA DASHBOARD - ${DOMAIN}:3000
#===============================================================================
server {
    listen ${GRAFANA_EXTERNAL_PORT};
    listen [::]:${GRAFANA_EXTERNAL_PORT};
    server_name ${DOMAIN};

    access_log /var/log/nginx/beon-grafana-access.log;
    error_log /var/log/nginx/beon-grafana-error.log;

    location / {
        proxy_pass http://grafana;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        
        limit_req zone=dashboard_limit burst=50 nodelay;
    }

    # Grafana API
    location /api {
        proxy_pass http://grafana;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }
}

#===============================================================================
# JUDGE NODE - ${DOMAIN}:8081
#===============================================================================
server {
    listen ${JUDGE_EXTERNAL_PORT};
    listen [::]:${JUDGE_EXTERNAL_PORT};
    server_name ${DOMAIN};

    access_log /var/log/nginx/beon-judge-access.log;
    error_log /var/log/nginx/beon-judge-error.log;

    # Optional: Restrict access
    # allow YOUR_ADMIN_IP;
    # allow 127.0.0.1;
    # deny all;

    location / {
        proxy_pass http://ipquality_judge;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Connection "";
        
        # WebSocket support
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}

#===============================================================================
# PROMETHEUS - ${DOMAIN}:9090
#===============================================================================
server {
    listen ${PROMETHEUS_EXTERNAL_PORT};
    listen [::]:${PROMETHEUS_EXTERNAL_PORT};
    server_name ${DOMAIN};

    access_log /var/log/nginx/beon-prometheus-access.log;
    error_log /var/log/nginx/beon-prometheus-error.log;

    # RECOMMENDED: Enable basic auth for security
    # auth_basic "Prometheus";
    # auth_basic_user_file /etc/nginx/.htpasswd-prometheus;

    # RECOMMENDED: Restrict access
    # allow YOUR_ADMIN_IP;
    # allow 127.0.0.1;
    # deny all;

    location / {
        proxy_pass http://prometheus;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
}

#===============================================================================
# METRICS EXPORTER - ${DOMAIN}:9100
#===============================================================================
server {
    listen ${METRICS_EXTERNAL_PORT};
    listen [::]:${METRICS_EXTERNAL_PORT};
    server_name ${DOMAIN};

    # RECOMMENDED: Restrict access
    # allow YOUR_PROMETHEUS_IP;
    # allow 127.0.0.1;
    # deny all;

    location /metrics {
        proxy_pass http://127.0.0.1:${METRICS_INTERNAL_PORT}/metrics;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
    }
}
EOF

print_step "Generated Nginx configuration"

# Enable site
ln -sf "${NGINX_CONF_DIR}/sites-available/beon-ipquality" "${NGINX_CONF_DIR}/sites-enabled/beon-ipquality"

# Remove default if exists
if [ -f "${NGINX_CONF_DIR}/sites-enabled/default" ]; then
    rm -f "${NGINX_CONF_DIR}/sites-enabled/default"
    print_step "Removed default Nginx site"
fi

# Test Nginx config
print_info "Testing Nginx configuration..."
nginx -t
print_step "Nginx configuration is valid"

#===============================================================================
# Configure Firewall
#===============================================================================
print_header "Configuring Firewall"

# Check if ufw is installed
if command -v ufw &> /dev/null; then
    # Allow necessary ports
    ufw allow 22/tcp comment 'SSH'
    ufw allow 80/tcp comment 'HTTP - API'
    ufw allow 443/tcp comment 'HTTPS - API'
    ufw allow ${GRAFANA_EXTERNAL_PORT}/tcp comment 'Grafana Dashboard'
    ufw allow ${JUDGE_EXTERNAL_PORT}/tcp comment 'Judge Node'
    ufw allow ${PROMETHEUS_EXTERNAL_PORT}/tcp comment 'Prometheus'
    ufw allow ${METRICS_EXTERNAL_PORT}/tcp comment 'Metrics Exporter'
    
    # Enable firewall if not already
    if ufw status | grep -q "inactive"; then
        print_warning "Enabling UFW firewall..."
        ufw --force enable
    fi
    
    ufw reload
    print_step "Firewall configured"
    
    echo ""
    echo "Firewall rules:"
    ufw status numbered
else
    print_warning "UFW not installed. Please configure your firewall manually."
    echo ""
    echo "Required ports:"
    echo "  - 22/tcp   : SSH"
    echo "  - 80/tcp   : HTTP (API)"
    echo "  - 443/tcp  : HTTPS (API)"
    echo "  - ${GRAFANA_EXTERNAL_PORT}/tcp : Grafana"
    echo "  - ${JUDGE_EXTERNAL_PORT}/tcp : Judge Node"
    echo "  - ${PROMETHEUS_EXTERNAL_PORT}/tcp : Prometheus"
    echo "  - ${METRICS_EXTERNAL_PORT}/tcp : Metrics"
fi

#===============================================================================
# Update Service Configurations
#===============================================================================
print_header "Updating Service Configurations"

# Update systemd service to use internal ports
if [ -f "/etc/systemd/system/beon-api.service" ]; then
    # Check if we need to update the port
    if grep -q "PORT=8080" /etc/systemd/system/beon-api.service; then
        print_step "API service already configured correctly"
    else
        print_info "API service using default port 8080"
    fi
fi

# Update Grafana port if installed
if [ -f "/etc/grafana/grafana.ini" ]; then
    sed -i "s/^;http_port = 3000/http_port = ${GRAFANA_INTERNAL_PORT}/" /etc/grafana/grafana.ini
    sed -i "s/^http_port = 3000/http_port = ${GRAFANA_INTERNAL_PORT}/" /etc/grafana/grafana.ini
    print_step "Updated Grafana to use internal port ${GRAFANA_INTERNAL_PORT}"
fi

# Update Prometheus port if installed
if [ -f "/etc/prometheus/prometheus.yml" ]; then
    print_info "Prometheus uses default port 9090, Nginx proxies from ${PROMETHEUS_EXTERNAL_PORT}"
fi

#===============================================================================
# SSL Setup with Let's Encrypt
#===============================================================================
if [ "$SKIP_SSL" = false ]; then
    print_header "Setting up SSL with Let's Encrypt"
    
    # Install certbot if not exists
    if ! command -v certbot &> /dev/null; then
        print_info "Installing Certbot..."
        apt-get update
        apt-get install -y certbot python3-certbot-nginx
    fi
    
    # Stop Nginx temporarily
    systemctl stop nginx
    
    # Get certificate
    print_info "Obtaining SSL certificate for ${DOMAIN}..."
    certbot certonly --standalone \
        --non-interactive \
        --agree-tos \
        --email "$EMAIL" \
        -d "$DOMAIN"
    
    if [ $? -eq 0 ]; then
        print_step "SSL certificate obtained successfully"
        
        # Generate HTTPS configuration
        cat >> "${NGINX_CONF_DIR}/sites-available/beon-ipquality" << EOF

#===============================================================================
# HTTPS API SERVER - ${DOMAIN} (Port 443)
#===============================================================================
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name ${DOMAIN};

    # SSL Configuration
    ssl_certificate /etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_stapling on;
    ssl_stapling_verify on;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;

    # Logging
    access_log /var/log/nginx/beon-api-ssl-access.log;
    error_log /var/log/nginx/beon-api-ssl-error.log;

    # Health check
    location /health {
        proxy_pass http://ipquality_api/health;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        access_log off;
    }

    # API Endpoints
    location / {
        proxy_pass http://ipquality_api;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Connection "";
        
        limit_req zone=api_limit burst=200 nodelay;
        
        proxy_connect_timeout 5s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
        
        proxy_buffering on;
        proxy_buffer_size 4k;
        proxy_buffers 8 4k;

        add_header Access-Control-Allow-Origin "*" always;
        add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Authorization, Content-Type, X-API-Key" always;

        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }

    # Internal endpoints
    location /api/v1/reload {
        allow 127.0.0.1;
        allow 10.0.0.0/8;
        allow 192.168.0.0/16;
        deny all;
        proxy_pass http://ipquality_api;
        proxy_http_version 1.1;
    }

    location /metrics {
        allow 127.0.0.1;
        allow 10.0.0.0/8;
        allow 192.168.0.0/16;
        deny all;
        proxy_pass http://ipquality_api/metrics;
        proxy_http_version 1.1;
    }
}

# HTTP to HTTPS redirect
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};
    return 301 https://\$server_name\$request_uri;
}
EOF

        # Setup auto-renewal
        if [ ! -f "/etc/cron.d/certbot-renewal" ]; then
            cat > /etc/cron.d/certbot-renewal << 'CRON'
# Renew SSL certificates twice daily
0 0,12 * * * root certbot renew --quiet --post-hook "systemctl reload nginx"
CRON
            print_step "Configured SSL auto-renewal"
        fi
        
    else
        print_error "Failed to obtain SSL certificate"
        print_warning "You can retry later with: certbot --nginx -d ${DOMAIN}"
    fi
    
    # Start Nginx
    systemctl start nginx
else
    print_warning "SSL setup skipped. API will be accessible via HTTP only."
fi

#===============================================================================
# Restart Services
#===============================================================================
print_header "Restarting Services"

# Reload systemd
systemctl daemon-reload

# Restart Nginx
systemctl restart nginx
print_step "Nginx restarted"

# Restart other services if they exist
for service in beon-api beon-judge grafana-server prometheus; do
    if systemctl is-enabled --quiet $service 2>/dev/null; then
        systemctl restart $service
        print_step "Restarted $service"
    fi
done

#===============================================================================
# Final Summary
#===============================================================================
print_header "Configuration Complete!"

echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║                    SERVICE ACCESS URLS                       ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║${NC} ${CYAN}Main API:${NC}      http://${DOMAIN}                      ${GREEN}║${NC}"
if [ "$SKIP_SSL" = false ]; then
echo -e "${GREEN}║${NC} ${CYAN}API (HTTPS):${NC}   https://${DOMAIN}                     ${GREEN}║${NC}"
fi
echo -e "${GREEN}║${NC} ${CYAN}Grafana:${NC}       http://${DOMAIN}:${GRAFANA_EXTERNAL_PORT}                 ${GREEN}║${NC}"
echo -e "${GREEN}║${NC} ${CYAN}Judge Node:${NC}    http://${DOMAIN}:${JUDGE_EXTERNAL_PORT}                 ${GREEN}║${NC}"
echo -e "${GREEN}║${NC} ${CYAN}Prometheus:${NC}    http://${DOMAIN}:${PROMETHEUS_EXTERNAL_PORT}                 ${GREEN}║${NC}"
echo -e "${GREEN}║${NC} ${CYAN}Metrics:${NC}       http://${DOMAIN}:${METRICS_EXTERNAL_PORT}/metrics       ${GREEN}║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"

echo ""
echo -e "${YELLOW}API Usage Examples:${NC}"
echo ""
echo "# Check IP reputation"
echo "curl \"http://${DOMAIN}/api/v1/check?ip=8.8.8.8\""
echo ""
echo "# Batch check"
echo "curl -X POST \"http://${DOMAIN}/api/v1/batch\" \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"ips\": [\"8.8.8.8\", \"1.1.1.1\"]}'"
echo ""
echo "# Health check"
echo "curl \"http://${DOMAIN}/health\""
echo ""

echo -e "${YELLOW}Next Steps:${NC}"
echo "1. Point your domain DNS A record to this server's IP"
echo "2. Test API: curl http://${DOMAIN}/health"
echo "3. Access Grafana: http://${DOMAIN}:${GRAFANA_EXTERNAL_PORT}"
echo "4. Configure monitoring in Grafana"
echo ""

if [ "$SKIP_SSL" = true ]; then
    echo -e "${YELLOW}To enable SSL later:${NC}"
    echo "sudo certbot --nginx -d ${DOMAIN}"
    echo ""
fi

echo -e "${CYAN}Configuration file:${NC} ${NGINX_CONF_DIR}/sites-available/beon-ipquality"
echo -e "${CYAN}Logs:${NC} /var/log/nginx/beon-*"
echo ""
print_step "Domain configuration completed successfully!"
