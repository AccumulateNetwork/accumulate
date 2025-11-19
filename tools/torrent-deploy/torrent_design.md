# Accumulate Torrent-Based Deployment Design

## Overview

This system enables distributed deployment of Accumulate follower nodes using BitTorrent to distribute the complete `/volumes` directory structure from an existing running follower.

## Problem Statement

Deploying new Accumulate follower nodes requires:
1. Large data transfer (complete volumes structure with databases)
2. Bandwidth constraints from single source
3. Slow sync from genesis

**Solution**: Use BitTorrent to distribute the complete volumes archive, allowing multiple nodes to share bandwidth and seed the data collectively.

---

## Architecture

### Two-Phase Deployment

**Phase 1: Torrent Server Deployment**
- accman deploys a torrent server (seeding node)
- Server seeds the volumes .tar.gz file: `/media/paul/Expansion/accumulate-blockchain/bvn0-production/compressed/BVN0_CLEAN_20241011.tar.gz`
- Torrent server connects to other torrent servers via bootstrap peer discovery
- Creates and shares magnetic link for the volumes archive

**Phase 2: Follower Deployment** (future step)
- New follower nodes download volumes via torrent
- Extract complete `/volumes` structure
- Launch Accumulate follower with extracted volumes

---

## Phase 1: Torrent Server

### What Gets Torrented

**File**: `/media/paul/Expansion/accumulate-blockchain/bvn0-production/compressed/BVN0_CLEAN_20241011.tar.gz`

**Contents**: Complete `/volumes` directory structure from a running follower, including:
- `/volumes/bvn0/` - BVN node data
- `/volumes/dn/` - Directory Network node data
- All subdirectories and databases
- Everything needed for a follower to run

**NOT** snapshots - the actual complete volume directory tree.

### Torrent Server Components

1. **Docker Container**: Runs transmission or qbittorrent
2. **Bootstrap Peer Discovery**: Connects to other torrent servers
3. **Seeding**: Makes the volumes .tar.gz available for download
4. **Magnetic Link**: Generated and distributed for downloaders

### Bootstrap Peers

The torrent server discovers other torrent servers by:
1. Starting with bootstrap tracker URLs
2. DHT (Distributed Hash Table) for peer discovery
3. Connecting to peers seeding the same magnetic link
4. Building a mesh network of seeders

**Default Trackers**:
- `udp://tracker.opentrackr.org:1337/announce`
- `udp://tracker.openbittorrent.com:6969/announce`
- `udp://9.rarbg.com:2810/announce`

### accman Workflow

```bash
# 1. Deploy torrent server
accman deploy-torrent-server

# 2. Server automatically:
#    - Starts torrent daemon
#    - Creates .torrent file for volumes archive
#    - Generates magnetic link
#    - Begins seeding
#    - Connects to bootstrap trackers
#    - Discovers other seeders

# 3. Output: Magnetic link for distribution
magnet:?xt=urn:btih:HASH&dn=BVN0_CLEAN_20241011.tar.gz&tr=...
```

---

## Torrent Server Configuration

### Docker Compose Service

```yaml
services:
  torrent-server:
    image: linuxserver/transmission:latest
    volumes:
      - ./torrent-config:/config
      - /media/paul/Expansion/.../compressed:/downloads
    ports:
      - "9091:9091"      # Web UI
      - "51413:51413"    # BitTorrent port
    environment:
      - PEERPORT=51413
```

### Key Features

1. **Automatic Seeding**: Watches `/downloads` directory for .tar.gz
2. **Peer Discovery**: Uses DHT and trackers to find other nodes
3. **Web UI**: Manage torrents at http://localhost:9091
4. **Configurable**: Can manage what files to seed

---

## Magnetic Link Generation

### Process

1. **Create .torrent file** from volumes .tar.gz:
   ```bash
   transmission-create -o volumes.torrent -t TRACKER_URL BVN0_CLEAN_20241011.tar.gz
   ```

2. **Extract info hash** from .torrent file

3. **Generate magnetic link**:
   ```
   magnet:?xt=urn:btih:<INFO_HASH>&dn=<FILENAME>&tr=<TRACKER_URL>
   ```

4. **Distribute** magnetic link to downloaders

### Magnetic Link Components

- `xt=urn:btih:HASH` - BitTorrent info hash (identifies the file)
- `dn=FILENAME` - Display name
- `tr=TRACKER` - Tracker URLs for peer discovery

---

## Network Topology

