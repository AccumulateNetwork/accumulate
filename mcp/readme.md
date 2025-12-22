# mcp-accumulate

MCP (Model Context Protocol) server for the Accumulate blockchain protocol. This server enables AI assistants like Claude to interact with Accumulate accounts, manage wallets, query transactions, and perform blockchain operations.

## Features

### 🆕 Wallet Integration (v0.2.0)
- **Stateful Configuration**: Persistent wallet and network settings via environment variables
- **Wallet Management**: Initialize wallets, generate keys, manage vaults
- **MCP Resources**: Query wallet state, configuration, and keys
- **Network Selection**: Seamless switching between mainnet, testnet, and devnet
- **Secure Key Storage**: Keys stored in wallet, integrated with `ccli`

### Core Capabilities (40 MCP Tools)
- **Wallet Tools (7)**: Init wallet, manage vaults, generate/list keys, set network
- **Account Queries (11)**: Query accounts, chains, data, directories, and pending transactions
- **Transaction Operations (22)**: Create ADIs, send tokens, manage data/token accounts, key management
- **Block Queries**: Query major and minor blocks by partition
- **Network Status**: Node info, network globals, consensus status, and metrics
- **Advanced Search**: Search by public key, key hash, or anchor
- **Testnet Support**: Faucet integration for testnet tokens
- Written in Go for native performance (~9MB binary)
- **Full SDK Integration**: Uses official Accumulate v1.4.2 SDK with typed queries and protocol types

## Installation

```bash
go build -o mcp-accumulate
```

## Usage with Claude Desktop

Add this configuration to your Claude Desktop config file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
**Linux**: `~/.config/Claude/claude_desktop_config.json`

### Basic Configuration (Mainnet)
```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/mcp-accumulate"
    }
  }
}
```

### With Wallet Configuration (Recommended)
```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/mcp-accumulate",
      "env": {
        "ACCUMULATE_NETWORK": "devnet",
        "ACCUMULATE_WALLET_DIR": "/home/user/.accumulate/devnet-wallet"
      }
    }
  }
}
```

### Environment Variables
- `ACCUMULATE_NETWORK`: Network to use (`mainnet`, `testnet`, `devnet`)
- `ACCUMULATE_WALLET_DIR`: Path to wallet directory (default: `~/.accumulate/wallet`)
- `ACCUMULATE_SERVER`: Custom RPC server URL (overrides network setting)

## MCP Resources

The server exposes 3 MCP resources for querying wallet state:

### `wallet://config`
Current wallet and network configuration.

### `wallet://state`
Runtime wallet state including vault lock status and active vault.

### `wallet://keys`
List of keys in the wallet (requires unlocked vault).

## Available Tools

### Wallet Management Tools

#### `wallet_init`
Initialize a new Accumulate wallet.

#### `wallet_vault_open`
Open and unlock a vault in the wallet.

#### `wallet_vault_lock`
Lock the currently opened vault.

#### `wallet_generate_key`
Generate a new key pair in the wallet (requires unlocked vault).

#### `wallet_list_keys`
List all keys in the wallet (requires unlocked vault).

#### `wallet_set_network`
Set the network for wallet operations (mainnet, testnet, devnet, or custom URL).

#### `wallet_get_status`
Get current wallet and network status.

### Network Query Tools

#### accumulate_query_account

Query an Accumulate account by URL to get account details, balance, and state.

**Parameters:**
- `url` (required): The Accumulate account URL (e.g., `acc://example.acme/tokens`)
- `network` (optional): Network to query (`mainnet`, `testnet`, or custom RPC endpoint)

**Example:**
```
Query account acc://paul.acme/tokens on mainnet
```

### accumulate_query_tx

Query a transaction by hash to get transaction details and status.

**Parameters:**
- `txid` (required): The transaction ID/hash to query
- `network` (optional): Network to query (`mainnet`, `testnet`, or custom RPC endpoint)

**Example:**
```
Query transaction abc123... on mainnet
```

### accumulate_create_lite_account

Create a new Accumulate lite account URL from a public key. Lite accounts are deterministically derived from public keys.

**Parameters:**
- `public_key` (required): Public key in hex format

**Example:**
```
Create lite account from public key 0x1234abcd...
```

### accumulate_send_tokens

Prepare a token transfer transaction to send ACME tokens.

**Parameters:**
- `from` (required): Source account URL
- `to` (required): Destination account URL
- `amount` (required): Amount of ACME tokens to send
- `private_key` (required): Private key of the source account
- `network` (optional): Network to use (`mainnet`, `testnet`, or custom RPC endpoint)

**Example:**
```
Send 10 ACME from acc://alice.acme/tokens to acc://bob.acme/tokens
```

## Development

### Build

```bash
go build -o mcp-accumulate
```

### Run directly

```bash
go run main.go
```

### Test

```bash
go test ./...
```

## Architecture

This MCP server consists of:

- `main.go`: Entry point that sets up stdio-based JSON-RPC communication
- `server/server.go`: MCP server implementation with protocol handlers
- `server/tools.go`: Tool implementations for Accumulate operations
- `client/client.go`: Accumulate API client with transaction signing support

The server uses the official Accumulate SDK (v1.4.2) with full type safety and proper protocol integration. Key features:

- **SDK-Based Client**: Uses `pkg/api/v3/jsonrpc` for all API calls
- **Typed Queries**: All queries use proper SDK structs (`api.DefaultQuery`, `api.ChainQuery`, etc.)
- **Protocol Types**: Transactions built with `protocol.Transaction` and proper signing
- **Correct URL Handling**: Uses `pkg/url` for Accumulate URL parsing
- **ED25519 Signing**: Proper transaction signing with `protocol.ED25519Signature`
- **Lite Accounts**: Correct derivation using `protocol.LiteAuthorityForKey()`

## Network Endpoints

- **Mainnet**: `https://mainnet.accumulatenetwork.io/v3`
- **Testnet**: `https://testnet.accumulatenetwork.io/v3`
- **Custom**: Provide any RPC endpoint URL

## SDK Integration

**Recently Completed**: Full rewrite to use official Accumulate SDK (v1.4.2)

All client code has been rewritten to properly integrate with the Accumulate SDK:
- Replaced custom JSON-RPC with `jsonrpc.Client` from SDK
- Using typed query/record structs instead of `map[string]interface{}`
- Proper transaction signing with protocol types
- Correct lite account derivation using SDK functions

See `SDK_REWRITE_SUMMARY.md` for complete details of what changed.

## Features

- **Full Transaction Signing**: ED25519 signature support using protocol types
- **SDK-Based Queries**: All queries use official SDK typed structs
- **Lite Account Support**: Generate lite account URLs using `protocol.LiteAuthorityForKey()`
- **Network Flexibility**: Support for mainnet, testnet, and custom endpoints

## License

MIT
