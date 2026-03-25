# Byzantine Test Infrastructure - Technical Details

## Overview

This document provides technical details about the Byzantine fault tolerance test infrastructure for developers and operators.

## Network Topology

### Physical Layout

```
172.30.0.0/16 Network (Docker Bridge)
│
├── bootstrap (172.30.0.2)
│   └── Coordinates initial network setup
│
├── BVN0 (8 validators)
│   ├── bvn0-0 (172.30.0.10) [Leader] - Port 8080, 27000, 27100
│   ├── bvn0-1 (172.30.0.11) - Port 27001, 27101
│   ├── bvn0-2 (172.30.0.12) - Port 27002, 27102
│   ├── bvn0-3 (172.30.0.13) - Port 27003, 27103
│   ├── bvn0-4 (172.30.0.14) - Port 27004, 27104
│   ├── bvn0-5 (172.30.0.15) - Port 27005, 27105
│   ├── bvn0-6 (172.30.0.16) - Port 27006, 27106
│   └── bvn0-7 (172.30.0.17) - Port 27007, 27107
│
├── BVN1 (8 validators)
│   ├── bvn1-0 (172.30.0.20) [Leader] - Port 8081, 27008, 27108
│   ├── bvn1-1 (172.30.0.21) - Port 27009, 27109
│   ├── bvn1-2 (172.30.0.22) - Port 27010, 27110
│   ├── bvn1-3 (172.30.0.23) - Port 27011, 27111
│   ├── bvn1-4 (172.30.0.24) - Port 27012, 27112
│   ├── bvn1-5 (172.30.0.25) - Port 27013, 27113
│   ├── bvn1-6 (172.30.0.26) - Port 27014, 27114
│   └── bvn1-7 (172.30.0.27) - Port 27015, 27115
│
├── BVN2 (8 validators)
│   ├── bvn2-0 (172.30.0.30) [Leader] - Port 8082, 27016, 27116
│   ├── bvn2-1 (172.30.0.31) - Port 27017, 27117
│   ├── bvn2-2 (172.30.0.32) - Port 27018, 27118
│   ├── bvn2-3 (172.30.0.33) - Port 27019, 27119
│   ├── bvn2-4 (172.30.0.34) - Port 27020, 27120
│   ├── bvn2-5 (172.30.0.35) - Port 27021, 27121
│   ├── bvn2-6 (172.30.0.36) - Port 27022, 27122
│   └── bvn2-7 (172.30.0.37) - Port 27023, 27123
│
└── DN (1 validator)
    └── dn-0 (172.30.0.40) - Port 8083, 27024, 27124
```

### Port Mapping

| Service | Internal Port | External Port Range | Purpose |
|---------|---------------|---------------------|---------|
| P2P | 26656 | 27000-27024 | Node communication |
| Metrics | 26660 | 27100-27124 | Prometheus metrics |
| API | 8080 | 8080-8083 | HTTP API (leaders only) |

### Consensus Parameters

```
Total Validators: 25
Byzantine Fault Tolerance: f < 25/3 ≈ 8.33
Maximum Byzantine Nodes: 8 (32%)

Test Configuration: 9 malicious (36%)
This EXCEEDS the BFT threshold intentionally to validate:
  - Honest validators maintain consensus
  - Security fixes prevent attacks from succeeding
```

## Container Configuration

### Resource Limits

Each container is configured with:

```yaml
mem_limit: 2g
cpu_shares: 1024  # Equal priority
```

**Total Resources:**
- Memory: 26 containers × 2GB = 52GB
- CPU: Shared across all containers
- Disk: ~1GB per container for data/logs

### Health Checks

```bash
# Defined in Dockerfile
HEALTHCHECK CMD curl --fail --silent http://localhost:26660/status || exit 1

# Check interval: 10s
# Timeout: 5s
# Retries: 5
# Start period: 30s
```

### Volume Mounts

Each validator has a persistent volume:

```yaml
volumes:
  bvn0-0-data:
    driver: local
```

Data persists across restarts (unless using `down -v`).

## Attack Simulation Architecture

### Byzantine Attack (Issue #3869)

**Attack Vector:** Duplicate votes from malicious validators

```
Malicious Validator (1 of 9)
    │
    ├─> Round 0: Create header H0
    │   ├─> Vote 1 for H0 (valid)
    │   ├─> Vote 2 for H0 (duplicate)
    │   ├─> Vote 3 for H0 (duplicate)
    │   └─> ... 10 votes total
    │
    ├─> Round 1: Create header H1
    │   └─> 10 votes for H1
    │
    └─> Continues for attack duration
```

**Detection Mechanism:**

```go
// vote_handler.go line 87-91
for _, v := range votes {
    if bytes.Equal(v.Author, vote.Author) {
        return // Duplicate detected - reject
    }
}
```

