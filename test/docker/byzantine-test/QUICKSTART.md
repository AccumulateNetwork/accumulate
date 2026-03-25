# Quick Start Guide - Byzantine Fault Tolerance Tests

## Prerequisites

- Docker and Docker Compose installed
- At least 52GB RAM available
- Go 1.23 or later

## Quick Test Run

### 1. Run Full Test Suite (Recommended)

```bash
cd test/docker/byzantine-test
./run-byzantine-tests.sh
```

This will:
1. Build Docker images (first run takes ~5 minutes)
2. Start 25-node network
3. Run Byzantine attack test (5 minutes)
4. Run vote spam attack test (3 minutes)
5. Generate reports
6. Clean up

**Total time:** ~20 minutes (first run), ~15 minutes (subsequent runs)

### 2. Run Shorter Test

```bash
cd test/docker/byzantine-test
ATTACK_DURATION=1m ./run-byzantine-tests.sh
```

**Total time:** ~5 minutes

### 3. Manual Step-by-Step

```bash
cd test/docker/byzantine-test

# 1. Start network
docker compose -f docker-compose-25node.yml up -d

# 2. Wait for network (important!)
sleep 60

# 3. Build and run Byzantine attack
go build byzantine-attack.go
ATTACK_DURATION=1m ./byzantine-attack

# 4. Build and run vote spam attack
go build vote-spam-attack.go
ATTACK_DURATION=1m ./vote-spam-attack

# 5. View results
cat /tmp/byzantine-attack-report.txt
cat /tmp/vote-spam-attack-report.txt

# 6. Cleanup
docker compose -f docker-compose-25node.yml down -v
```

## Understanding Results

### Success Indicators

**Byzantine Attack Test (Issue #3869):**
- ✅ "SUCCESS: Consensus continued despite Byzantine attack"
- Rounds should advance during the attack
- Duplicate votes should be rejected

**Vote Spam Attack Test (Issue #3870):**
- ✅ "SUCCESS: Validators efficiently rejected spam (max CPU: X.XX%)"
- Max CPU should be < 10%
- All spam votes should be rejected

### Failure Indicators

- ❌ "FAILURE: Consensus stalled"
- ❌ "FAILURE: Validators used excessive CPU"
- Container health checks failing
- Insufficient healthy nodes

## Common Issues

### Issue: "Insufficient healthy nodes"

**Solution:**
```bash
# Increase wait time
docker compose -f docker-compose-25node.yml down -v
docker compose -f docker-compose-25node.yml up -d
sleep 120  # Wait longer
```

### Issue: Docker build fails

**Solution:**
```bash
# Clean Docker cache
docker system prune -a
# Rebuild
docker compose -f docker-compose-25node.yml build --no-cache
```

### Issue: Out of memory

**Solution:**
```bash
# Reduce to 3-BVN test with 4 validators each (12 nodes)
docker compose -f ../docker-compose.yml up -d
```

### Issue: Port already in use

**Solution:**
```bash
# Check what's using the ports
lsof -i :27000-27124
# Or change ports in docker-compose-25node.yml
```

## Viewing Logs

```bash
# All containers
docker compose -f docker-compose-25node.yml logs -f

# Specific BVN
docker compose -f docker-compose-25node.yml logs -f bvn0-0

# Bootstrap node
docker compose -f docker-compose-25node.yml logs -f bootstrap

# Search for errors
docker compose -f docker-compose-25node.yml logs | grep -i error
```

## Monitoring During Tests

### In one terminal - watch logs:
```bash
docker compose -f docker-compose-25node.yml logs -f | grep -E "(vote|consensus|Byzantine)"
```

### In another terminal - watch stats:
```bash
watch "docker stats --no-stream --format 'table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}'"
```

### Check consensus progress:
```bash
# Query BVN0 API
curl http://localhost:8080/v3/describe

# Check validator status
docker exec byzantine-bvn0-0 accumulated status
```

## Environment Variable Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `SKIP_BUILD` | 0 | Skip Docker build step |
| `SKIP_NETWORK_START` | 0 | Use existing network |
| `KEEP_NETWORK` | 0 | Don't cleanup after tests |
| `ATTACK_DURATION` | 5m | Duration for each attack |
| `MALICIOUS_NODES` | 9 | Byzantine validators |
| `DUPLICATE_VOTES` | 10 | Dupes per round |
| `NUM_SPAMMERS` | 100 | Spam nodes |
| `VOTES_PER_SECOND` | 1000 | Spam rate per node |
| `CPU_THRESHOLD` | 10.0 | Max CPU % for spam |

## Example Commands

### Fast iteration during development:
```bash
# First run - build everything
./run-byzantine-tests.sh

# Keep network, just rerun tests
SKIP_BUILD=1 SKIP_NETWORK_START=1 ATTACK_DURATION=30s ./run-byzantine-tests.sh

# Done - cleanup
docker compose -f docker-compose-25node.yml down -v
```

### Stress test:
```bash
# Maximum Byzantine attack (12 out of 25 validators)
MALICIOUS_NODES=12 DUPLICATE_VOTES=100 ./run-byzantine-tests.sh
```

### Heavy spam load:
```bash
# 200 spammers @ 2000 votes/sec = 400k votes/sec
NUM_SPAMMERS=200 VOTES_PER_SECOND=2000 ./run-byzantine-tests.sh
```

## Next Steps

After running tests successfully:

1. Review reports in `results/` directory
2. Check `README.md` for detailed documentation
3. Modify attack parameters to test edge cases
4. Integrate into CI/CD pipeline

## CI/CD Integration

Add to `.gitlab-ci.yml`:

```yaml
byzantine-tests:
  stage: test
  script:
    - cd test/docker/byzantine-test
    - ATTACK_DURATION=2m ./run-byzantine-tests.sh
  artifacts:
    paths:
      - test/docker/byzantine-test/results/
    expire_in: 1 week
  rules:
    - if: $CI_MERGE_REQUEST_TARGET_BRANCH_NAME == "main"
```
