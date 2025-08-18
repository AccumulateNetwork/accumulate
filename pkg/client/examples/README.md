# Accumulate Go SDK Examples

This directory contains example applications demonstrating how to use the Accumulate Go SDK for various use cases. Each example includes both a command-line interface and an optional web UI for interactive exploration.

## 🚀 Quick Start - Web UIs

Launch all web interfaces at once:

```bash
./launch_all.sh
```

Or open `index.html` in your browser for a dashboard linking to all applications.

Individual web UIs:
- **Network Monitor**: http://localhost:8080 (`go run ./network_monitor -web`)
- **Balance Checker**: http://localhost:8081 (`go run ./balance_checker -web`)

## Example Applications

### 1. Account Explorer (`account_explorer`)

A comprehensive tool to explore Accumulate accounts and their properties, including sub-accounts and hierarchical structures.

**Features:**
- Explore any account by URL
- Display account properties (balances, tokens, authorities)
- Recursively explore sub-accounts with configurable depth
- JSON or human-readable output
- Support for all account types (ADI, token, data, key)

**Usage:**
```bash
# Explore ACME token on mainnet
go run account_explorer/main.go -url acc://ACME

# Explore with sub-accounts (depth=2)
go run account_explorer/main.go -url acc://dn.acme -depth 2

# Output as JSON
go run account_explorer/main.go -url acc://ACME -json

# Use testnet
go run account_explorer/main.go -network testnet -url acc://ACME
```

**Example Output:**
```
📁 acc://ACME (tokenIssuer)
  • symbol: ACME
  • precision: 8
  • issued: 30886056506745980
  • supplyLimit: 50000000000000000
  📂 Sub-accounts (2):
    📁 acc://ACME/treasury (tokenAccount)
      • tokenUrl: acc://ACME
      • balance: 1000000000000
```

### 2. Network Monitor (`network_monitor`)

Real-time dashboard displaying network status, metrics, and node information with automatic refresh. **Includes interactive web UI!**

**Features:**
- Live network status monitoring
- Node information and version
- Partition metrics and TPS
- Network parameters and limits
- Auto-refresh with configurable interval
- Clean terminal UI with ANSI colors
- **Web UI with real-time updates**

**Usage:**
```bash
# Monitor mainnet with 5-second refresh
go run network_monitor/main.go

# Monitor testnet with 10-second refresh
go run network_monitor/main.go -network testnet -interval 10s

# Run once (no refresh)
go run network_monitor/main.go -once

# Monitor local network
ACCUMULATE_ENDPOINT=http://localhost:8080/v3 go run network_monitor/main.go -network local

# Launch web UI (recommended!)
go run network_monitor/main.go -web -port 8080
```

**Example Output:**
```
================================================================================
                     🌐 ACCUMULATE NETWORK MONITOR
================================================================================
Last Update: 2025-08-11 15:30:45

📡 NODE INFORMATION
--------------------------------------------------------------------------------
  Network:    MainNet
  Peer ID:    12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn
  Version:    v1.4.1-5-g774daaf0e
  Services:   node(1), consensus(2), network(2), metrics(2), query(2)

🌍 NETWORK STATUS
--------------------------------------------------------------------------------
  Network Name:       MainNet
  Partitions:         2
    • Cyclops (blockValidator)
    • Directory (directory)
  Validators:         8 total
  Directory Height:   2447218
  Major Block Height: 58

📊 PARTITION METRICS
--------------------------------------------------------------------------------
  🟡 Cyclops: 0.00 TPS
  🟡 Directory: 0.00 TPS
```

### 3. Data Reader (`data_reader`)

Tool to read and display data from Accumulate data accounts with support for different formats.

**Features:**
- Read data from any data account
- Multiple output formats (hex, text, json, auto-detect)
- Display latest entry or specific index
- Automatic format detection
- Support for both full and lite data accounts