**Expected Outcome:**
- First vote from each validator accepted
- Subsequent duplicates rejected
- CPU impact: O(n) where n = votes received (not created)
- Memory impact: 1 vote per validator per header

### Vote Spam Attack (Issue #3870)

**Attack Vector:** Votes from non-validator nodes

```
Non-Validator Spammer (1 of 100)
    │
    ├─> 1000 votes/sec
    │   ├─> Random header digests
    │   ├─> Valid signatures (but not in committee)
    │   └─> Sent to all validators
    │
    └─> Total network load: 100,000 votes/sec
```

**Detection Mechanism:**

```go
// vote_handler.go line 34-42
p.committeeMu.RLock()
inCommittee := p.committee.ContainsValidator(vote.Author)
p.committeeMu.RUnlock()

if !inCommittee {
    // Reject early - before signature verification
    return
}
```

**Expected Outcome:**
- Non-validator votes rejected before signature check
- CPU impact: O(log n) for committee lookup
- No expensive crypto operations on spam
- Target: < 10% CPU for spam processing

## Security Properties Validated

### 1. Byzantine Fault Tolerance (BFT)

**Property:** Consensus continues with f < n/3 Byzantine validators

**Test:**
- 9 Byzantine out of 25 total (36% > 33%)
- Honest validators: 16 (64%)
- Quorum threshold: 17 (2f+1)

**Validation:**
- Monitor round progression
- Check certificate creation
- Verify honest nodes maintain agreement

### 2. Duplicate Vote Prevention

**Property:** Each validator contributes at most 1 vote per header

**Test:**
- Malicious validators send 10 votes each
- Monitor vote acceptance rate
- Check vote storage doesn't grow

**Validation:**
- Confirm only 1 vote stored per validator
- Verify memory usage stays constant
- Check no duplicate vote signatures in certificates

### 3. Spam Resistance

**Property:** Non-validator votes rejected with minimal CPU cost

**Test:**
- 100 non-validators @ 1000 votes/sec
- Total: 100,000 spam votes/sec
- Monitor validator CPU usage

**Validation:**
- CPU < 10% for spam processing
- Committee check happens before signature verification
- Spam doesn't affect consensus progress

## Performance Metrics

### Expected Performance

| Metric | Target | Critical Threshold |
|--------|--------|-------------------|
| Validator CPU (normal) | < 5% | 20% |
| Validator CPU (under spam) | < 10% | 25% |
| Memory per validator | < 1GB | 2GB |
| Vote processing latency | < 1ms | 10ms |
| Round time | 1-2 seconds | 10 seconds |
| Certificate creation | Within 1 round | Within 3 rounds |

### Monitoring Commands

```bash
# CPU and Memory
docker stats --no-stream

# Network I/O
docker stats --format "table {{.Container}}\t{{.NetIO}}"

# Consensus progress
curl http://localhost:8080/v3/describe | jq '.round, .epoch'

# Vote metrics (if exposed)
curl http://localhost:27100/metrics | grep vote
```

## Failure Modes and Recovery

### Network Partition

**Symptom:** Some validators can't reach others

**Detection:**
```bash
docker compose logs | grep "peer.*disconnect"
```

**Recovery:**
```bash
docker compose restart <container>
```

### Memory Exhaustion

**Symptom:** Container OOM, health check fails

**Detection:**
```bash
docker stats | grep "MEM USAGE.*2.*GiB"
```

**Recovery:**
```bash
# Increase memory limit in docker-compose-25node.yml
mem_limit: 4g
```

### Consensus Stall

**Symptom:** Rounds not advancing

**Detection:**
```bash
# Check round number over time
watch -n 5 'curl -s http://localhost:8080/v3/describe | jq .round'
```

**Recovery:**
```bash
# Check validator logs
docker compose logs bvn0-0 | grep -i "error\|stall\|timeout"

# Restart stalled node
docker compose restart bvn0-0
```

### Byzantine Attack Success

**Symptom:** Attack report shows "FAILURE"

**Investigation:**
1. Check how many validators are healthy
2. Verify vote deduplication logic
3. Review vote handler logs
4. Check if honest validators have quorum

## Test Data Collection

### Logs

```bash
# Location
test/docker/byzantine-test/logs/

# Contents
├── build_YYYYMMDD_HHMMSS.log           # Docker build output
├── startup_YYYYMMDD_HHMMSS.log         # Network startup
├── byzantine-attack-YYYYMMDD_HHMMSS.log # Attack execution
├── vote-spam-attack-YYYYMMDD_HHMMSS.log # Spam execution
├── all-containers-YYYYMMDD_HHMMSS.log   # All container logs
└── cleanup_YYYYMMDD_HHMMSS.log         # Cleanup output
```

