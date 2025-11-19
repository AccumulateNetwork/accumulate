# Installing Accumulate Torrent Server

## Quick Install

```bash
cd tools/torrent-deploy
sudo ./install-torrent-server.sh
```

This will:
1. Build the torrent server
2. Install to `/opt/accumulate-torrent`
3. Create systemd service
4. Configure firewall
5. Create management scripts

## Custom Installation

### Set Installation Directory
```bash
sudo INSTALL_DIR=/usr/local/accumulate-torrent ./install-torrent-server.sh
```

### Set Volumes File Path
```bash
sudo VOLUMES_FILE=/path/to/your/volumes.tar.gz ./install-torrent-server.sh
```

### User Installation (No Systemd)
```bash
SYSTEMD=false INSTALL_DIR=$HOME/accumulate-torrent ./install-torrent-server.sh
```

### Set Custom Port
```bash
sudo TORRENT_PORT=6881 ./install-torrent-server.sh
```

## Usage

### Start Server
```bash
# With systemd
sudo systemctl start accumulate-torrent

# Or use script
/opt/accumulate-torrent/start.sh
```

### Check Status
```bash
# With systemd
sudo systemctl status accumulate-torrent
sudo journalctl -u accumulate-torrent -f

# Or use script
/opt/accumulate-torrent/status.sh
```

### Get Magnetic Link
```bash
/opt/accumulate-torrent/magnet.sh
```

### Stop Server
```bash
# With systemd
sudo systemctl stop accumulate-torrent

# Or use script
/opt/accumulate-torrent/stop.sh
```

## accman Integration

accman can call this installer with custom parameters:

```bash
# In accman deployment script
VOLUMES_FILE="/path/from/accman/volumes.tar.gz" \
INSTALL_DIR="/opt/accumulate-torrent" \
/path/to/install-torrent-server.sh
```

Or accman can use the compiled binary directly:

```bash
# Copy binary
cp tools/torrent-deploy/cmd/torrent-server/torrent-server /opt/accman/bin/

# Run directly
/opt/accman/bin/torrent-server \
  -file /volumes/BVN0.tar.gz \
  -data /var/lib/accman/torrent \
  -port 51413
```

## Files

After installation:

```
/opt/accumulate-torrent/
├── bin/
│   └── torrent-server          # Binary
├── data/
│   ├── *.torrent               # Generated torrent files
│   ├── *.magnet                # Magnetic links
│   └── ...                     # Torrent client data
├── torrent-server.conf         # Configuration
├── start.sh                    # Start server
├── stop.sh                     # Stop server
├── status.sh                   # Check status
└── magnet.sh                   # Get magnetic link
```

## Systemd Service

Service file: `/etc/systemd/system/accumulate-torrent.service`

Commands:
```bash
# Enable auto-start on boot
sudo systemctl enable accumulate-torrent

# Start now
sudo systemctl start accumulate-torrent

# Stop
sudo systemctl stop accumulate-torrent

# Restart
sudo systemctl restart accumulate-torrent

# View logs
sudo journalctl -u accumulate-torrent -f

# Disable auto-start
sudo systemctl disable accumulate-torrent
```

## Uninstall

```bash
# Stop service
sudo systemctl stop accumulate-torrent
sudo systemctl disable accumulate-torrent

# Remove service
sudo rm /etc/systemd/system/accumulate-torrent.service
sudo systemctl daemon-reload

# Remove installation
sudo rm -rf /opt/accumulate-torrent

# Remove firewall rules (optional)
sudo ufw delete allow 51413/tcp
sudo ufw delete allow 51413/udp
```

## Troubleshooting

### Service won't start
```bash
# Check configuration
cat /opt/accumulate-torrent/torrent-server.conf

# Check if volumes file exists
ls -lh $(grep VOLUMES_FILE /opt/accumulate-torrent/torrent-server.conf | cut -d= -f2)

# Check logs
sudo journalctl -u accumulate-torrent -n 50
```

### Port already in use
```bash
# Check what's using the port
sudo netstat -tulpn | grep 51413

# Change port in config
sudo nano /opt/accumulate-torrent/torrent-server.conf
sudo systemctl restart accumulate-torrent
```

### No peers connecting
```bash
# Check firewall
sudo ufw status | grep 51413

# Open ports if needed
sudo ufw allow 51413/tcp
sudo ufw allow 51413/udp

# Check if server is listening
sudo netstat -tulpn | grep 51413
```

### Get more verbose output
```bash
# Stop service
sudo systemctl stop accumulate-torrent

# Run manually to see output
cd /opt/accumulate-torrent
source torrent-server.conf
./bin/torrent-server -file "$VOLUMES_FILE" -data "$DATA_DIR" -port "$TORRENT_PORT"
```
