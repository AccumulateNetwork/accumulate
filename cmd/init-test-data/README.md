# init-test-data

Initialize test accounts on an Accumulate network using the test wallet.

## Overview

The `init-test-data` tool creates accounts on a running Accumulate network based on keys from the test wallet. It handles:

- Lite account funding
- ADI identity creation
- Token account creation and funding
- Data account creation
- Credit allocation for key pages

## Installation

```bash
go install ./cmd/init-test-data
```

## Usage

### Basic Usage

```bash
# Initialize all accounts from default wallet
init-test-data

# Use custom wallet and node
init-test-data \
  -wallet ~/.accumulate/my-wallet.json \
  -node http://localhost:8080/v3
```

### Options

| Flag | Description | Default |
|------|-------------|---------|
| `-wallet <path>` | Path to test wallet | `~/.accumulate/test-wallet.json` |
| `-node <url>` | Accumulate node API endpoint | `http://localhost:8080/v3` |
| `-batch-size <n>` | Accounts per batch | 10 |
| `-concurrency <n>` | Concurrent workers | 5 |
| `-initial-funds <n>` | Initial ACME per account | 1000 |
| `-skip-lite` | Skip lite account creation | false |
| `-skip-adi-token` | Skip ADI token accounts | false |
| `-skip-adi-data` | Skip ADI data accounts | false |
| `-dry-run` | Preview without executing | false |

### Examples

**Initialize everything:**
```bash
init-test-data
```

**Fast initialization:**
```bash
init-test-data -batch-size 20 -concurrency 10
```

**Initialize only lite accounts:**
```bash
init-test-data -skip-adi-token -skip-adi-data
```

**Initialize only ADI accounts:**
```bash
init-test-data -skip-lite
```

**Preview the plan:**
```bash
init-test-data -dry-run
```

**Custom funding:**
```bash
init-test-data -initial-funds 5000
```

## Wrapper Script

For convenience, use the wrapper script:

```bash
# Standard initialization
./test/scripts/init-test-data.sh

# Custom configuration
./test/scripts/init-test-data.sh \
  --batch-size 20 \
  --concurrency 10 \
  --initial-funds 5000

# Preview only
./test/scripts/init-test-data.sh --dry-run
```

The wrapper script:
- Checks prerequisites (wallet exists, node is running)
- Builds the tool if not installed
- Provides friendly prompts and colors
- Saves summary to `/tmp/init-test-data-summary.json`

## Process

### 1. Verification

- Loads test wallet
- Verifies funder account exists
- Checks funder has sufficient balance
- Estimates required funds

### 2. Lite Accounts

For each lite account in wallet:
- Send tokens from funder to lite address
- Wait for transaction confirmation
- Account is automatically created on first receipt

### 3. ADI Token Accounts

For each ADI token account:
1. Create ADI identity with key from wallet
2. Create key book and key page
3. Add 10,000 credits to key page
4. Create token account under the ADI
5. Fund token account from funder

### 4. ADI Data Accounts

For each ADI data account:
1. Create ADI identity with key from wallet
2. Create key book and key page
3. Add 10,000 credits to key page
4. Create data account under the ADI

## Output

### Console Output

```
Starting test data initialization
Wallet: /home/user/.accumulate/test-wallet.json
Node: http://localhost:8080/v3
Funder: acc://test-funder.acme/tokens
Verifying funder account...
Funder balance: 100000 ACME
Estimated required funds: 3000 ACME for 3000 accounts

Initializing 1000 lite accounts...
Progress: 500/1000 lite accounts created
Progress: 1000/1000 lite accounts created
Lite accounts complete: 1000 created, 0 failed

Initializing 1000 ADI token accounts...
Progress: 500/1000 ADI token accounts created
Progress: 1000/1000 ADI token accounts created
ADI token accounts complete: 1000 created, 0 failed

Initializing 1000 ADI data accounts...
Progress: 500/1000 ADI data accounts created
Progress: 1000/1000 ADI data accounts created
ADI data accounts complete: 1000 created, 0 failed

==================================================
Initialization Complete
==================================================
Duration: 15m30s

Results:
  Lite Accounts:      1000 created, 0 failed
  ADI Token Accounts: 1000 created, 0 failed
  ADI Data Accounts:  1000 created, 0 failed

Total: 3000 created, 0 failed

Summary saved to: /tmp/init-test-data-summary.json
```

