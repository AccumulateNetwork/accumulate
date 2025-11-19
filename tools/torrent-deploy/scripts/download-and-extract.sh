#!/bin/bash
set -e

# Download complete /volumes directory structure via torrent and extract
# The .tar.gz contains the complete /volumes tree from a running follower
# Usage: docker run -v /path/to/output:/output bootstrap-image MAGNET_LINK

MAGNET_LINK="${1}"
OUTPUT_DIR="${2:-/output}"
DOWNLOAD_DIR="/tmp/download"

if [ -z "$MAGNET_LINK" ]; then
    echo "Error: MAGNET_LINK required"
    echo "Usage: $0 <magnet-link> [output-dir]"
    exit 1
fi

echo "=== Accumulate Complete Volumes Bootstrap ==="
echo "Output: $OUTPUT_DIR"
echo ""

mkdir -p "$DOWNLOAD_DIR"
mkdir -p "$OUTPUT_DIR"

# Download via aria2c
echo "Downloading complete volumes structure via torrent..."
aria2c \
    --dir="$DOWNLOAD_DIR" \
    --seed-time=0 \
    --max-upload-limit=1K \
    --bt-max-peers=50 \
    --enable-dht=true \
    --bt-enable-lpd=true \
    --follow-torrent=mem \
    "$MAGNET_LINK"

# Find the downloaded .tar.gz file
TARBALL=$(find "$DOWNLOAD_DIR" -name "*.tar.gz" -type f | head -1)

if [ -z "$TARBALL" ]; then
    echo "Error: No .tar.gz file found in download"
    exit 1
fi

echo "Found: $(basename $TARBALL)"
echo "Size: $(du -h $TARBALL | cut -f1)"
echo "Extracting complete /volumes structure to $OUTPUT_DIR..."

# Extract with progress - this contains everything under /volumes
pv "$TARBALL" | pigz -dc | tar -xf - -C "$OUTPUT_DIR"

echo "Extraction complete!"
echo "Complete volumes structure ready at: $OUTPUT_DIR"
echo "Contents:"
ls -lh "$OUTPUT_DIR"

# Cleanup
rm -rf "$DOWNLOAD_DIR"
