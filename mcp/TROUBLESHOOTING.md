# Accumulate MCP - Troubleshooting Guide

**Last Updated:** 2025-11-16

This guide covers common errors, their causes, and solutions when using Accumulate MCP tools and the `accumulated` binary.

---

## MCP Tool Errors

### Error: "missing required parameter: dn_database"

**Symptom:**
```json
{
  "error": {
    "code": -32602,
    "message": "missing required parameter: dn_database"
  }
}
```

**Cause:** Tool call missing required parameter

**Solution:**
Check tool definition for required parameters:
```bash
# For accumulate_init_follower, these are required:
{
  "dn_database": "/path/to/dn",
  "bvn_database": "/path/to/bvn",
  "work_dir": "/path/to/work"
}
```

---

### Error: "source database not found"

**Symptom:**
```
Error: source database not found: /media/paul/Expansion/databases/2025-10-13-dn
```

**Cause:** Database path doesn't exist or is inaccessible

**Solution:**
```bash
# Verify path exists
ls -la /media/paul/Expansion/databases/2025-10-13-dn

# Check permissions
ls -ld /media/paul/Expansion/databases/2025-10-13-dn

# Verify it's a directory
test -d /media/paul/Expansion/databases/2025-10-13-dn && echo "OK" || echo "NOT A DIRECTORY"
```

---

### Error: "accumulate.db directory is empty (corrupted snapshot)"

**Symptom:**
```
Error: accumulate.db directory is empty (corrupted snapshot)
```

**Cause:** Database snapshot is incomplete or corrupted

**Solution:**
```bash
# Check if database has files
ls -la /path/to/database/data/accumulate.db/

# Should show multiple files:
# MANIFEST, *.vlog, *.sst files

# If empty, snapshot is corrupted - use a different snapshot
```

---

### Error: "source missing data/accumulate.db/"

**Symptom:**
```
Error: source missing data/accumulate.db/: /path (may be incomplete snapshot)
```

**Cause:** Database directory doesn't have required structure

**Solution:**
Ensure database has complete structure:
```
database/
├── config/
│   ├── addrbook.json
│   └── tendermint.toml
└── data/
    ├── accumulate.db/    ← MUST EXIST
    │   ├── MANIFEST
    │   └── *.vlog files
    └── blockstore.db/
```

If missing, the snapshot is incomplete. Use a complete node backup.

---

### Error: "container started but is not running"

**Symptom:**
```json
{
  "error": "container started but is not running. Logs:\n..."
}
```

**Cause:** Docker container crashed immediately after start

**Solution:**
```bash
# Check full logs
docker logs accumulate-follower

# Common causes:
# 1. Port already in use
sudo netstat -tulpn | grep -E '16591|16691'

# 2. Invalid configuration
cat /work/dir/accumulate.toml

# 3. Bad database files
ls -la /work/dir/dnn/data/accumulate.db/

# 4. Permission issues
ls -ld /work/dir/dnn /work/dir/bvnn
```

---

## `accumulated` Binary Errors

### Error: "Unsupported network type PartitionType:0"

**Symptom:**
```
Error: Unsupported network type PartitionType:0
```

**Command:**
```bash
accumulated init dual --follow tcp://23.22.212.106:16691
```

**Cause:** Protocol incompatibility between binary version and network peer

**Known Issue:** Binary reports "version unknown" but peer is v1.4.1

**Workaround:**
Use MCP tools or manual database setup instead of `accumulated init dual --follow`:

```bash
# Option 1: Use MCP tools
./mcp-server <<EOF
{
  "method": "accumulate_init_follower",
  "params": {
    "dn_database": "/path/to/dn",
    "bvn_database": "/path/to/bvn",
    "work_dir": "/tmp/follower"
  }
}
EOF

# Option 2: Manual setup
cp -r /backup/dnn /work/dir/
cp -r /backup/bvnn /work/dir/
# Create accumulate.toml manually
```

**Status:** Requires fix in `accumulated` binary (cmd/accumulated/cmd_init.go)

---

### Error: "read dnn: is a directory"

**Symptom:**
```
read dnn: is a directory
```

**Command:**
```bash
accumulated run-dual dnn bvnn
```

**Cause:** Binary expects a file but found a directory (unclear what file)

