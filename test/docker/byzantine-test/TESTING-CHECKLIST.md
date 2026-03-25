# Byzantine Test Execution Checklist

Use this checklist when running Byzantine fault tolerance tests to ensure comprehensive validation.

## Pre-Test Checklist

### System Requirements

- [ ] Docker version 20.10+ installed
- [ ] Docker Compose version 2.0+ installed
- [ ] Go 1.23+ installed
- [ ] At least 52GB RAM available
- [ ] At least 20GB disk space free
- [ ] No other containers using ports 27000-27124, 8080-8083

### Verification Commands

```bash
# Check Docker
docker --version
docker compose version

# Check Go
go version

# Check resources
free -h  # Memory
df -h    # Disk
```

## Test Execution Checklist

### 1. Environment Setup

- [ ] Navigate to test directory
  ```bash
  cd test/docker/byzantine-test
  ```

- [ ] Verify all files present
  ```bash
  ls -l *.go *.yml *.sh *.md
  ```

- [ ] Clean up any previous test runs
  ```bash
  docker compose -f docker-compose-25node.yml down -v
  rm -rf results/* logs/*
  ```

### 2. Build Phase

- [ ] Build Docker images
  ```bash
  docker compose -f docker-compose-25node.yml build
  ```

- [ ] Verify images built successfully
  ```bash
  docker images | grep accumulate
  ```

- [ ] Build attack tools
  ```bash
  go build byzantine-attack.go
  go build vote-spam-attack.go
  ```

### 3. Network Startup

- [ ] Start 25-node network
  ```bash
  docker compose -f docker-compose-25node.yml up -d
  ```

- [ ] Wait for network stabilization (60 seconds minimum)
  ```bash
  sleep 60
  ```

- [ ] Check all containers are running
  ```bash
  docker compose -f docker-compose-25node.yml ps
  ```
  Expected: 26 containers (25 validators + 1 bootstrap)

- [ ] Verify health checks passing
  ```bash
  docker compose -f docker-compose-25node.yml ps | grep -c "healthy"
  ```
  Expected: At least 17 healthy (2/3 for quorum)

- [ ] Test API endpoints
  ```bash
  curl -s http://localhost:8080/v3/describe | jq .
  curl -s http://localhost:8081/v3/describe | jq .
  curl -s http://localhost:8082/v3/describe | jq .
  curl -s http://localhost:8083/v3/describe | jq .
  ```

### 4. Byzantine Attack Test (Issue #3869)

- [ ] Start attack test
  ```bash
  MALICIOUS_NODES=9 DUPLICATE_VOTES=10 ATTACK_DURATION=5m ./byzantine-attack
  ```

- [ ] Monitor during execution
  ```bash
  # In another terminal
  docker stats --no-stream
  docker compose logs -f | grep -i "vote\|byzantine"
  ```

- [ ] Verify attack completes
  - [ ] No crashes or panics
  - [ ] Report file generated: `/tmp/byzantine-attack-report.txt`

- [ ] Review results
  ```bash
  cat /tmp/byzantine-attack-report.txt
  ```

- [ ] Check success criteria:
  - [ ] ✅ "SUCCESS: Consensus continued despite Byzantine attack"
  - [ ] Rounds advanced during attack
  - [ ] Duplicate votes were rejected
  - [ ] Certificates created successfully

### 5. Vote Spam Attack Test (Issue #3870)

- [ ] Start spam test
  ```bash
  NUM_SPAMMERS=100 VOTES_PER_SECOND=1000 ATTACK_DURATION=3m ./vote-spam-attack
  ```

- [ ] Monitor CPU usage
  ```bash
  # In another terminal
  watch "docker stats --no-stream | grep byzantine"
  ```

- [ ] Verify spam test completes
  - [ ] No crashes or panics
  - [ ] Report file generated: `/tmp/vote-spam-attack-report.txt`

- [ ] Review results
  ```bash
  cat /tmp/vote-spam-attack-report.txt
  ```

- [ ] Check success criteria:
  - [ ] ✅ "SUCCESS: Validators efficiently rejected spam"
  - [ ] Max validator CPU < 10%
  - [ ] All spam votes rejected
  - [ ] Consensus continued unaffected

### 6. Results Collection

- [ ] Copy reports to results directory
  ```bash
  mkdir -p results
  cp /tmp/byzantine-attack-report.txt results/
  cp /tmp/vote-spam-attack-report.txt results/
  ```

