# accman Torrent Option - Usage Guide

## Overview

When a user installs Accumulate via accman, they can select the **torrent option** to:
1. Download volumes via torrent (if not present)
2. Install Accumulate with those volumes
3. **Automatically seed to help the network**

Every installation becomes a seeder, creating a distributed network for fast installations.

## User Experience

### When user runs accman:

```bash
$ accman install

Accumulate Installation Manager
================================

Select installation method:
  1. Genesis sync (slow, full history)
  2. Latest snapshot (fast, requires snapshot server)
  3. Torrent network (fast, distributed, helps network)  ← New option

Choice: 3

Using torrent network installation...

Magnetic link: magnet:?xt=urn:btih:abc123...
Download directory: /var/lib/accumulate/downloads

Checking for existing volumes file...
✗ Not found locally

Downloading via torrent network...
Progress: 15.2% (6.8 GB / 45 GB) | Peers: 12 | Download: 8.5 MB/s
Progress: 42.8% (19.3 GB / 45 GB) | Peers: 15 | Download: 12.1 MB/s
...
✓ Download complete!

Extracting volumes...
✓ Volumes extracted to: /var/lib/accumulate/volumes

Installing Accumulate...
✓ Accumulate installed

Starting torrent seeding service...
✓ Now helping other nodes download (seeding)

Installation complete!
You are contributing to the network by seeding the volumes file.
```

## How accman Implements This

### Step 1: Provide Magnetic Link

accman needs to know the magnetic link. Options:

**Option A: Hardcoded in accman**
```go
const MAINNET_VOLUMES_MAGNET = "magnet:?xt=urn:btih:abc123..."
```

**Option B: Fetch from bootstrap server**
```bash
MAGNET=$(curl https://bootstrap.accumulate.network/magnet-link)
```

**Option C: Config file**
```yaml
# accman.yaml
torrent:
  magnet_link: "magnet:?xt=urn:btih:abc123..."
```

### Step 2: Call Torrent Installation Script

```bash
# In accman's install function
MAGNET_LINK="$VOLUMES_MAGNET" \
DOWNLOAD_DIR="/var/lib/accumulate/downloads" \
EXTRACT_DIR="/var/lib/accumulate/volumes" \
/path/to/accman-torrent-option.sh
```

This script:
1. Checks if volumes file already exists
2. Downloads via torrent if needed
3. Extracts volumes
4. Installs seeding service
5. Returns control to accman

### Step 3: Continue Normal Installation

accman continues with Accumulate installation using volumes from `/var/lib/accumulate/volumes`

## Components

### Torrent Client (`cmd/torrent-client/main.go`)

Downloads AND seeds files:

```bash
# Download first, then seed
./accumulate-torrent-client \
  -magnet "magnet:?xt=urn:btih:..." \
  -download /var/lib/accumulate/downloads \
  -port 51413 \
  -seed-after=true
```

Features:
- Downloads if file doesn't exist
- Shows progress bar
- Automatically seeds after download
- Can run as daemon

### Installation Script (`accman-torrent-option.sh`)

What accman calls:

```bash
MAGNET_LINK="..." ./accman-torrent-option.sh
```

Does:
1. ✓ Check for existing file
2. ✓ Download via torrent if needed
3. ✓ Extract volumes
4. ✓ Install seeding service
5. ✓ Configure firewall

### Seeding Service

After installation, systemd service runs:

```ini
[Service]
ExecStart=/usr/bin/accumulate-torrent-client \
  -magnet "..." \
  -download /var/lib/accumulate/downloads \
  -seed-after=true
```

This keeps the node seeding to help others.

## Network Effect

```
First Install (Slow)
====================
User 1 ──→ Downloads from original seeder
         └→ Becomes seeder

Second Install (Faster)
=======================
User 2 ──→ Downloads from: Original + User 1
         └→ Becomes seeder

Third Install (Even Faster)
===========================
User 3 ──→ Downloads from: Original + User 1 + User 2
         └→ Becomes seeder

Nth Install (Very Fast)
=======================
User N ──→ Downloads from: Many seeders
         └→ Becomes seeder

= Distributed, self-scaling network
```

## Example: accman Menu

```go
// In accman code
func promptInstallMethod() string {
    fmt.Println("Select installation method:")
    fmt.Println("  1. Genesis sync")
    fmt.Println("  2. Snapshot")
    fmt.Println("  3. Torrent (recommended)")

    var choice int
    fmt.Scanln(&choice)

    switch choice {
    case 3:
        return "torrent"
    default:
        return "genesis"
    }
}

func installWithTorrent() error {
    magnetLink := getMagnetLink() // From config/server

    cmd := exec.Command("/path/to/accman-torrent-option.sh")
    cmd.Env = append(os.Environ(),
        "MAGNET_LINK="+magnetLink,
        "DOWNLOAD_DIR=/var/lib/accumulate/downloads",
        "EXTRACT_DIR=/var/lib/accumulate/volumes",
    )

    return cmd.Run()
}
```

## Configuration

### accman Config File

```yaml
# accman.yaml
installation:
  default_method: torrent

torrent:
  enabled: true
  magnet_link: "magnet:?xt=urn:btih:..."  # Or fetch from server
  port: 51413
  seed_after_install: true
  download_dir: /var/lib/accumulate/downloads
  extract_dir: /var/lib/accumulate/volumes
```

### Environment Variables

```bash
MAGNET_LINK=...              # Required
DOWNLOAD_DIR=...             # Default: /var/lib/accumulate/downloads
EXTRACT_DIR=...              # Default: /var/lib/accumulate/volumes
TORRENT_PORT=51413           # Default: 51413
```

## User Benefits

1. **Fast Installation**: Download from multiple peers simultaneously
2. **No Central Server**: Doesn't rely on single snapshot server
3. **Helps Network**: Your node helps others after installation
4. **Automatic**: Works seamlessly, user just selects option

## Network Benefits

1. **Distributed**: No single point of failure
2. **Scalable**: More nodes = faster downloads
3. **Resilient**: Network grows stronger with each install
4. **Bandwidth Efficient**: Spreads load across many nodes

## Monitoring

Users can check seeding status:

```bash
# Check if seeding
sudo systemctl status accumulate-torrent-seed

# View peer count and upload stats
sudo journalctl -u accumulate-torrent-seed -f

# Stop seeding (optional)
sudo systemctl stop accumulate-torrent-seed
```

## Firewall

Port 51413 (tcp/udp) needs to be open:

```bash
sudo ufw allow 51413/tcp
sudo ufw allow 51413/udp
```

Script does this automatically.

## Future Enhancements

1. **Bandwidth Limits**: Let users cap upload/download speeds
2. **Seeding Schedule**: Only seed during certain hours
3. **Multiple Files**: Support different networks/versions
4. **Progress API**: Real-time progress for accman UI
5. **Metrics**: Track contribution to network

## Testing

Test the full flow:

```bash
# 1. Start a seeder (simulating existing network)
cd tools/torrent-deploy/cmd/torrent-server
./torrent-server -file /path/to/volumes.tar.gz &
MAGNET_LINK=$(cat torrent-data/*.magnet)

# 2. Run accman torrent option
cd ../..
MAGNET_LINK="$MAGNET_LINK" ./accman-torrent-option.sh

# 3. Verify download completed
ls -lh /var/lib/accumulate/downloads/

# 4. Verify seeding service running
sudo systemctl status accumulate-torrent-seed
```

## Summary

**For Users**: Select "Torrent" option in accman → Fast install + help network
**For accman**: Call `accman-torrent-option.sh` with magnetic link → Done
**For Network**: Every install becomes a seeder → Self-scaling distribution
