# Accumulate Load Test Guide

## Available Load Tests

### 1. TestStreamlinedLoad
The main configurable load test with full command-line flag support.

**Usage:**
```bash
go test -v -run TestStreamlinedLoad -args [FLAGS]
```

**Available Flags:**
- `-txs` : Number of transactions to send (default: 1000)
- `-k` : Number of sender accounts (default: 10)
- `-a` : Number of receiver accounts (default: 10)
- `-tps` : Target transactions per second (0 = unlimited, default: 0)
- `-timeout` : Settlement timeout (0 = auto-calculated, default: 0)
- `-verbose` : Enable verbose logging (default: false)

**Examples:**
```bash
# 50k transactions at 100 TPS
go test -v -run TestStreamlinedLoad -args -txs 50000 -tps 100 -k 20 -a 20

# 100k transactions at 200 TPS with 40 senders/receivers
go test -v -run TestStreamlinedLoad -args -txs 100000 -tps 200 -k 40 -a 40 -timeout 15m

# 10k transactions at maximum speed (no rate limit)
go test -v -run TestStreamlinedLoad -args -txs 10000 -tps 0 -k 10 -a 10
```

### 2. TestSimple50K
Simplified 50k transaction test with defaults optimized for 50k transactions.

**Defaults:**
- Transactions: 50,000
- Senders: 20
- Receivers: 20
- Target TPS: 100
- Amount per tx: 0.001 ACME

**Usage:**
```bash
go test -v -run TestSimple50K -timeout 20m
```

### 3. TestSimple100K
Simplified 100k transaction test with defaults optimized for 100k transactions.

**Defaults:**
- Transactions: 100,000
- Senders: 40
- Receivers: 40
- Target TPS: 200
- Amount per tx: 0.001 ACME

**Usage:**
```bash
go test -v -run TestSimple100K -timeout 45m
```

## Important Notes

1. **Devnet Must Be Running**: All tests require a local devnet running on `127.0.0.1:26660`

2. **Automatic Endpoint Discovery**: Tests use smart discovery to find the devnet endpoint automatically

3. **Rate Limiting**: The TPS parameter controls the submission rate. Lower TPS provides better stability.

4. **Timeout Calculation**: 
   - Auto-calculated based on transaction count if not specified
   - 10k+ txs: 10 minutes
   - 1k+ txs: 5 minutes
   - <1k txs: 3 minutes

5. **Success Criteria**: Tests pass if they achieve at least 80% of target TPS

## Troubleshooting

### Test Fails to Start
- Check devnet is running: `ps aux | grep accumulated`
- Verify ports are listening: `ss -tlnp | grep 266`

### Low Success Rate
- Reduce TPS (try 50 instead of 200)
- Increase sender/receiver accounts for better distribution
- Add more funding per account

### Funding Issues
- Tests automatically use faucet for funding
- Each sender gets enough ACME for their share of transactions
- Credits are added automatically

## Performance Guidelines

Based on testing:
- 50 TPS: 100% success rate, very stable
- 100 TPS: 99%+ success rate, stable
- 200 TPS: May have lower success rate, depends on system resources

## Monitoring During Test

Tests provide real-time updates:
- Progress updates every 1000-5000 transactions
- Current TPS calculation
- Success/failure counts
- Final verification of balances

## Example Test Runs

### Small Test (Quick Verification)
```bash
go test -v -run TestStreamlinedLoad -args -txs 1000 -tps 50 -k 5 -a 5
```

### Medium Test (Standard Load)
```bash
go test -v -run TestStreamlinedLoad -args -txs 50000 -tps 100 -k 20 -a 20
```

### Large Test (Heavy Load)
```bash
go test -v -run TestStreamlinedLoad -args -txs 100000 -tps 50 -k 40 -a 40 -timeout 40m
```

### Stress Test (Maximum Speed)
```bash
go test -v -run TestStreamlinedLoad -args -txs 10000 -tps 0 -k 20 -a 20
```