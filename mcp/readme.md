# Accumulate MCP Server

A Model Context Protocol (MCP) server that provides full access to the Accumulate blockchain protocol. This server enables AI assistants like Claude to interact with the Accumulate network, query accounts, search the blockchain, and build/submit transactions.

## Features

- **38 Tools** for comprehensive Accumulate protocol access
- **3 Resources** for protocol documentation
- **Key Management** for transaction signing
- **Full Transaction Building** capabilities

## Installation

```bash
cd mcp
go build -o mcp-server ./cmd/mcp-server
```

## Usage

### Basic Usage

```bash
./mcp-server
```

### With Custom Endpoint

```bash
./mcp-server -endpoint https://mainnet.accumulatenetwork.io/v3
```

### With Logging

```bash
./mcp-server -log /tmp/mcp-server.log
```

## Claude Desktop Configuration

Add to your `~/.config/claude/mcp.json`:

```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/mcp-server",
      "args": ["-endpoint", "https://mainnet.accumulatenetwork.io/v3"]
    }
  }
}
```

For testnet/devnet:

```json
{
  "mcpServers": {
    "accumulate-devnet": {
      "command": "/path/to/mcp-server",
      "args": ["-endpoint", "http://127.0.0.1:26660/v3"]
    }
  }
}
```

## Available Tools

### Wallet Management (5 tools)
- `generate_key` - Generate a new ED25519 signing key pair
- `import_key` - Import an existing ED25519 private key
- `list_keys` - List all keys currently in memory
- `get_lite_address` - Compute lite token account address from a public key
- `get_lite_identity` - Compute lite identity URL from a public key

### Network Information (6 tools)
- `network_status` - Get current network status, oracle price, and globals
- `node_info` - Get information about the connected node
- `consensus_status` - Get consensus status of a partition
- `metrics` - Get node metrics
- `find_service` - Find services on the network
- `list_snapshots` - List available snapshots for a partition

### Query Tools (7 tools)
- `query_account` - Query an account by URL
- `query_transaction` - Query a transaction by hash/txid
- `query_directory` - List entries in an identity/directory
- `query_chain` - Query a specific chain on an account
- `query_data` - Query data entries from a data account
- `query_block` - Query block information
- `query_pending` - Query pending transactions

### Search Tools (5 tools)
- `search_public_key` - Search accounts by public key
- `search_public_key_hash` - Search accounts by public key hash
- `search_delegate` - Search delegation relationships
- `search_anchor` - Search for an anchor by hash
- `search_message_hash` - Search for a message by hash

### Transaction Submission (3 tools)
- `submit` - Submit a signed transaction envelope
- `validate` - Validate a transaction without submitting
- `faucet` - Request test tokens (testnet only)

### Transaction Building (12 tools)
- `send_tokens` - Send tokens between accounts
- `add_credits` - Convert ACME to credits
- `create_identity` - Create a new ADI
- `create_token_account` - Create a token account
- `create_data_account` - Create a data account
- `write_data` - Write data to a data account
- `create_key_book` - Create a key book
- `create_key_page` - Create a key page
- `update_key_page` - Add, remove, or update keys
- `create_token` - Create a token issuer
- `issue_tokens` - Issue tokens from a token issuer
- `burn_tokens` - Burn tokens

## Available Resources

- `accumulate://protocol/transaction-types` - List of supported transaction types
- `accumulate://protocol/account-types` - List of supported account types
- `accumulate://protocol/signature-types` - List of supported signature types

## Testing

### Unit Tests

```bash
go test ./...
```

### Integration Tests

Integration tests run against a live Accumulate network. By default they use a local devnet at `http://127.0.0.1:26660/v3`.

```bash
# Start a local devnet first
accumulated run devnet -w .devnet -b 1 -v 1 -f 0

# Run integration tests
go test -tags=integration ./...

# Use a different endpoint
ACCUMULATE_ENDPOINT=https://testnet.accumulatenetwork.io/v3 go test -tags=integration ./...
```

Integration tests cover:
- Network status and node info queries
- Account queries (ACME token, lite accounts)
- Faucet requests (devnet/testnet only)
- Key generation and management
- Transaction building (SendTokens, AddCredits, WriteData)
- Chain and block queries

## Protocol

This server implements the Model Context Protocol (MCP) version 2024-11-05. It communicates over stdio using JSON-RPC 2.0.

## License

MIT License - see LICENSE file in the Accumulate repository.
