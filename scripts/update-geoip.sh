#!/bin/bash
# MaxMind GeoIP Database Update Script
# Run this script to download/update GeoLite2 databases

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$PROJECT_ROOT/configs/GeoIP.conf"
CONFIG_EXAMPLE="$PROJECT_ROOT/configs/GeoIP.conf.example"
DATA_DIR="$PROJECT_ROOT/data/mmdb"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== MaxMind GeoIP Database Updater ===${NC}"
echo "Config: $CONFIG_FILE"
echo "Output: $DATA_DIR"
echo ""

# Create data directory if not exists
mkdir -p "$DATA_DIR"

# Check if config file exists, if not create from example
if [[ ! -f "$CONFIG_FILE" ]]; then
    if [[ -f "$CONFIG_EXAMPLE" ]]; then
        echo -e "${YELLOW}Config file not found. Creating from example...${NC}"
        cp "$CONFIG_EXAMPLE" "$CONFIG_FILE"
        echo -e "${RED}IMPORTANT: Please edit $CONFIG_FILE with your MaxMind credentials!${NC}"
        echo ""
        echo "Get your free license key at: https://www.maxmind.com/en/geolite2/signup"
        echo ""
        echo "Then update these values in $CONFIG_FILE:"
        echo "  AccountID YOUR_ACCOUNT_ID"
        echo "  LicenseKey YOUR_LICENSE_KEY"
        echo ""
        exit 1
    else
        echo -e "${RED}Config file not found: $CONFIG_FILE${NC}"
        echo "Please create the config file with your MaxMind credentials."
        exit 1
    fi
fi

# Check if config has placeholder values
if grep -q "YOUR_ACCOUNT_ID\|YOUR_LICENSE_KEY" "$CONFIG_FILE"; then
    echo -e "${RED}Please update $CONFIG_FILE with your real MaxMind credentials!${NC}"
    echo ""
    echo "Get your free license key at: https://www.maxmind.com/en/geolite2/signup"
    exit 1
fi

# Check if geoipupdate is installed
if ! command -v geoipupdate &> /dev/null; then
    echo -e "${YELLOW}geoipupdate not found. Installing...${NC}"
    
    # Detect OS and install
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Check for package manager
        if command -v apt-get &> /dev/null; then
            echo "Installing via apt..."
            sudo apt-get update
            sudo apt-get install -y geoipupdate
        elif command -v yum &> /dev/null; then
            echo "Installing via yum..."
            sudo yum install -y geoipupdate
        elif command -v pacman &> /dev/null; then
            echo "Installing via pacman..."
            sudo pacman -S geoipupdate
        else
            echo -e "${RED}Please install geoipupdate manually:${NC}"
            echo "  - Ubuntu/Debian: sudo apt install geoipupdate"
            echo "  - RHEL/CentOS: sudo yum install geoipupdate"
            echo "  - Arch: sudo pacman -S geoipupdate"
            echo "  - Or download from: https://github.com/maxmind/geoipupdate/releases"
            exit 1
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        if command -v brew &> /dev/null; then
            echo "Installing via Homebrew..."
            brew install geoipupdate
        else
            echo -e "${RED}Please install Homebrew first, then run: brew install geoipupdate${NC}"
            exit 1
        fi
    else
        echo -e "${RED}Unsupported OS. Please install geoipupdate manually.${NC}"
        exit 1
    fi
fi

# Run geoipupdate
echo -e "${GREEN}Downloading/Updating GeoLite2 databases...${NC}"
echo ""

geoipupdate -f "$CONFIG_FILE" -d "$DATA_DIR" -v

echo ""
echo -e "${GREEN}=== Update Complete ===${NC}"
echo "Downloaded databases:"
ls -lh "$DATA_DIR"/*.mmdb 2>/dev/null || echo "No MMDB files found"

echo ""
echo -e "${GREEN}Database info:${NC}"
for db in "$DATA_DIR"/*.mmdb; do
    if [[ -f "$db" ]]; then
        filename=$(basename "$db")
        size=$(du -h "$db" | cut -f1)
        modified=$(stat -c %y "$db" 2>/dev/null || stat -f %Sm "$db" 2>/dev/null)
        echo "  - $filename ($size) - Modified: $modified"
    fi
done