**Usage:**
```bash
# Read data from network account
go run data_reader/main.go -url acc://dn.acme/network

# Display as hex
go run data_reader/main.go -url acc://dn.acme/network -format hex

# Read specific entry (note: API currently returns full account)
go run data_reader/main.go -url acc://mydata.acme -index 0

# Show latest entry only
go run data_reader/main.go -url acc://mydata.acme -latest

# Auto-detect format (JSON, text, or hex)
go run data_reader/main.go -url acc://mydata.acme -format auto
```

**Example Output:**
```
📄 Data Account: acc://dn.acme/network
Type: Full Data Account
Entry Type: doubleHash
Last Update: 2025-08-11T23:59:13Z
--------------------------------------------------------------------------------
Entry #0
  Type: doubleHash
  01074d61696e4e6574030b01074379636c6f70730202...
```

### 4. Balance Checker (`balance_checker`)

Simple utility to check ACME token balances for multiple accounts with formatted table output. **Includes interactive web UI!**

**Features:**
- Check balances for multiple accounts
- Support for all token account types
- Display credits balance
- Watch mode for continuous monitoring
- CSV export option
- Token issuer statistics
- **Web UI with quick account presets**

**Usage:**
```bash
# Check ACME balance
go run balance_checker/main.go

# Check multiple accounts
go run balance_checker/main.go -accounts "acc://ACME,acc://my-account.acme"

# Watch mode (refresh every 10 seconds)
go run balance_checker/main.go -watch -accounts "acc://ACME,acc://my-token.acme"

# Export as CSV
go run balance_checker/main.go -csv -accounts "acc://ACME" > balances.csv

# Use testnet
go run balance_checker/main.go -network testnet

# Launch web UI (recommended!)
go run balance_checker/main.go -web -port 8081
```

**Example Output:**
```
💰 ACCUMULATE ACCOUNT BALANCES
Time: 2025-08-11 15:30:45

ACCOUNT                  TYPE           SYMBOL  BALANCE        CREDITS  STATUS
-------                  ----           ------  -------        -------  ------
acc://ACME               tokenIssuer    ACME    308860.56507   -        ✅
acc://my-account.acme    tokenAccount   ACME    1000.00000     100      ✅

📊 TOKEN ISSUER SUMMARY
  acc://ACME (ACME):
    • Issued: 308860.56507 ACME
    • Supply Limit: 500000.00000 ACME
    • Utilization: 61.77%
```

## Building the Examples

Each example can be built as a standalone binary:

```bash
# Build all examples
cd examples
go build ./account_explorer
go build ./network_monitor
go build ./data_reader
go build ./balance_checker

# Or build individually
go build -o explorer ./account_explorer/main.go
go build -o monitor ./network_monitor/main.go
go build -o reader ./data_reader/main.go
go build -o balance ./balance_checker/main.go
```

## Common Flags

All examples support these common flags:

- `-network` - Network to connect to: `mainnet` (default), `testnet`, or `local`
- `-h` or `-help` - Display help message

For local networks, set the `ACCUMULATE_ENDPOINT` environment variable:
```bash
export ACCUMULATE_ENDPOINT=http://localhost:8080/v3
```

## Use Cases

### For Developers
- **Account Explorer**: Debug account structures and understand ADI hierarchies
- **Network Monitor**: Monitor network health during development and testing

### For Operations
- **Network Monitor**: Track network performance and validator status
- **Balance Checker**: Monitor treasury and operational accounts

### For Users
- **Balance Checker**: Track token balances across multiple accounts
- **Account Explorer**: Explore account details and sub-accounts

## Requirements

- Go 1.21 or later
- Network connectivity to Accumulate nodes
- For mainnet: Access to https://mainnet.accumulatenetwork.io
- For testnet: Access to https://kermit.accumulatenetwork.io
- For local: Running Accumulate node with API endpoint

## Contributing

Feel free to extend these examples or create new ones! Some ideas:

- Transaction builder and submitter
- Event stream monitor
- Data account manager
- Validator performance tracker
- Block explorer
- Token transfer tool

## License

MIT License - See LICENSE file in the repository root.