**Troubleshooting:**
```bash
# Verify structure
ls -la dnn/ bvnn/
ls -la accumulate.toml

# Check accumulate.toml is NOT inside dnn/
test -f accumulate.toml && echo "OK" || echo "MISSING"

# Verify parent directory structure
cd /work/dir
ls -la
# Should show:
#   accumulate.toml (file)
#   dnn/ (directory)
#   bvnn/ (directory)
```

**Workaround:**
Use Docker deployment via MCP instead:
```bash
# MCP handles directory structure correctly
./examples/deploy-follower-complete.sh
```

**Status:** Requires better error message in `accumulated` binary (cmd/accumulated/cmd_run_dual.go)

---

### Error: "invalid character '\x00' looking for beginning of value"

**Symptom:**
```
Error: invalid character '\x00' looking for beginning of value
```

**Command:**
```bash
accumulated init dual --dn-genesis-doc dn-genesis.snap
```

**Cause:** Passing binary `.snap` file to flag expecting JSON format

**Solution:**
**Don't use `--dn-genesis-doc` flag with `.snap` files!**

`.snap` files are binary snapshots, not JSON documents.

**Options:**
```bash
# Option 1: Let accumulated download genesis
accumulated init dual --follow tcp://peer:port

# Option 2: Use MCP tools (handle .snap files correctly)
# MCP copies .snap files to work directory without parsing

# Option 3: Don't use genesis doc flags
accumulated init dual --skip-version-check
# Then place .snap files in parent directory manually
```

---

## Docker Issues

### Error: Port already in use

**Symptom:**
```
Error: failed to start Docker container
...bind: address already in use
```

**Cause:** Ports 16591-16593 or 16691-16693 already in use

**Solution:**
```bash
# Find what's using the ports
sudo netstat -tulpn | grep -E '16591|16691'

# Stop conflicting service or use different ports
# Or stop old follower:
docker stop accumulate-follower
docker rm accumulate-follower
```

---

### Error: Docker daemon not running

**Symptom:**
```
Error: Cannot connect to the Docker daemon
```

**Solution:**
```bash
# Start Docker
sudo systemctl start docker

# Or on macOS
open -a Docker

# Verify
docker ps
```

---

### Error: Permission denied

**Symptom:**
```
Error: permission denied while trying to connect to Docker daemon
```

**Solution:**
```bash
# Add user to docker group
sudo usermod -aG docker $USER

# Log out and back in, then verify
docker ps

# Or use sudo (not recommended)
sudo docker ps
```

---

## Genesis File Issues

### Error: Genesis files not found

**Symptom:**
```json
{
  "warning": "Genesis files not found: [dn-genesis.snap bvn1-genesis.snap]"
}
```

**Cause:** Genesis files not in standard location (~/.accumulate/)

**Solution:**
```bash
# Check if files exist
ls -la ~/.accumulate/*genesis.snap

# If not found, obtain from:
# 1. Node backup
cp /backup/node/dn-genesis.snap ~/.accumulate/
cp /backup/node/bvn1-genesis.snap ~/.accumulate/

# 2. Or continue without them (may work depending on database state)
# MCP tools make genesis files optional
```

---

### Issue: Wrong BVN genesis file

**Symptom:**
Follower starts but won't sync correctly

**Cause:** Using wrong BVN partition genesis file (e.g., bvn1 instead of bvn2)

**Solution:**
```bash
# Verify which BVN you're deploying
# Cyclops = bvn1
# Apollo = bvn2
# etc.

# Use correct genesis file
{
  "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap"
}

# Or check which BVN partitions are available
ls ~/.accumulate/bvn*-genesis.snap
```

---

## Database Issues

### Error: Database corruption detected

**Symptom:**
```
Badger CORRUPTION: ...
```

**Cause:** Database files corrupted

**Solution:**
```bash
# 1. Use a different database snapshot
# 2. Or try Badger recovery (advanced)
# 3. Re-download from network

# Verify database integrity before deployment
ls -lh /path/to/database/data/accumulate.db/MANIFEST
# Should be non-zero size
```

---

### Error: Disk space issues

**Symptom:**
```
Error: no space left on device
```

**Solution:**
```bash
# Check available space
df -h /var/lib/accumulate-follower

# Database snapshots can be large (50+ GB)
# Ensure sufficient space before deployment

# Clean up old deployments
docker system prune -a
```

---

## Network Issues

### Error: Cannot connect to bootstrap peers

**Symptom:**
Follower logs show:
```
Failed to dial peer
Connection refused
```