```
┌─────────────────┐
│ Source Follower │
│   (Running)     │
└────────┬────────┘
         │
         │ tar -czf volumes.tar.gz /volumes
         │
         ▼
┌─────────────────────────────┐
│ Compressed Archive          │
│ BVN0_CLEAN_20241011.tar.gz │
└────────┬────────────────────┘
         │
         │ accman deploy-torrent-server
         │
         ▼
┌─────────────────────────────┐
│ Torrent Server 1 (Seeder)   │
│ Creates: magnet link         │
└────────┬────────────────────┘
         │
         │ Bootstrap trackers + DHT
         │
    ┌────┴────┬──────────┬──────────┐
    ▼         ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐
│Torrent │ │Torrent │ │Torrent │ │ ...    │
│Server 2│ │Server 3│ │Server 4│ │        │
└────────┘ └────────┘ └────────┘ └────────┘
 (Seeder)   (Seeder)   (Seeder)

    │          │          │          │
    │          │ Swarm of seeders    │
    └──────────┴──────────┴──────────┘
                    │
                    │ Download via magnet link
                    │
                    ▼
          ┌──────────────────┐
          │ New Follower Node│
          │  (Downloads via  │
          │   torrent)       │
          └──────────────────┘
```

---

## File Structure

```
tools/torrent-deploy/
├── torrent_design.md                 # This file
├── docker-compose.yml                # Torrent server deployment
├── create-torrent.sh                 # Generate .torrent + magnet link
├── scripts/
│   └── download-and-extract.sh       # Download via torrent (Phase 2)
├── Dockerfile.bootstrap              # Bootstrap container (Phase 2)
└── torrents/                         # Generated .torrent files
    ├── BVN0_CLEAN_20241011.tar.gz.torrent
    ├── BVN0_CLEAN_20241011.tar.gz.magnet
    └── BVN0_CLEAN_20241011.tar.gz.info
```

---

## Phase 1 Implementation Steps

### Step 1: Deploy Torrent Server

```bash
cd tools/torrent-deploy
docker-compose up -d torrent-server
```

- Starts transmission daemon
- Exposes Web UI on port 9091
- Ready to seed files

### Step 2: Create Torrent from Volumes Archive

```bash
./create-torrent.sh /media/paul/Expansion/accumulate-blockchain/bvn0-production/compressed/BVN0_CLEAN_20241011.tar.gz
```

**Outputs**:
- `.torrent` file
- Magnetic link
- SHA256 checksum
- Info file with instructions

### Step 3: Add to Torrent Server

**Option A**: Web UI
1. Open http://localhost:9091
2. Upload .torrent file
3. Verify seeding

**Option B**: Auto-add (watch directory)
```bash
cp torrents/BVN0_CLEAN_20241011.tar.gz.torrent ./torrent-config/watch/
```

**Option C**: Command line
```bash
transmission-remote localhost:9091 -a torrents/BVN0_CLEAN_20241011.tar.gz.torrent
```

### Step 4: Verify Seeding

```bash
# Check status via Web UI or CLI
transmission-remote localhost:9091 -l

# Should show:
# ID   Size   Ratio  Upload  Peers  ETA  Status  Name
# 1    45 GB  1.0    10 MB/s  3      -    Seeding BVN0_CLEAN_20241011.tar.gz
```

### Step 5: Distribute Magnetic Link

The magnetic link can now be shared with:
- Other torrent servers (to become seeders)
- New follower nodes (to download and deploy)

---

## Bootstrap Peer Discovery

### How Peers Find Each Other

1. **Tracker Announce**: Torrent client contacts tracker URLs
2. **Tracker Response**: Returns list of peers seeding the same file
3. **DHT**: Distributed peer discovery without central tracker
4. **PEX**: Peer exchange - peers share other peer addresses

### Multiple Torrent Servers

When you deploy multiple torrent servers:
1. Each runs `accman deploy-torrent-server`
2. Each adds the SAME magnetic link
3. Trackers connect them together
4. They form a swarm and share bandwidth
5. Download speeds increase with more seeders

---

## accman Integration

### Command: `deploy-torrent-server`

```bash
accman deploy-torrent-server [OPTIONS]

Options:
  --file PATH          Path to volumes .tar.gz (default: auto-detect)
  --port PORT          BitTorrent port (default: 51413)
  --tracker URL        Additional tracker URL
  --output-magnet FILE Save magnetic link to file
```

**Workflow**:
1. Start torrent server container
2. Scan for volumes .tar.gz files
3. Create .torrent file
4. Generate magnetic link
5. Add to seeding queue
6. Display magnetic link
7. Optionally save to file for distribution

### Example Output

