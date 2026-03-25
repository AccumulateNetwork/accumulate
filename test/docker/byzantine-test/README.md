# Byzantine Fault Tolerance Test Suite

This directory contains a comprehensive test infrastructure for validating Byzantine fault tolerance in the Accumulate consensus protocol under adversarial conditions.

## Overview

The test suite validates security fixes for:

- **Issue #3869**: Duplicate vote handling from malicious validators
- **Issue #3870**: Vote spam protection from non-validator nodes

## Network Architecture

### 25-Node Docker Testnet

The test network consists of:

- **3 Block Validator Networks (BVNs)**
  - BVN0: 8 validators
  - BVN1: 8 validators
  - BVN2: 8 validators
- **1 Directory Network (DN)**
  - DN: 1 validator
- **Total**: 25 validators + 1 bootstrap node

Each node is:
- Isolated in its own Docker container
- Limited to 2GB memory
- Connected via a dedicated test network (172.30.0.0/16)

### Byzantine Fault Tolerance

The network can tolerate up to 8 Byzantine validators (33% of 25) while maintaining consensus.

The test suite intentionally creates **9 malicious validators** (36% fault rate) to validate that:
1. Consensus continues with honest validators
2. Duplicate votes are detected and rejected
3. Vote spam is handled efficiently

## Components

### 1. docker-compose-25node.yml

Docker Compose configuration for the 25-node network.

**Features:**
- Separate containers for each validator
- Proper port mapping for P2P and API access
- Health checks on all nodes
- Persistent volumes for node data

**Usage:**
```bash
# Start network
docker compose -f docker-compose-25node.yml up -d

# Check status
docker compose -f docker-compose-25node.yml ps

# View logs
docker compose -f docker-compose-25node.yml logs -f

# Stop network
docker compose -f docker-compose-25node.yml down -v
```

### 2. byzantine-attack.go

Simulates a Byzantine attack with malicious validators sending duplicate votes.