### Summary File

The tool saves a JSON summary to `/tmp/init-test-data-summary.json`:

```json
{
  "timestamp": "2026-03-22T13:30:00Z",
  "duration": "15m30s",
  "node": "http://localhost:8080/v3",
  "wallet": "/home/user/.accumulate/test-wallet.json",
  "lite": {
    "created": 1000,
    "failed": 0
  },
  "adi_token": {
    "created": 1000,
    "failed": 0
  },
  "adi_data": {
    "created": 1000,
    "failed": 0
  },
  "total": 3000,
  "failed": 0,
  "batch_size": 10,
  "concurrency": 5
}
```

## Performance

### Timing Estimates

Based on default configuration (batch size 10, concurrency 5):

- **Lite accounts**: ~30-60 seconds per 100 accounts
- **ADI token accounts**: ~2-3 minutes per 100 accounts (4 transactions each)
- **ADI data accounts**: ~1.5-2 minutes per 100 accounts (3 transactions each)

**Total for 3000 accounts**: ~15-30 minutes

### Optimization

To speed up initialization:

```bash
# Increase batch size and concurrency
init-test-data -batch-size 50 -concurrency 20
```

**Caution**: Too high concurrency may overwhelm the node or hit rate limits.

## Prerequisites

### 1. Test Wallet

Create a test wallet first:

```bash
test-wallet create
```

### 2. Running Network

Start the Accumulate network:

```bash
./test/docker/manage.sh start
```

### 3. Funded Funder Account

The funder account must exist on the network and have sufficient balance:

```bash
# For devnet, the funder should be pre-funded
# Check funder balance
curl http://localhost:8080/v3/query \
  -d '{"url":"acc://test-funder.acme/tokens"}'
```

## Troubleshooting

### Funder account not found

```
Error: query funder account: not found
```

**Solution**: The funder account doesn't exist on the network. For devnet, you may need to use a different funder URL that matches a pre-funded account in the devnet configuration.

### Insufficient funds

```
WARNING: Funder balance may be insufficient
  Have: 1000 ACME, Need: ~3000 ACME
```

**Solution**: Fund the funder account or reduce the number of accounts in the wallet.

### Transaction timeouts

```
Failed to create acc://test-a00000123.acme/tokens: timeout waiting for transaction
```

**Solution**:
- Reduce concurrency: `-concurrency 2`
- Increase transaction timeout in code
- Check network health

### Node not responding

```
Error: submit: connection refused
```

**Solution**:
- Verify node is running: `curl http://localhost:8080/v3/describe`
- Check node URL is correct
- Start network: `./test/docker/manage.sh start`

### Too many failures

```
WARNING: 50 accounts failed to initialize
```

**Solution**:
- Check logs for specific errors
- Verify network stability
- Try with smaller batch size
- Re-run for failed accounts only (feature TBD)

## Integration

### With Reset Script

```bash
# Complete reset and initialization
./test/scripts/reset.sh
./test/scripts/init-test-data.sh
```

### With Monitoring

```bash
# Start monitoring first
./test/monitor/network-monitor.sh &

# Initialize data
init-test-data

# Monitor will track resource usage
```

### With Load Generator

```bash
# Initialize test data
init-test-data

# Run load test
load-generator \
  --nodes http://localhost:8080/v3 \
  --tps 1000 \
  --duration 30m
```

## See Also

- [Test Wallet](../../test/wallet/README.md) - Creating and managing test wallets
- [Docker Deployment](../../test/docker/README.md) - Running the network
- [Load Generator](../load-generator/) - Generating transaction load