**Solution:**
```bash
# Verify peer is accessible
curl http://23.22.212.106:16592/status

# Check firewall
sudo iptables -L | grep 16591

# Verify bootstrap peers in config
cat /work/dir/accumulate.toml | grep bootstrap

# Try different bootstrap peers
# Get current peers:
curl https://mainnet.accumulatenetwork.io/v3 | jq
```

---

## Configuration Issues

### Error: Invalid accumulate.toml

**Symptom:**
```
Error parsing config
```

**Solution:**
```bash
# Validate TOML syntax
cat accumulate.toml

# Required fields:
network = "MainNet"
[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"
  listen = "/ip4/0.0.0.0/tcp/16591"
  dn-bootstrap-peers = [...]
  bvn-bootstrap-peers = [...]
```

---

## Common Workflow Issues

### Issue: Which tool should I use?

**Question:** Should I use `accumulated` binary or MCP tools?

**Answer:**

**Use MCP Tools when:**
- You have database snapshots/backups
- You want Docker deployment
- You want automated, scripted deployment
- You want better error messages

**Use `accumulated` Binary when:**
- You want native (non-Docker) deployment
- You need specific `accumulated` features
- You're initializing from network peer

**Recommended:** Start with MCP tools, they're more robust.

---

### Issue: Follower won't sync

**Symptom:**
Follower running but sync_height not increasing

**Troubleshooting:**
```bash
# Check sync status
curl http://localhost:16592/status | jq '.result.sync_info'

# Check peer count
curl http://localhost:16592/net_info | jq '.result.n_peers'

# If peers = 0, bootstrap peer issue
# Check logs:
docker logs -f accumulate-follower | grep -i peer

# Verify bootstrap peers are correct
cat /work/dir/accumulate.toml
```

---

## Error Messages Quick Reference

| Error | Cause | Fix |
|-------|-------|-----|
| missing required parameter | Missing tool parameter | Add required parameter |
| source database not found | Wrong path | Verify path exists |
| accumulate.db directory is empty | Corrupted snapshot | Use different snapshot |
| container started but is not running | Container crashed | Check docker logs |
| Unsupported network type PartitionType:0 | accumulated binary bug | Use MCP tools instead |
| read dnn: is a directory | accumulated binary bug | Use MCP tools instead |
| invalid character '\x00' | Wrong file format | Don't pass .snap to --genesis-doc |
| port already in use | Port conflict | Stop conflicting service |
| Genesis files not found | Missing files | Obtain from backup or network |

---

## Getting Help

### Check Logs

**MCP Tools:**
```bash
# MCP server logs (if running in background)
tail -f mcp-server.log

# Docker container logs
docker logs -f accumulate-follower
```

**`accumulated` Binary:**
```bash
# Run with verbose logging
accumulated --debug run-dual dnn bvnn

# Check systemd logs (if running as service)
journalctl -u accumulated -f
```

### Verify Prerequisites

```bash
# Database snapshots exist
test -d /path/to/dn && test -d /path/to/bvn && echo "OK"

# Have complete structure
ls /path/to/dn/data/accumulate.db/MANIFEST

# Docker is running
docker ps

# Ports are available
sudo netstat -tulpn | grep -E '16591|16691'

# Disk space available
df -h /var/lib/accumulate-follower
```

### Debug Checklist

- [ ] Database snapshots have complete structure
- [ ] Databases are not corrupted (MANIFEST exists, files present)
- [ ] Docker is running and accessible
- [ ] Ports 16591-16593 and 16691-16693 are available
- [ ] Sufficient disk space (50+ GB recommended)
- [ ] Genesis files present (if required)
- [ ] Bootstrap peers are accessible
- [ ] Configuration file (accumulate.toml) is valid

---

## Reporting Bugs

**For `accumulated` binary bugs:**
- Report to Accumulate repository
- Include: binary version, command used, full error output
- Known issues: PartitionType:0, "is a directory" error

**For MCP tool bugs:**
- Report to Accumulate MCP
- Include: tool name, parameters used, error response
- Include: Docker version, OS version if relevant

---

## See Also

- `FOLLOWER_DOCKER_GUIDE.md` - Deployment guide
- `GENESIS_FILES_GUIDE.md` - Genesis file details
- `examples/README.md` - Integration examples
- `MCP_DEPLOYMENT_ISSUES.md` - Known issues list
