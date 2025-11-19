#!/bin/bash
set -e

# Create torrent and magnetic link for complete Accumulate volumes .tar.gz
# The .tar.gz should contain the complete /volumes directory structure
# Usage: ./create-torrent.sh <volumes-tarball> [tracker-url]

VOLUMES_TARBALL="${1:-/media/paul/Expansion/accumulate-blockchain/bvn0-production/compressed/BVN0_CLEAN_20241011.tar.gz}"
TRACKER_URL="${2:-udp://tracker.opentrackr.org:1337/announce}"
OUTPUT_DIR="./torrents"
TORRENT_SERVER_IP="${TORRENT_SERVER_IP:-$(hostname -I | awk '{print $1}')}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Accumulate Snapshot Torrent Creator ===${NC}"
echo ""

# Check if file exists
if [ ! -f "$VOLUMES_TARBALL" ]; then
    echo -e "${RED}Error: Volumes tarball not found: $VOLUMES_TARBALL${NC}"
    exit 1
fi

# Get file info
FILE_SIZE=$(du -h "$VOLUMES_TARBALL" | cut -f1)
FILE_NAME=$(basename "$VOLUMES_TARBALL")
FILE_SHA256=$(sha256sum "$VOLUMES_TARBALL" | cut -d' ' -f1)

echo -e "${YELLOW}File:${NC} $FILE_NAME"
echo -e "${YELLOW}Size:${NC} $FILE_SIZE"
echo -e "${YELLOW}SHA256:${NC} $FILE_SHA256"
echo ""

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Check if transmission-create is available
if ! command -v transmission-create &> /dev/null; then
    echo -e "${YELLOW}transmission-create not found. Installing transmission-cli...${NC}"
    sudo apt-get update && sudo apt-get install -y transmission-cli
fi

# Create .torrent file
TORRENT_FILE="$OUTPUT_DIR/${FILE_NAME}.torrent"
echo -e "${GREEN}Creating torrent file...${NC}"

transmission-create \
    -o "$TORRENT_FILE" \
    -t "$TRACKER_URL" \
    -t "udp://tracker.openbittorrent.com:6969/announce" \
    -t "udp://9.rarbg.com:2810/announce" \
    -c "Accumulate Complete Volumes - $(date +%Y-%m-%d)" \
    "$VOLUMES_TARBALL"

if [ ! -f "$TORRENT_FILE" ]; then
    echo -e "${RED}Error: Failed to create torrent file${NC}"
    exit 1
fi

echo -e "${GREEN}Torrent file created: $TORRENT_FILE${NC}"
echo ""

# Generate magnetic link using transmission-show
echo -e "${GREEN}Generating magnetic link...${NC}"

if command -v transmission-show &> /dev/null; then
    MAGNET_LINK=$(transmission-show "$TORRENT_FILE" | grep "magnet:" | cut -d' ' -f2-)

    if [ -z "$MAGNET_LINK" ]; then
        # Alternative: extract hash and build manually
        INFO_HASH=$(transmission-show "$TORRENT_FILE" | grep "Hash:" | awk '{print $2}')
        MAGNET_LINK="magnet:?xt=urn:btih:${INFO_HASH}&dn=${FILE_NAME}&tr=${TRACKER_URL}"
    fi
else
    echo -e "${YELLOW}Warning: transmission-show not available, using simplified magnet link${NC}"
    INFO_HASH=$(openssl dgst -sha1 "$TORRENT_FILE" | awk '{print $2}')
    MAGNET_LINK="magnet:?xt=urn:btih:${INFO_HASH}&dn=${FILE_NAME}&tr=${TRACKER_URL}"
fi

# Save output to files
MAGNET_FILE="$OUTPUT_DIR/${FILE_NAME}.magnet"
INFO_FILE="$OUTPUT_DIR/${FILE_NAME}.info"

echo "$MAGNET_LINK" > "$MAGNET_FILE"

cat > "$INFO_FILE" <<EOF
Accumulate Complete Volumes Archive
==========================================

File: $FILE_NAME
Size: $FILE_SIZE
Date: $(date +"%Y-%m-%d %H:%M:%S")
SHA256: $FILE_SHA256

Torrent File: $TORRENT_FILE
Magnet File: $MAGNET_FILE

Tracker: $TRACKER_URL
Server IP: $TORRENT_SERVER_IP

Instructions
------------
1. Add torrent to server:
   cp "$TORRENT_FILE" ./torrent-config/watch/

2. Or use magnet link:
   transmission-remote localhost:9091 -a "$MAGNET_LINK"

3. Download using client:
   aria2c "$MAGNET_LINK"

4. Verify checksum:
   echo "$FILE_SHA256  $FILE_NAME" | sha256sum -c

EOF

# Display results
echo ""
echo -e "${GREEN}=== Torrent Created Successfully ===${NC}"
echo ""
echo -e "${YELLOW}Torrent File:${NC}"
echo "  $TORRENT_FILE"
echo ""
echo -e "${YELLOW}Magnetic Link:${NC}"
echo "  $MAGNET_LINK"
echo ""
echo -e "${YELLOW}Info saved to:${NC} $INFO_FILE"
echo ""
echo -e "${GREEN}Next Steps:${NC}"
echo "  1. Start torrent server: docker-compose up -d torrent-server"
echo "  2. Add torrent via Web UI: http://${TORRENT_SERVER_IP}:9091"
echo "  3. Or copy torrent file: cp $TORRENT_FILE ./torrent-config/watch/"
echo "  4. Verify it's seeding in the Web UI"
echo ""

# Option to automatically add to local transmission
read -p "Add torrent to local transmission server now? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    if transmission-remote localhost:9091 -a "$TORRENT_FILE" 2>/dev/null; then
        echo -e "${GREEN}Torrent added successfully!${NC}"
    else
        echo -e "${YELLOW}Could not add automatically. Add manually via Web UI or copy to watch folder.${NC}"
    fi
fi
