#!/bin/bash
# =============================================================================
# BEON IP Quality - Auto Update Script
# =============================================================================
# This script automatically:
# 1. Fetches latest threat feeds (ingestor)
# 2. Recompiles MMDB database (compiler)
# 3. Clears Redis cache (to serve fresh data)
# 4. Optionally triggers API hot reload
#
# Run via cron:
#   0 */6 * * * /path/to/auto-update.sh >> /var/log/beon-update.log 2>&1
# =============================================================================

set -e

# Configuration
BEON_DIR="${BEON_DIR:-/opt/beon-ipquality}"
CONFIG_PATH="${CONFIG_PATH:-$BEON_DIR/configs/config.yaml}"
FEEDS_PATH="${FEEDS_PATH:-$BEON_DIR/configs/feeds.yaml}"
LOG_FILE="${LOG_FILE:-/var/log/beon-update.log}"
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"
API_HOST="${API_HOST:-localhost}"
API_PORT="${API_PORT:-8080}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

log_success() {
    log "${GREEN}✓ $1${NC}"
}

log_warning() {
    log "${YELLOW}⚠ $1${NC}"
}

log_error() {
    log "${RED}✗ $1${NC}"
}

# =============================================================================
# Step 1: Run Ingestor to fetch threat feeds
# =============================================================================
run_ingestor() {
    log "Step 1: Fetching threat feeds..."
    
    if [ -f "$BEON_DIR/bin/ingestor" ]; then
        cd "$BEON_DIR"
        ./bin/ingestor -config "$CONFIG_PATH" -feeds "$FEEDS_PATH"
        log_success "Threat feeds updated successfully"
    else
        log_error "Ingestor binary not found at $BEON_DIR/bin/ingestor"
        return 1
    fi
}

# =============================================================================
# Step 2: Run Compiler to rebuild MMDB
# =============================================================================
run_compiler() {
    log "Step 2: Compiling MMDB database..."
    
    if [ -f "$BEON_DIR/bin/compiler" ]; then
        cd "$BEON_DIR"
        ./bin/compiler -config "$CONFIG_PATH"
        log_success "MMDB compiled successfully"
    else
        log_error "Compiler binary not found at $BEON_DIR/bin/compiler"
        return 1
    fi
}

# =============================================================================
# Step 3: Clear Redis cache
# =============================================================================
clear_redis_cache() {
    log "Step 3: Clearing Redis cache..."
    
    # Try using redis-cli
    if command -v redis-cli &> /dev/null; then
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" KEYS "ipq:*" | xargs -r redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" DEL
        log_success "Redis cache cleared via redis-cli"
    # Try using docker
    elif docker ps --format '{{.Names}}' | grep -q "beon-redis"; then
        docker exec beon-redis redis-cli KEYS "ipq:*" | xargs -r docker exec beon-redis redis-cli DEL
        log_success "Redis cache cleared via docker"
    # Try using API endpoint
    else
        response=$(curl -s -X DELETE "http://${API_HOST}:${API_PORT}/api/v1/cache" 2>/dev/null)
        if echo "$response" | grep -q "success"; then
            log_success "Redis cache cleared via API"
        else
            log_warning "Could not clear Redis cache automatically"
        fi
    fi
}

# =============================================================================
# Step 4: Trigger API MMDB reload (if hot reload is enabled)
# =============================================================================
trigger_api_reload() {
    log "Step 4: Triggering API MMDB reload..."
    
    response=$(curl -s -X POST "http://${API_HOST}:${API_PORT}/api/v1/reload" 2>/dev/null)
    
    if echo "$response" | grep -q "success\|reloaded"; then
        log_success "API MMDB reloaded successfully"
    else
        log_warning "Hot reload not available. API restart may be required for MMDB changes."
        log_warning "To apply changes immediately, restart the API: systemctl restart beon-api"
    fi
}

# =============================================================================
# Step 5: Update GeoIP databases (weekly)
# =============================================================================
update_geoip() {
    log "Step 5: Checking GeoIP update..."
    
    # Only run GeoIP update on Sundays (day 0)
    if [ "$(date +%u)" = "7" ]; then
        if [ -f "$BEON_DIR/scripts/update-geoip.sh" ]; then
            bash "$BEON_DIR/scripts/update-geoip.sh"
            log_success "GeoIP databases updated"
        else
            log_warning "GeoIP update script not found"
        fi
    else
        log "GeoIP update skipped (runs on Sundays only)"
    fi
}

# =============================================================================
# Main execution
# =============================================================================
main() {
    log "=========================================="
    log "BEON IP Quality - Auto Update Started"
    log "=========================================="
    
    # Run all steps
    run_ingestor || log_error "Ingestor failed"
    run_compiler || log_error "Compiler failed"
    clear_redis_cache
    trigger_api_reload
    update_geoip
    
    log "=========================================="
    log "Auto Update Completed"
    log "=========================================="
}

# Run main function
main "$@"
