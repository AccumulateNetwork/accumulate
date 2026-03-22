# Load Generator

The load generator is a configurable tool for comprehensive testing of the Accumulate protocol. It exercises the full range of Accumulate operations and distributes them across BVNs.

## Features

- Configurable target TPS (transactions per second)
- Support for multiple operation types
- Hot-reloadable configuration
- Comprehensive metrics collection
- Ramp-up schedule support
- Logs to `/tmp/load-generator.log`

## Supported Operations

The load generator supports the following operation types:

1. **Token Transfers**
   - Lite-to-Lite transfers
   - Lite-to-ADI transfers
   - ADI-to-ADI transfers

2. **Key Management**
   - Key rotation (UpdateKey)
   - Adding key books
   - Adding key pages
   - Updating key weights

3. **Account Operations**
   - Creating new accounts
   - Writing to data accounts

4. **Synthetic Transactions**
   - Generated automatically by the protocol for cross-BVN operations

## Configuration

The load generator uses a JSON configuration file. Example:

```json
{
  "server": "http://localhost:26660",
  "targetTPS": 100,
  "runtimeSeconds": 300,
  "rampUpSeconds": 30,
  "operations": {
    "liteToLiteTransfer": 30,
    "liteToADITransfer": 15,
    "adiToAdiTransfer": 15,
    "keyRotation": 10,
    "addKeyBook": 5,
    "addKeyPage": 5,
    "writeData": 15,
    "createAccount": 5,
    "updateKeyWeight": 0
  }
}
```

### Configuration Options

- `server`: Accumulate API server endpoint
- `targetTPS`: Target transactions per second (must be > 0)
- `runtimeSeconds`: Total runtime in seconds (0 = run indefinitely)
- `rampUpSeconds`: Time to ramp up from 0 to targetTPS (0 = start at full rate)
- `operations`: Operation mix percentages (must sum to 100)

### Operation Types

Each operation type is specified as a percentage of total operations:

- `liteToLiteTransfer`: Transfer tokens between lite accounts
- `liteToADITransfer`: Transfer tokens from lite account to ADI account
- `adiToAdiTransfer`: Transfer tokens between ADI accounts
- `keyRotation`: Rotate keys on accounts
- `addKeyBook`: Add new key books to ADI accounts
- `addKeyPage`: Add new key pages to key books
- `writeData`: Write data entries to data accounts
- `createAccount`: Create new token accounts under ADI
- `updateKeyWeight`: Update key weights by adding keys to pages

## Usage

```bash
# Build the debug tool
go build ./tools/cmd/debug

# Run the load generator
./debug loadgen config.json
```

The load generator will:

1. Load the configuration file
2. Set up test accounts (10 lite accounts, 3 ADI accounts)
3. Fund accounts with ACME tokens and credits
4. Start generating transactions according to the configured mix
5. Report metrics every 10 seconds
6. Watch for configuration changes and reload automatically

## Hot-Reload Configuration

The load generator watches the configuration file and automatically reloads it when changes are detected (checked every 5 seconds). This allows you to adjust the operation mix and target TPS while the load generator is running.

To adjust configuration:
1. Edit the config file
2. Save the changes
3. The load generator will reload within 5 seconds
4. New settings will take effect immediately

## Metrics

The load generator collects and reports the following metrics every 10 seconds:

- **TPS**: Current transactions per second
- **Total**: Total transactions submitted
- **Success**: Successfully submitted transactions
- **Failed**: Failed transaction submissions
- **Success Rate**: Percentage of successful submissions
- **Average Latency**: Average submission latency in milliseconds
- **Operation Counts**: Count of each operation type executed

Metrics are logged to `/tmp/load-generator.log`.

### Example Metrics Output

```
Metrics: TPS=98.50, Total=985, Success=980, Failed=5, Success Rate=99.49%, Avg Latency=45.23ms
Operation counts:
  liteToLiteTransfer: 295
  liteToADITransfer: 148
  adiToAdiTransfer: 147
  keyRotation: 99
  addKeyBook: 49
  addKeyPage: 50
  writeData: 148
  createAccount: 49
  updateKeyWeight: 0
```

## Distribution Across BVNs

Operations are distributed across BVNs based on the account URLs used:

- Lite accounts are distributed based on their key hash
- ADI accounts are distributed based on their identity URL
- Transfers between accounts in different BVNs generate synthetic transactions

The load generator creates multiple lite and ADI accounts to ensure operations are distributed across the network.

## Example Configurations

### High Token Transfer Load
```json
{
  "server": "http://localhost:26660",
  "targetTPS": 100,
  "runtimeSeconds": 600,
  "rampUpSeconds": 60,
  "operations": {
    "liteToLiteTransfer": 50,
    "liteToADITransfer": 25,
    "adiToAdiTransfer": 25,
    "keyRotation": 0,
    "addKeyBook": 0,
    "addKeyPage": 0,
    "writeData": 0,
    "createAccount": 0,
    "updateKeyWeight": 0
  }
}
```

### Balanced Protocol Test
```json
{
  "server": "http://localhost:26660",
  "targetTPS": 50,
  "runtimeSeconds": 1800,
  "rampUpSeconds": 30,
  "operations": {
    "liteToLiteTransfer": 20,
    "liteToADITransfer": 15,
    "adiToAdiTransfer": 15,
    "keyRotation": 15,
    "addKeyBook": 5,
    "addKeyPage": 10,
    "writeData": 15,
    "createAccount": 5,
    "updateKeyWeight": 0
  }
}
```

### Data-Heavy Load
```json
{
  "server": "http://localhost:26660",
  "targetTPS": 200,
  "runtimeSeconds": 300,
  "rampUpSeconds": 30,
  "operations": {
    "liteToLiteTransfer": 10,
    "liteToADITransfer": 5,
    "adiToAdiTransfer": 5,
    "keyRotation": 0,
    "addKeyBook": 0,
    "addKeyPage": 0,
    "writeData": 80,
    "createAccount": 0,
    "updateKeyWeight": 0
  }
}
```

## Troubleshooting

### "not enough accounts" errors
The load generator needs accounts to be created before it can run certain operations. Wait for account setup to complete before starting high-rate operations.

### Operations failing with insufficient credits
The load generator automatically funds accounts with 1000 credits during setup. For long-running tests, you may need to manually add more credits.

### Configuration not reloading
Check that the config file is valid JSON and that all operation percentages sum to 100.

## Integration with Testing Framework

This load generator is part of Epic #3838 (dagbft-integration testing framework). It is designed to work with:

- DevNet deployments
- CI/CD pipelines
- Performance benchmarking
- Protocol validation tests

See the main testing framework documentation for integration examples.
