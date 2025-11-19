# accman Integration Guide

## Overview

The Accumulate torrent server can be integrated into accman for automated deployment of torrent seeders alongside validator/follower nodes.

## Integration Options

### Option 1: Shell Script Integration

accman can call the provided scripts:

```bash
# In accman deployment
/path/to/accumulate/tools/torrent-deploy/accman-integration.sh deploy
```

This script:
1. Finds the volumes .tar.gz file
2. Installs the torrent server
3. Starts seeding
4. Returns the magnetic link

### Option 2: Direct Binary Integration

accman can use the torrent server binary directly:

```bash
# Build once
cd /path/to/accumulate/tools/torrent-deploy/cmd/torrent-server
go build -o /usr/local/bin/accumulate-torrent-server .

# In accman: Start torrent server
/usr/local/bin/accumulate-torrent-server \
  -file /var/lib/accumulate/volumes/BVN0_CLEAN.tar.gz \
  -data /var/lib/accumulate/torrent \
  -port 51413 &
```

### Option 3: Go Library Integration

If accman is written in Go, import the torrent library directly:

```go
import "github.com/anacrolix/torrent"

// In accman code
func startTorrentServer(volumesFile string) (magnetLink string, err error) {
    cfg := torrent.NewDefaultClientConfig()
    cfg.DataDir = "/var/lib/accumulate/torrent"
    cfg.Seed = true
    cfg.ListenPort = 51413

    client, err := torrent.NewClient(cfg)
    if err != nil {
        return "", err
    }

    // Create torrent and get magnet link
    // ... (see cmd/torrent-server/main.go for full implementation)

    return magnetLink, nil
}
```

## accman Command Examples

### Deploy Torrent Server

```bash
# accman would add a command like:
accman deploy-torrent-server --node validator1 --volumes /path/to/volumes.tar.gz

# Or as part of node deployment:
accman deploy-node --node validator1 --with-torrent
```

### Get Magnetic Link

```bash
# accman would query the installed server:
accman get-magnet-link --node validator1
```

### Multi-Node Deployment

```bash
# Deploy torrent servers on multiple nodes
accman deploy-torrent-server --nodes validator1,validator2,validator3

# All nodes will seed the same file, forming a swarm
```

## Configuration in accman

accman could have configuration like:

```yaml
# accman.yaml
torrent:
  enabled: true
  port: 51413
  volumes_path: /var/lib/accumulate/volumes
  install_dir: /opt/accumulate-torrent
  auto_seed: true  # Automatically seed volumes archives

nodes:
  - name: validator1
    role: validator
    torrent:
      enabled: true
      volumes_file: /volumes/BVN0_20241011.tar.gz

  - name: validator2
    role: validator
    torrent:
      enabled: true
      volumes_file: /volumes/BVN0_20241011.tar.gz  # Same file = same swarm

  - name: follower1
    role: follower
    torrent:
      enabled: false  # Followers download, don't seed (unless configured)
```

## Workflow

### Phase 1: Deploy Seeders (Current)

```
1. accman identifies validator nodes
2. accman checks for volumes .tar.gz files
3. accman deploys torrent server on each validator
4. Each torrent server:
   - Creates .torrent file (first one wins, others use same)
   - Generates magnetic link
   - Starts seeding
   - Discovers peers via DHT + trackers
5. accman collects magnetic link
6. accman stores magnetic link for Phase 2
```

### Phase 2: Deploy Followers (Future)

```
1. accman deploys new follower
2. accman provides magnetic link from Phase 1
3. Follower downloads volumes via torrent from seeders
4. Follower extracts volumes
5. Follower starts Accumulate node
```

## Example accman Integration Code

```bash
#!/bin/bash
# In accman's deployment script

deploy_node_with_torrent() {
    local node_name="$1"
    local volumes_file="$2"

    echo "Deploying torrent server on $node_name..."

    # SSH to node and install
    ssh "$node_name" bash -s <<EOF
        # Download installer
        curl -O https://gitlab.com/.../install-torrent-server.sh

        # Install
        sudo VOLUMES_FILE="$volumes_file" ./install-torrent-server.sh

        # Start
        sudo systemctl start accumulate-torrent

        # Wait and get magnet link
        sleep 5
        /opt/accumulate-torrent/magnet.sh
EOF

    # Store magnet link for later use
    magnet_link=$(ssh "$node_name" /opt/accumulate-torrent/magnet.sh)
    echo "$magnet_link" > "$ACCMAN_CONFIG_DIR/magnet-link.txt"

    echo "Torrent server deployed on $node_name"
    echo "Magnetic link: $magnet_link"
}

# Deploy on multiple nodes
for node in validator1 validator2 validator3; do
    deploy_node_with_torrent "$node" "/volumes/BVN0.tar.gz" &
done
wait

echo "All torrent servers deployed and forming swarm!"
```

## Environment Variables

accman can set these when calling the installer:

| Variable | Description | Default |
|----------|-------------|---------|
| `VOLUMES_FILE` | Path to volumes .tar.gz | Required |
| `INSTALL_DIR` | Installation directory | `/opt/accumulate-torrent` |
| `TORRENT_PORT` | BitTorrent port | `51413` |
| `SYSTEMD` | Use systemd service | `true` |

Example:
```bash
VOLUMES_FILE="/data/BVN0.tar.gz" \
INSTALL_DIR="/opt/accman/torrent" \
TORRENT_PORT=6881 \
./install-torrent-server.sh
```

## Monitoring

accman can monitor torrent servers:

```bash
# Check if running
ssh node1 systemctl is-active accumulate-torrent

# Get peer count
ssh node1 journalctl -u accumulate-torrent -n 1 | grep "Peers="

# Get upload stats
ssh node1 journalctl -u accumulate-torrent | grep "Upload="
```

## Network Requirements

accman needs to ensure:
- Port 51413 (tcp/udp) open on all seeder nodes
- Nodes can reach public trackers
- Nodes can communicate with each other (P2P)

## Security Considerations

- Torrent servers only seed specified files (controlled binary)
- No Web UI exposed (unlike Transmission/qBittorrent)
- Runs as systemd service with security restrictions
- Files are read-only in torrent client

## Future Enhancements

1. **Magnetic Link Registry**: accman maintains a registry of magnetic links for different volumes versions
2. **Auto-Discovery**: New followers automatically discover seeders via accman
3. **Bandwidth Limits**: accman can configure upload/download limits per node
4. **Health Checks**: accman monitors torrent server health and restarts if needed
5. **Multi-Network**: Support different networks (MainNet, TestNet) with separate torrents

## Testing

Test accman integration:

```bash
# 1. Deploy torrent server on test node
cd tools/torrent-deploy
./accman-integration.sh deploy

# 2. Verify service is running
systemctl status accumulate-torrent

# 3. Get magnetic link
./accman-integration.sh magnet

# 4. Test with another node downloading via that magnet
```

## Support

For accman integration questions:
- Review `torrent_design.md` for architecture details
- See `INSTALL.md` for installation options
- Check `README.md` for usage examples
