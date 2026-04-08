# Accumulate Deployment Guide

## Branching Strategy

This repository uses a two-branch deployment strategy aligned with network requirements:

### main (Production - CometBFT Consensus)
- **Scope**: Non-breaking critical bug fixes only
- **Audience**: Running production Accumulate network
- **Consensus**: CometBFT-based (current)
- **Network Impact**: No network reset required
- **Examples of valid commits**:
  - Security fixes (#3824, #3860, #3866, #3868)
  - Correctness fixes for data corruption
  - P2P connection handling improvements
  - Database concurrency fixes

### dagbft-integration (Future - DAG-BFT Consensus)
- **Scope**: All breaking changes including consensus layer replacement
- **Audience**: Network upgrade planning
- **Consensus**: DAG-BFT-based (new protocol)
- **Network Impact**: **Requires complete network reset**
- **Examples of valid work**:
  - Consensus protocol replacement (Tendermint → DAG-BFT)
  - BPT parallel sharding optimizations
  - New submission APIs with HTTP 429 backpressure
  - Per-peer vote rate limiting
  - Vote deduplication spam fixes (DAG-BFT specific)

## Building Versioned Images

### Local Build
```bash
# Build with current commit hash
./tmp/build-accumulated-image.sh

# Build with semantic version
./tmp/build-accumulated-image.sh "v1.0.0-critical-fixes"

# Build with registry prefix (for pushing)
./tmp/build-accumulated-image.sh "v1.0.0-critical-fixes" "docker.io/accumulatenetwork"
```

### Image Naming Convention
- **Main branch**: `accumulated:v{MAJOR}.{MINOR}.{PATCH}-critical-fixes`
- **Tags**: `accumulated:{commit-hash}`, `accumulated:latest`
- **Registry**: Configure in build script as needed

### Current Release Candidates
- **v1.0.0-critical-fixes**: Main branch with security and correctness fixes
  - Includes: #3824, #3860, #3866, #3868
  - Based on: CometBFT consensus
  - Status: Ready for production deployment

## Deployment Testing

### Local Validation
```bash
# Build image
docker build -f Dockerfile -t accumulated:v1.0.0-critical-fixes .

# Test image runs
docker run accumulated:v1.0.0-critical-fixes accumulated version

# Test basic functionality
docker run accumulated:v1.0.0-critical-fixes accumulated run devnet --bvns 3
```

### Network Testing via accman Followers
```bash
# Deploy to follower nodes managed by accman
accman follower update --image accumulated:v1.0.0-critical-fixes

# Monitor deployment
accman follower status

# Validate sync and performance
accman follower validate --check-tps --check-errors
```

### Full Network Testing
```bash
# Deploy to test network (issue-3892 infrastructure)
cd test/docker

# Build compatible test image
docker build -f ../../Dockerfile \
    -t accumulated:v1.0.0-critical-fixes .

# Update docker-compose
docker-compose up -d

# Run validation test
go run parallel-loadtest.go -duration 120s -start-tps 10000 -end-tps 10000

# Monitor metrics
curl http://localhost:8888/metrics | jq '.tps'
```

## Critical Files to Protect

### Do Not Modify
These files define the network topology and bootstrap configuration:
- `test/docker/docker-compose.yml` - Bootstrap key, port bindings
- `test/docker/docker-network.yml` - Genesis topology, BVN/validator count
- `test/docker/parallel-loadtest.go` - Load generator configuration (workersPerNode = 4)

### Safe to Modify
- `test/docker/dashboard.html` - Monitoring display
- `test/docker/metrics-server.py` - Metrics collection
- `test/docker/monitoring.py` - Per-node resource monitoring

## Integration Checklist

After deploying a new version:

- [ ] Build succeeds: `go build ./cmd/accumulated`
- [ ] Image builds: `docker build -f Dockerfile .`
- [ ] Image runs: `docker run accumulated version`
- [ ] All 12 validators healthy: `docker compose ps`
- [ ] TPS ≥ 8000: `go run parallel-loadtest.go`
- [ ] Failure rate < 0.1%
- [ ] No critical errors in logs: `docker compose logs | grep ERROR`
- [ ] State hashes match across validators
- [ ] Snapshot loading works (if testing state sync)

## Rollback Procedure

If deployment fails:
```bash
# Revert to previous version
docker-compose down -v
git checkout HEAD~1
docker build -f Dockerfile -t accumulated:latest .
docker-compose up -d
docker-compose logs # Monitor recovery
```

## Future: DAG-BFT Network Upgrade

When `dagbft-integration` is ready for deployment:

1. Tag release: `accumulated:v2.0.0-dagbft`
2. Deploy to test network first
3. Run 24-hour stability test
4. Communicate network upgrade to all followers
5. Perform network-wide reset
6. Deploy dagbft-integration version to all nodes
7. Monitor consensus formation
8. Validate all blocks and state hashes

