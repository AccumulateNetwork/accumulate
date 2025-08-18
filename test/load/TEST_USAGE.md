# Streamlined Load Test Usage

## Single Test, Complete Control

The streamlined load test is now a single test function (`TestStreamlinedLoad`) with all behavior controlled by command-line flags. No more hardcoded variations or multiple test functions.

## Running the Test

**IMPORTANT**: Use `-args` before custom flags to pass them to the test:

```bash
# Basic usage - run with defaults
go test -v -run TestStreamlinedLoad

# With custom parameters (note the -args)
go test -v -run TestStreamlinedLoad -args -txs=5000 -k=20 -a=10 -tps=100

# Full example with all flags
go test -v -timeout 30m -run TestStreamlinedLoad -args \
  -txs=10000 \
  -k=50 \
  -a=20 \
  -tps=200 \
  -timeout=10m \
  -verbose
```

## Flag Reference

- `-txs`: Number of transactions to send (default: 1000)
- `-k`: Number of sender accounts (default: 10)
- `-a`: Number of receiver accounts (default: 10)
- `-tps`: Target transactions per second, 0 = unlimited (default: 0)
- `-timeout`: Settlement verification timeout (default: auto-calculated)
- `-verbose`: Enable detailed logging (default: false)

## Common Scenarios

### Quick Smoke Test
```bash
go test -v -run TestStreamlinedLoad -args -txs=100
```

### Network Stress Test
```bash
go test -v -run TestStreamlinedLoad -args -txs=50000 -tps=500
```

### Account Scaling Test
```bash
go test -v -run TestStreamlinedLoad -args -txs=5000 -k=100 -a=50
```

### Rate Limited Test
```bash
go test -v -run TestStreamlinedLoad -args -txs=10000 -tps=100
```

### Debug Failed Test
```bash
go test -v -run TestStreamlinedLoad -args -txs=100 -verbose
```

## What the Test Does

1. Creates unique test accounts using timestamp-based seeds
2. Funds sender accounts with calculated ACME amounts
3. Adds credits to all accounts for transactions
4. Sends transactions at specified rate (or max speed)
5. Waits for settlement and verifies all balances
6. Reports detailed results and any discrepancies

## Automatic Calculations

The test automatically calculates:
- ACME needed per sender based on transaction count
- Credits required based on transaction volume
- Settlement timeout based on number of transactions
- Expected balances for verification

## Success Criteria

The test passes when:
- All sender balances match expected (initial - sent)
- All receiver balances match expected (received amount)
- Total ACME in system is conserved
- All transactions are accounted for