### Reports

```bash
# Location
test/docker/byzantine-test/results/

# Contents
├── byzantine-attack-YYYYMMDD_HHMMSS.txt    # Attack report
├── vote-spam-attack-YYYYMMDD_HHMMSS.txt    # Spam report
├── network-metrics-YYYYMMDD_HHMMSS.txt     # Resource metrics
├── container-logs-YYYYMMDD_HHMMSS.tar.gz   # Compressed logs
└── summary-YYYYMMDD_HHMMSS.txt             # Test summary
```

## Development Workflow

### Adding New Attack Scenarios

1. Create new Go program in test/docker/byzantine-test/
2. Follow the same pattern as existing attacks:
   - Config struct
   - Initialize() for setup
   - Start() for execution
   - GenerateReport() for results
3. Add to run-byzantine-tests.sh
4. Update README.md with description

### Modifying Network Topology

To change validator count:

1. Edit docker-compose-25node.yml
2. Add/remove validator services
3. Update node indices (--node=N)
4. Update port mappings
5. Update volumes section
6. Update README.md with new topology

### Custom Metrics

To add custom metrics:

1. Expose metrics endpoint in validator
2. Query in attack programs:
   ```go
   resp, _ := http.Get("http://localhost:27100/metrics")
   ```
3. Parse and include in report

## Integration with CI/CD

### GitLab CI Example

```yaml
byzantine-tests:
  stage: security-test
  image: docker:latest
  services:
    - docker:dind
  variables:
    DOCKER_HOST: tcp://docker:2375
    DOCKER_TLS_CERTDIR: ""
  before_script:
    - apk add --no-cache bash go
  script:
    - cd test/docker/byzantine-test
    - ATTACK_DURATION=2m ./run-byzantine-tests.sh
  artifacts:
    when: always
    paths:
      - test/docker/byzantine-test/results/
    reports:
      junit: test/docker/byzantine-test/results/junit.xml
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
    - if: '$CI_COMMIT_BRANCH == "main"'
```

### GitHub Actions Example

```yaml
name: Byzantine Tests

on:
  pull_request:
  push:
    branches: [main]

jobs:
  byzantine-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: docker/setup-buildx-action@v2
      - name: Run Byzantine tests
        run: |
          cd test/docker/byzantine-test
          ATTACK_DURATION=2m ./run-byzantine-tests.sh
      - uses: actions/upload-artifact@v3
        if: always()
        with:
          name: byzantine-test-results
          path: test/docker/byzantine-test/results/
```

## Troubleshooting Guide

### Build Issues

**Problem:** Docker build fails

**Solutions:**
- Clear Docker cache: `docker system prune -a`
- Check Dockerfile syntax
- Verify Go version compatibility
- Check disk space: `df -h`

### Runtime Issues

**Problem:** Containers won't start

**Solutions:**
- Check port conflicts: `lsof -i :27000-27124`
- Verify memory available: `free -h`
- Check Docker daemon: `systemctl status docker`
- Review logs: `docker compose logs`

**Problem:** Health checks failing

**Solutions:**
- Increase start_period in healthcheck
- Check API endpoint responds: `curl localhost:27100/status`
- Review container logs for errors
- Verify network connectivity

### Test Issues

**Problem:** Attack programs crash

**Solutions:**
- Check Go version: `go version`
- Run with verbose logging: `go run -v program.go`
- Check dependencies: `go mod verify`
- Review stack trace

**Problem:** Reports show failures

**Solutions:**
- Check if enough validators are healthy
- Verify consensus is progressing normally
- Review specific failure message
- Check if security fixes are applied

## Performance Tuning

### For Faster Tests

```bash
# Reduce validator count
# Edit docker-compose-25node.yml to use 12 validators instead of 25

# Reduce attack duration
ATTACK_DURATION=30s ./run-byzantine-tests.sh

# Skip build
SKIP_BUILD=1 ./run-byzantine-tests.sh
```

### For Stress Testing

```bash
# Increase Byzantine validators
MALICIOUS_NODES=12 ./run-byzantine-tests.sh

# Increase spam load
NUM_SPAMMERS=200 VOTES_PER_SECOND=2000 ./run-byzantine-tests.sh

# Longer attack duration
ATTACK_DURATION=30m ./run-byzantine-tests.sh
```

### Resource Optimization

```yaml
# Lower memory per container
mem_limit: 1g

# Add CPU limits
cpus: 1.0

# Use tmpfs for faster I/O
tmpfs:
  - /tmp
```

## References

- Docker Compose docs: https://docs.docker.com/compose/
- Byzantine Fault Tolerance: https://en.wikipedia.org/wiki/Byzantine_fault
- Go testing: https://pkg.go.dev/testing
- Accumulate consensus: /pkg/consensus/README.md