```
=== Accumulate Torrent Server Deployment ===

✓ Torrent server started (port 51413)
✓ Found volumes archive: BVN0_CLEAN_20241011.tar.gz (45 GB)
✓ Created .torrent file
✓ Generated magnetic link
✓ Added to seeding queue

Magnetic Link:
magnet:?xt=urn:btih:a1b2c3d4e5f6...&dn=BVN0_CLEAN_20241011.tar.gz&tr=udp://tracker.opentrackr.org:1337/announce

Status:
- Seeding: YES
- Peers: 0 (discovering...)
- Upload: 0 MB/s

Web UI: http://localhost:9091

Share this magnetic link with other nodes to join the swarm.
```

---

## Configuration Management

### Default Volumes Archive

**Location**: `/media/paul/Expansion/accumulate-blockchain/bvn0-production/compressed/BVN0_CLEAN_20241011.tar.gz`

**Can be configured via**:
- Environment variable: `VOLUMES_ARCHIVE_PATH`
- Config file: `torrent-config.yml`
- Command line: `--file` flag

### Managing Multiple Archives

To seed multiple volumes archives (e.g., different dates):

```bash
# Scan directory for all .tar.gz files
accman deploy-torrent-server --scan-dir /media/paul/Expansion/.../compressed

# Or specify multiple files
accman deploy-torrent-server \
  --file BVN0_CLEAN_20241011.tar.gz \
  --file BVN0_CLEAN_20241101.tar.gz
```

---

## Phase 2: Follower Deployment (Future)

Once the torrent server is seeding, new followers can:

1. **Download via torrent**:
   ```bash
   accman deploy-follower --magnet "magnet:?xt=urn:btih:..."
   ```

2. **Extract volumes**:
   ```bash
   tar -xzf BVN0_CLEAN_20241011.tar.gz -C /var/lib/accumulate/
   ```

3. **Launch follower**:
   ```bash
   docker run -v /var/lib/accumulate/volumes:/volumes accumulate:latest
   ```

---

## Benefits

### Distributed Bandwidth
- Multiple seeders share upload bandwidth
- Faster downloads as more nodes join
- No single point of bottleneck

### Scalability
- Add more torrent servers → more bandwidth
- Self-organizing mesh network
- Automatic peer discovery

### Reliability
- If one seeder goes down, others continue
- DHT provides trackerless operation
- Checksum verification ensures data integrity

---

## Security Considerations

### Data Integrity
- SHA256 checksum of .tar.gz
- BitTorrent piece hashing
- Verify after download

### Network Security
- Torrent traffic is peer-to-peer
- Consider VPN for torrent servers if needed
- Firewall rules for BitTorrent port (51413)

### Access Control
- Web UI password protected
- Restrict Web UI to internal network
- Public BitTorrent port for peer connections

---

## Monitoring

### Check Torrent Server Status

```bash
# Via CLI
transmission-remote localhost:9091 -l

# Via Web UI
http://localhost:9091

# Check peer count
transmission-remote localhost:9091 -t 1 -i | grep "Peers:"
```

### Metrics to Monitor
- Number of peers
- Upload/download speed
- Ratio (uploaded/downloaded)
- Seeding status

---

## Troubleshooting

### No Peers Connected

**Cause**: Firewall blocking BitTorrent port

**Solution**:
```bash
sudo ufw allow 51413/tcp
sudo ufw allow 51413/udp
```

### Slow Seeding

**Cause**: Limited bandwidth or few peers

**Solution**:
- Deploy more torrent servers
- Check network bandwidth limits
- Ensure DHT is enabled

### Torrent Server Won't Start

**Cause**: Port conflict

**Solution**:
```bash
# Check port usage
sudo netstat -tulpn | grep 51413

# Change port in docker-compose.yml
```

---

## Future Enhancements

1. **Auto-update**: Automatically detect new volumes archives and create torrents
2. **Peer discovery**: Integrate with Accumulate P2P network for torrent peer discovery
3. **Metrics**: Prometheus metrics for torrent server
4. **Multi-network**: Support different networks (MainNet, TestNet) with separate torrents
5. **Incremental updates**: Delta torrents for volume updates instead of full archives

---

## Summary

**Phase 1** deploys a torrent server that:
1. Seeds the complete `/volumes` .tar.gz archive
2. Generates a magnetic link
3. Connects to other torrent servers via bootstrap trackers
4. Forms a distributed swarm for efficient data distribution

This creates the infrastructure for **Phase 2**: rapidly deploying new follower nodes by downloading the volumes archive via the torrent swarm.
