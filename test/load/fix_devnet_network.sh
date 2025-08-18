#!/bin/bash

# Smart DevNet Network Configuration Fixer
# Automatically detects and fixes network configuration issues

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=== DevNet Network Configuration Checker ===${NC}"
echo ""

# Function to check if IP is configured
check_ip() {
    local ip=$1
    if ip addr show | grep -q "inet $ip"; then
        return 0
    else
        return 1
    fi
}

# Function to add IP address to loopback
add_loopback_ip() {
    local ip=$1
    echo -e "${YELLOW}Adding $ip to loopback interface...${NC}"
    
    if [ "$EUID" -ne 0 ]; then
        echo -e "${YELLOW}Need sudo privileges to add IP address${NC}"
        sudo ip addr add "$ip/8" dev lo 2>/dev/null || true
    else
        ip addr add "$ip/8" dev lo 2>/dev/null || true
    fi
    
    if check_ip "$ip"; then
        echo -e "${GREEN}✅ Successfully added $ip${NC}"
        return 0
    else
        echo -e "${RED}❌ Failed to add $ip${NC}"
        return 1
    fi
}

# Check current network configuration
echo -e "${BLUE}1. Checking current network configuration...${NC}"
echo ""

# Check what IPs devnet is trying to use
DEVNET_IPS=$(ss -tln 2>/dev/null | grep -E "127\.0\.[0-9]+\.[0-9]+" | grep -oE "127\.0\.[0-9]+\.[0-9]+" | sort -u)

if [ -z "$DEVNET_IPS" ]; then
    echo -e "${YELLOW}No devnet processes found listening.${NC}"
    echo -e "${YELLOW}Checking devnet configuration...${NC}"
    
    # Check if devnet.go uses 127.0.1.1
    if grep -q "127.0.1.1" "$ROOT_DIR/cmd/accumulated/run/devnet.go"; then
        echo -e "${YELLOW}DevNet is configured to use 127.0.1.x addresses${NC}"
        DEVNET_IPS="127.0.1.1 127.0.1.8 127.0.1.10 127.0.1.12 127.0.1.13"
    else
        echo -e "${GREEN}DevNet is configured to use 127.0.0.x addresses${NC}"
        DEVNET_IPS="127.0.0.1"
    fi
else
    echo -e "${BLUE}Found devnet listening on:${NC}"
    echo "$DEVNET_IPS" | while read ip; do
        echo "  - $ip"
    done
fi

echo ""
echo -e "${BLUE}2. Checking IP availability...${NC}"
echo ""

MISSING_IPS=""
for ip in $DEVNET_IPS; do
    if check_ip "$ip"; then
        echo -e "${GREEN}✅ $ip is configured${NC}"
    else
        echo -e "${RED}❌ $ip is NOT configured${NC}"
        MISSING_IPS="$MISSING_IPS $ip"
    fi
done

echo ""

# Fix missing IPs
if [ ! -z "$MISSING_IPS" ]; then
    echo -e "${BLUE}3. Fixing network configuration...${NC}"
    echo ""
    
    echo -e "${YELLOW}The following IPs need to be added:${NC}"
    for ip in $MISSING_IPS; do
        echo "  - $ip"
    done
    echo ""
    
    read -p "Do you want to add these IPs to your loopback interface? (y/n) " -n 1 -r
    echo
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        for ip in $MISSING_IPS; do
            add_loopback_ip "$ip"
        done
    else
        echo -e "${YELLOW}Skipping IP configuration.${NC}"
        echo -e "${YELLOW}You can manually add IPs with:${NC}"
        for ip in $MISSING_IPS; do
            echo "  sudo ip addr add $ip/8 dev lo"
        done
    fi
else
    echo -e "${GREEN}3. Network configuration is correct!${NC}"
fi

echo ""
echo -e "${BLUE}4. Testing connectivity...${NC}"
echo ""

# Test if we can connect to common devnet ports
TEST_PORTS="26660 26760 26860 26960"
WORKING_ENDPOINTS=""

for port in $TEST_PORTS; do
    # Try different IP ranges
    for ip_base in "127.0.0" "127.0.1"; do
        for i in 1 8 10 12 13; do
            ip="$ip_base.$i"
            if timeout 1 bash -c "echo > /dev/tcp/$ip/$port" 2>/dev/null; then
                endpoint="http://$ip:$port/v3"
                echo -e "${GREEN}✅ Found working endpoint: $endpoint${NC}"
                WORKING_ENDPOINTS="$WORKING_ENDPOINTS $endpoint"
                break 2
            fi
        done
    done
done

if [ -z "$WORKING_ENDPOINTS" ]; then
    echo -e "${RED}❌ No working endpoints found${NC}"
    echo ""
    echo -e "${YELLOW}Troubleshooting steps:${NC}"
    echo "1. Check if devnet is running: ps aux | grep accumulated"
    echo "2. Check listening ports: ss -tln | grep 266"
    echo "3. Try starting devnet: ./devnet_config.sh start"
else
    echo ""
    echo -e "${GREEN}✅ DevNet connectivity verified!${NC}"
    echo ""
    echo -e "${BLUE}Working endpoints:${NC}"
    for endpoint in $WORKING_ENDPOINTS; do
        echo "  export DEVNET_ENDPOINT=$endpoint"
        break  # Just show the first one
    done
fi

echo ""
echo -e "${BLUE}5. Creating discovery file...${NC}"

DISCOVERY_FILE="$ROOT_DIR/.devnet-test/devnet-discovery.json"
mkdir -p "$(dirname "$DISCOVERY_FILE")"

# Create a simple discovery file
cat > "$DISCOVERY_FILE" <<EOF
{
  "endpoints": {
    "primary": "${WORKING_ENDPOINTS%% *}"
  },
  "updated": "$(date -Iseconds)",
  "network_config": {
    "base_ip": "${DEVNET_IPS%% *}",
    "configured_ips": [
$(echo "$DEVNET_IPS" | sed 's/^/      "/; s/$/",/' | sed '$ s/,$//')
    ]
  }
}
EOF

echo -e "${GREEN}✅ Discovery file created at: $DISCOVERY_FILE${NC}"

echo ""
echo -e "${BLUE}=== Configuration Complete ===${NC}"
echo ""

if [ ! -z "$WORKING_ENDPOINTS" ]; then
    echo -e "${GREEN}Your devnet is properly configured and accessible!${NC}"
    echo ""
    echo "You can now run tests with:"
    echo "  cd $SCRIPT_DIR"
    echo "  go test -v -run TestSmartDevnet"
else
    echo -e "${YELLOW}DevNet is not currently accessible.${NC}"
    echo ""
    echo "To start devnet:"
    echo "  cd $SCRIPT_DIR"
    echo "  ./devnet_config.sh start"
    echo ""
    echo "Or fix the configuration:"
    echo "  1. Edit $ROOT_DIR/cmd/accumulated/run/devnet.go"
    echo "  2. Change devNetDefaultHost to use 127.0.0.1"
    echo "  3. Rebuild and restart devnet"
fi