**Attack Scenario:**
- 9 validators turn malicious (36% Byzantine fault)
- Each malicious validator sends 10 duplicate votes per round
- Tests duplicate vote detection (Issue #3869)

**Configuration (via environment variables):**
```bash
MALICIOUS_NODES=9           # Number of Byzantine validators
DUPLICATE_VOTES=10          # Duplicate votes per round
ATTACK_DURATION=5m          # Attack duration
```

**Expected Behavior:**
- Honest validators (16 out of 25) should maintain consensus
- Duplicate votes should be detected and rejected
- Consensus should continue despite Byzantine attack

**Build and Run:**
```bash
cd test/docker/byzantine-test
go build byzantine-attack.go
./byzantine-attack
```

### 3. vote-spam-attack.go

Simulates vote spam from non-validator nodes.

**Attack Scenario:**
- 100 non-validator nodes send spam votes
- Each sends 1000 votes per second
- Total: 100,000 votes/sec spam load
- Tests non-validator rejection efficiency (Issue #3870)

**Configuration (via environment variables):**
```bash
NUM_SPAMMERS=100            # Number of spam nodes
VOTES_PER_SECOND=1000       # Votes per second (per spammer)
CPU_THRESHOLD=10.0          # Max acceptable CPU usage (%)
ATTACK_DURATION=3m          # Attack duration
```

**Expected Behavior:**
- Validators should reject non-validator votes early (before signature verification)
- CPU usage for spam processing should remain < 10%
- Consensus should continue unaffected

**Build and Run:**
```bash
cd test/docker/byzantine-test
go build vote-spam-attack.go
./vote-spam-attack
```

### 4. run-byzantine-tests.sh

Automated test runner that orchestrates the complete test suite.

**Workflow:**
1. Build Docker images
2. Start 25-node network
3. Wait for network stabilization
4. Run Byzantine attack test
5. Run vote spam attack test
6. Collect metrics and logs
7. Generate comprehensive reports
8. Clean up network (optional)

**Usage:**
```bash
cd test/docker/byzantine-test
./run-byzantine-tests.sh
```

**Environment Variables:**
```bash
SKIP_BUILD=1              # Skip Docker build
SKIP_NETWORK_START=1      # Use existing network
KEEP_NETWORK=1            # Don't tear down after tests
TEST_DURATION=5m          # Duration for each test
MALICIOUS_NODES=9         # Byzantine validators
NUM_SPAMMERS=100          # Spam nodes
```

**Example - Quick Test:**
```bash
# Run shorter test with fewer spammers
ATTACK_DURATION=2m NUM_SPAMMERS=50 ./run-byzantine-tests.sh
```

**Example - Keep Network Running:**
```bash
# Run tests but keep network for manual inspection
KEEP_NETWORK=1 ./run-byzantine-tests.sh
```

## Test Results

Results are saved in the `results/` directory:

```
results/
├── byzantine-attack-YYYYMMDD_HHMMSS.txt
├── vote-spam-attack-YYYYMMDD_HHMMSS.txt
├── network-metrics-YYYYMMDD_HHMMSS.txt
├── container-logs-YYYYMMDD_HHMMSS.tar.gz
└── summary-YYYYMMDD_HHMMSS.txt
```

### Report Contents

**Byzantine Attack Report:**
- Attack configuration
- Malicious validator count
- Duplicate votes sent
- Consensus progress (rounds advanced)
- Success/failure verdict

**Vote Spam Attack Report:**
- Spam node count and rate
- Total votes sent
- CPU usage statistics
- Spam rejection efficiency
- Success/failure verdict

**Summary Report:**
- Overall test configuration
- Quick status for both tests
- Paths to detailed reports

## Performance Metrics

### Byzantine Attack Test (Issue #3869)

**Success Criteria:**
- ✅ Honest validators maintain consensus
- ✅ Duplicate votes detected and rejected
- ✅ Consensus rounds continue advancing

**Metrics Tracked:**
- Rounds advanced during attack
- Certificates created
- Votes processed vs rejected

### Vote Spam Attack Test (Issue #3870)

**Success Criteria:**
- ✅ Non-validator votes rejected early
- ✅ Validator CPU usage < 10%
- ✅ No degradation in consensus

**Metrics Tracked:**
- Validator CPU usage (average and peak)
- Spam votes rejected
- Vote processing rate

## Resource Requirements

### Minimum System Requirements

- **CPU**: 8 cores (26 containers + attack tools)
- **Memory**: 52GB (26 containers × 2GB)
- **Disk**: 20GB (for logs and data)
- **Network**: Docker bridge networking support

### Recommended System Requirements

- **CPU**: 16 cores
- **Memory**: 64GB
- **Disk**: 50GB SSD
- **Network**: 1Gbps

## Troubleshooting

### Network Won't Start

```bash
# Check Docker resources
docker system df

# Clean up old containers
docker compose -f docker-compose-25node.yml down -v

# Check logs
docker compose -f docker-compose-25node.yml logs
```

### Insufficient Healthy Nodes

```bash
# Check container health
docker compose -f docker-compose-25node.yml ps

# Inspect specific container
docker logs byzantine-bvn0-0

# Increase stabilization wait time
# Edit run-byzantine-tests.sh and increase sleep duration
```

### Build Failures

```bash
# Clean build cache
go clean -cache

# Rebuild with verbose output
go build -v byzantine-attack.go
go build -v vote-spam-attack.go
```

### High CPU Usage

If tests show high CPU usage (> 10% threshold):

1. Verify early rejection is working (check vote_handler.go line 39)
2. Check if signature verification is being skipped for non-validators
3. Review logs for repeated validation attempts

## Architecture Details

### Vote Processing Pipeline

The test suite validates this security pipeline:

```
Incoming Vote
    ↓
1. Signature verification (crypto)
    ↓
2. Committee membership check ← Issue #3870 (non-validator rejection)
    ↓
3. Duplicate vote detection ← Issue #3869 (duplicate rejection)
    ↓
4. Vote limit enforcement (spam protection)
    ↓
Accept Vote
```

### Security Properties Tested

1. **Byzantine Fault Tolerance**
   - System tolerates f < n/3 Byzantine validators
   - Honest validators (2f+1) maintain consensus

2. **Duplicate Vote Prevention**
   - Malicious validators cannot gain extra votes
   - Vote deduplication per header per validator

3. **Spam Resistance**
   - Non-validator votes rejected early (< 10% CPU)
   - No expensive operations on spam votes
   - Vote rate limiting per header

## Development

### Modifying Attack Parameters

Edit test configurations in the attack programs:

**byzantine-attack.go:**
```go
config := ByzantineAttackConfig{
    NumMaliciousNodes:      9,   // Increase for stronger attack
    DuplicateVotesPerRound: 10,  // More duplicates
    AttackDuration:         5 * time.Minute,
}
```

**vote-spam-attack.go:**
```go
config := VoteSpamAttackConfig{
    NumSpammers:         100,   // More spammers
    VotesPerSecond:      1000,  // Higher rate
    CPUThresholdPercent: 10.0,  // Stricter threshold
}
```

### Adding New Tests

1. Create new attack simulator in Go
2. Add to `run-byzantine-tests.sh`
3. Update this README with test description

### Running Individual Tests

```bash
# Start network only
docker compose -f docker-compose-25node.yml up -d

# Run single test
go run byzantine-attack.go

# Keep network running for inspection
KEEP_NETWORK=1 ./run-byzantine-tests.sh
```

## References

- **Issue #3869**: Duplicate vote handling
- **Issue #3870**: Non-validator vote spam protection
- **Vote Handler**: `/pkg/consensus/primary/vote_handler.go`
- **Vote Types**: `/pkg/consensus/types/vote.go`
- **Gossip Layer**: `/pkg/consensus/gossip/gossip.go`

## Support

For issues or questions:

1. Check Docker logs: `docker compose logs`
2. Review test reports in `results/`
3. Consult vote handler implementation
4. File issue with logs and reports attached
