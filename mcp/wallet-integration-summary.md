# MCP Accumulate - Wallet Integration Summary

## Overview

The MCP server has been enhanced with wallet integration and stateful configuration. Users now get a consistent wallet and network configuration across all MCP operations.

## What Was Added

### 1. Configuration System (`server/config.go`)
- **Environment Variables**:
  - `ACCUMULATE_WALLET_DIR` - Wallet directory path (default: `~/.accumulate/wallet`)
  - `ACCUMULATE_NETWORK` - Network name: `mainnet`, `testnet`, `devnet`
  - `ACCUMULATE_SERVER` - Custom RPC server URL (overrides network)

- **Network Auto-Configuration**:
  - `mainnet` → `https://mainnet.accumulatenetwork.io/v3`
  - `testnet` → `https://testnet.accumulatenetwork.io/v3`
  - `devnet` → `http://127.0.0.1:26660/v2`

### 2. State Management (`server/state.go`)
- **Runtime State**: Thread-safe state management for:
  - Current wallet directory
  - Active network and server URL
  - Vault lock status
  - Active vault name and token

### 3. Wallet Client (`wallet/client.go`)
- **CLI Integration**: Wraps `ccli` binary for wallet operations
- **Auto-Discovery**: Finds `ccli` in common locations or PATH
- **Key Operations**:
  - Initialize wallet
  - Generate keys
  - List keys
  - Get key information

### 4. New MCP Tools (7 Added)

#### `wallet_init`
Initialize a new Accumulate wallet.
```json
{
  "password": "optional",
  "no_password": false
}
```

#### `wallet_vault_open`
Open and unlock a vault.
```json
{
  "vault": "default",
  "password": "required"
}
```

#### `wallet_vault_lock`
Lock the currently opened vault.

#### `wallet_generate_key`
Generate a new key pair (requires unlocked vault).
```json
{
  "key_name": "my-key"
}
```

#### `wallet_list_keys`
List all keys in wallet (requires unlocked vault).

#### `wallet_set_network`
Change the network configuration.
```json
{
  "network": "mainnet|testnet|devnet|<custom-url>"
}
```

#### `wallet_get_status`
Get current wallet and network status.

### 5. MCP Resources (3 Added)

#### `wallet://config`
Current wallet and network configuration.
```json
{
  "walletDir": "~/.accumulate/devnet-wallet",
  "network": "devnet",
  "server": "http://127.0.0.1:26660/v2"
}
```

#### `wallet://state`
Runtime wallet state including vault status.
```json
{
  "walletDir": "~/.accumulate/devnet-wallet",
  "network": "devnet",
  "server": "http://127.0.0.1:26660/v2",
  "vaultLocked": true,
  "activeVault": ""
}
```

#### `wallet://keys`
List of keys (requires unlocked vault).
```json
{
  "keys": [
    {
      "name": "my-key",
      "publicKey": "0x...",
      "liteAccount": "acc://..."
    }
  ],
  "count": 1
}
```

## Usage with Claude Desktop

### Configuration

Add to `~/.config/Claude/claude_desktop_config.json` (Linux) or equivalent:

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

### Example Workflows

#### Setup Wallet for Devnet
1. "Set network to devnet"
2. "Initialize a wallet with no password"
3. "Generate a key called 'my-key'"
4. "List my wallet keys"
5. "Get my wallet status"

#### Query Accounts
1. "Set network to mainnet"
2. "Query account acc://paul.acme/tokens"

#### Work with Testnet
1. "Set network to testnet"
2. "Open vault with password 'secret'"
3. "Generate a key called 'test-key'"
4. "Request faucet funds for my lite account"

## Architecture Changes

### Before
```
MCP Server
├── Network Query Tools (33 tools)
└── Direct SDK calls with private keys
```

### After
```
MCP Server
├── Configuration (env-based)
├── Runtime State (thread-safe)
├── Wallet Client (CLI wrapper)
├── Network Query Tools (33 tools)
├── Wallet Management Tools (7 new tools)
└── MCP Resources (3 resources)
```

## Total Tool Count

- **Before**: 33 tools
- **After**: 40 tools (33 + 7 wallet tools)

## Resources

- **Before**: 0 resources
- **After**: 3 resources (`wallet://config`, `wallet://state`, `wallet://keys`)

## Key Benefits

1. **Stateful Configuration**: Network and wallet settings persist across operations
2. **Secure Key Storage**: Keys stored in wallet, not passed as parameters
3. **Environment-Based**: Easy configuration via environment variables
4. **Wallet Integration**: Leverages existing `ccli` infrastructure
5. **Resource Discovery**: Claude can inspect wallet state via MCP resources

## Testing

### Test Configuration
```bash
export ACCUMULATE_NETWORK=devnet
export ACCUMULATE_WALLET_DIR=$HOME/.accumulate/devnet-wallet
./mcp-accumulate
```

### Test Resources
```bash
echo '{"jsonrpc":"2.0","method":"resources/list","id":1}' | ./mcp-accumulate
echo '{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"wallet://config"}}' | ./mcp-accumulate
```

### Test Tools
```bash
echo '{"jsonrpc":"2.0","method":"tools/list","id":3}' | ./mcp-accumulate | jq '.result.tools | length'
# Should return 40
```

## Implementation Notes

### Wallet Client Approach
- Uses exec to call `ccli` binary (not direct SDK imports)
- Avoids internal package dependencies
- Parses CLI output for key information
- Compatible with existing wallet infrastructure

### Thread Safety
- State management uses `sync.RWMutex`
- Safe for concurrent MCP requests
- Atomic vault lock/unlock operations

### Future Enhancements
1. **Transaction Signing**: Use wallet keys to sign transactions
2. **Password Management**: Secure password handling for vault operations
3. **Multi-Vault Support**: Switch between multiple vaults
4. **Key Import/Export**: Import existing keys into wallet
5. **Backup/Restore**: Wallet backup and restore via MCP

## Files Modified/Created

### Created
- `server/config.go` - Configuration system
- `server/state.go` - Runtime state management
- `server/resources.go` - MCP resources handlers
- `server/tools_wallet.go` - Wallet tool implementations
- `wallet/client.go` - Wallet CLI wrapper

### Modified
- `server/server.go` - Added state, wallet client, resources support
- `server/tool_definitions.go` - Added 7 wallet tool definitions

## Next Steps

1. ✅ Configuration system
2. ✅ State management
3. ✅ Wallet client integration
4. ✅ Basic wallet tools
5. ✅ MCP resources
6. ⏳ Test with actual devnet
7. ⏳ Update transaction tools to use wallet keys
8. ⏳ Add confirmation prompts for sensitive operations
9. ⏳ Documentation update

## Version

- **MCP Server Version**: 0.2.0
- **Tool Count**: 40
- **Resource Count**: 3
- **Status**: Wallet integration complete, testing in progress