- [ ] Collect network metrics
  ```bash
  docker stats --no-stream > results/docker-stats.txt
  docker compose ps > results/container-status.txt
  ```

- [ ] Collect logs
  ```bash
  docker compose logs > results/all-logs.txt
  tar -czf results/container-logs.tar.gz results/*.txt
  ```

### 7. Cleanup

- [ ] Stop all containers
  ```bash
  docker compose -f docker-compose-25node.yml down
  ```

- [ ] Remove volumes (optional - removes all data)
  ```bash
  docker compose -f docker-compose-25node.yml down -v
  ```

- [ ] Verify cleanup
  ```bash
  docker compose -f docker-compose-25node.yml ps
  ```
  Expected: No containers running

## Post-Test Validation

### Results Analysis

- [ ] Both tests show "SUCCESS" verdict
- [ ] No consensus stalls observed
- [ ] No container crashes or restarts
- [ ] CPU usage within acceptable limits
- [ ] Memory usage within container limits
- [ ] Network I/O reasonable (no spam flooding disk)

### Report Review

Check each report contains:

**Byzantine Attack Report:**
- [ ] Attack configuration section
- [ ] Consensus progress metrics
- [ ] Success/failure verdict
- [ ] Recommendations

**Vote Spam Attack Report:**
- [ ] Attack configuration section
- [ ] CPU usage statistics
- [ ] Spam rejection stats
- [ ] Success/failure verdict
- [ ] Performance analysis

### Log Review

- [ ] No ERROR level messages related to consensus
- [ ] Duplicate votes logged and rejected
- [ ] Non-validator votes logged and rejected
- [ ] Certificates created successfully
- [ ] Rounds advancing normally

### Common Issues to Check

- [ ] If Byzantine test fails:
  - Check if < 17 validators healthy
  - Verify duplicate vote detection code
  - Review vote_handler.go line 87-91

- [ ] If Spam test fails:
  - Check CPU > 10%
  - Verify early rejection (before signature check)
  - Review vote_handler.go line 34-42

- [ ] If consensus stalled:
  - Check validator logs for errors
  - Verify network connectivity
  - Check if enough validators running

## Automated Test Run

For a complete automated run:

```bash
# Full test suite
./run-byzantine-tests.sh

# Quick test (2 minute attacks)
ATTACK_DURATION=2m ./run-byzantine-tests.sh

# Keep network running after tests
KEEP_NETWORK=1 ./run-byzantine-tests.sh
```

### Automated Run Checklist

- [ ] Script starts successfully
- [ ] Docker build completes
- [ ] Network starts and stabilizes
- [ ] Byzantine attack runs and completes
- [ ] Vote spam attack runs and completes
- [ ] Metrics collected
- [ ] Logs archived
- [ ] Summary report generated
- [ ] All tests show SUCCESS

## Documentation

After successful test execution:

- [ ] Results saved in `results/` directory
- [ ] Summary report reviewed
- [ ] Logs archived for future reference
- [ ] Any failures documented with reproduction steps
- [ ] Test execution time recorded

## Next Steps

After completing all tests:

- [ ] Review both attack reports
- [ ] Analyze any failures or warnings
- [ ] Update test parameters if needed
- [ ] Document any issues found
- [ ] Create issues for any bugs discovered
- [ ] Update README.md if procedures changed

## Test Matrix

For comprehensive validation, run tests with different parameters:

### Standard Test (Default)
- [ ] 9 malicious validators, 10 duplicates/round
- [ ] 100 spammers, 1000 votes/sec
- [ ] 5 minute attacks

### Light Test (Quick Validation)
- [ ] 5 malicious validators, 5 duplicates/round
- [ ] 50 spammers, 500 votes/sec
- [ ] 1 minute attacks

### Stress Test (Maximum Load)
- [ ] 12 malicious validators, 100 duplicates/round
- [ ] 200 spammers, 2000 votes/sec
- [ ] 10 minute attacks

### Edge Cases
- [ ] Exactly 8 malicious (BFT threshold)
- [ ] Single malicious validator
- [ ] All validators malicious (should fail)
- [ ] Zero spam (baseline performance)

## Sign-Off

Test executed by: ________________
Date: ________________
Test result: [ ] PASS [ ] FAIL
Notes: ________________________________________________
