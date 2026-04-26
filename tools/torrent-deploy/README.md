# Accumulate Torrent-Based Deployment

## Quick Start

### Build the Torrent Server

```bash
cd tools/torrent-deploy/cmd/torrent-server
go build -o torrent-server .
```

### Run Torrent Server

```bash
# With default file location
./torrent-server

# Or specify file
./torrent-server \
  -file /path/to/volumes.tar.gz \
  -data ./torrent-data \
  -port 51413
```

### Output

The server will:
1. Create a `.torrent` file
2. Generate a magnetic link
3. Save the magnetic link to a `.magnet` file
4. Start seeding
5. Display status every 30 seconds

Example:
```
=== Accumulate Torrent Server ===
Volumes file: /media/paul/Expansion/.../BVN0_CLEAN_20241011.tar.gz
Port: 51413
Data directory: ./torrent-data
Torrent client started on port 51413
Creating torrent file for: BVN0_CLEAN_20241011.tar.gz
Generating .torrent file (this may take a while)...
Torrent file created: ./torrent-data/BVN0_CLEAN_20241011.tar.gz.torrent

=== MAGNETIC LINK ===
magnet:?xt=urn:btih:...&dn=BVN0_CLEAN_20241011.tar.gz&tr=...
=====================

Magnetic link saved to: ./torrent-data/BVN0_CLEAN_20241011.tar.gz.magnet
Added torrent: BVN0_CLEAN_20241011.tar.gz
Info hash: ...
Seed-only mode: will not download, only seed existing file
Got torrent info: BVN0_CLEAN_20241011.tar.gz (45 GB)
Seeding... Press Ctrl+C to stop
Status: Peers=3 Upload=1.2 GB Download=0 B Ratio=inf
```

## Docker Deployment

### Build Docker Image

```bash
cd tools/torrent-deploy
docker build -f Dockerfile.torrent-server -t accumulate-torrent-server:latest ../..
```

### Run with Docker Compose

```bash
# Edit docker-compose.controlled.yml to set your volumes path
docker-compose -f docker-compose.controlled.yml up -d

# View logs and get magnetic link
docker logs accumulate-torrent-server

# Get magnetic link from file
cat torrent-data/BVN0_CLEAN_20241011.tar.gz.magnet
```

## Features

- ✅ **Controlled**: Only seeds specified files (no Web UI to add random torrents)
- ✅ **Standard Protocol**: Compatible with all BitTorrent clients
- ✅ **DHT Enabled**: Trackerless peer discovery
- ✅ **Auto-generation**: Creates .torrent and magnetic link automatically
- ✅ **Monitoring**: Displays peer count and upload stats every 30s
- ✅ **Seed-only Mode**: Won't download, only seeds existing files

## Network Ports

- **51413/tcp** - BitTorrent protocol
- **51413/udp** - BitTorrent DHT

Make sure these ports are open in your firewall:
```bash
sudo ufw allow 51413/tcp
sudo ufw allow 51413/udp
```

## Files Generated

| File | Description |
|------|-------------|
| `*.torrent` | Standard BitTorrent metadata file |
| `*.magnet` | Magnetic link (text file) |
| `torrent-data/` | Client data and piece cache |

## Next Steps

See [torrent_design.md](torrent_design.md) for the complete design and Phase 2 (follower deployment).

## Troubleshooting

### No peers connecting
- Check firewall allows ports 51413 tcp/udp
- Verify trackers are reachable
- DHT takes time to populate (5-10 minutes)

### High memory usage
- Normal for large files
- Torrent client caches pieces in memory
- Reduce if needed by stopping/starting

### Can't find file
- Check file path is correct
- File must exist before starting server
- Use absolute paths to avoid confusion
