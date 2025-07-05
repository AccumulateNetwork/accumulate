# Accumulate P2P Node Key Generation and Configuration

This document explains how to properly generate and configure P2P node keys for Accumulate nodes, which is essential for successful node startup and peer communication.

## Overview

Each Accumulate node requires a valid Ed25519 key pair for P2P communication. This key is used to:
- Identify the node on the network
- Secure peer-to-peer connections
- Sign messages between nodes

The key must be properly formatted and stored in the correct location for the node to start successfully.

## Key File Format

The P2P node key is stored in a JSON file named `node_key.json` in the `config/` directory of each node. The file has the following format:

```json
{
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "BASE64_OR_HEX_ENCODED_PRIVATE_KEY"
  }
}
```

Where:
- `type` must be exactly `"tendermint/PrivKeyEd25519"`
- `value` must be a valid Ed25519 private key (64 bytes / 128 hex characters)

## Key Generation Methods

### Method 1: Using Go Program (Recommended)

This method uses a simple Go program to generate a valid Ed25519 key pair and save it in the correct format:

1. Create a file named `generate_node_key.go`:

```go
package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/ed25519"
)

func main() {
	// Generate a new Ed25519 key pair
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("Error generating key: %v\n", err)
		os.Exit(1)
	}
	
	// Create the key file structure in the format Tendermint expects
	keyFile := map[string]interface{}{
		"priv_key": map[string]interface{}{
			"type":  "tendermint/PrivKeyEd25519",
			"value": fmt.Sprintf("%X", privateKey),
		},
	}
	
	// Marshal to JSON
	keyJSON, err := json.MarshalIndent(keyFile, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling key: %v\n", err)
		os.Exit(1)
	}
	
	// Write to file
	outputPath := "config/node_key.json"
	err = os.WriteFile(outputPath, keyJSON, 0600)
	if err != nil {
		fmt.Printf("Error writing key file: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Generated node key successfully at %s\n", outputPath)
}
```

2. Run the program:

```bash
cd /path/to/node/directory
go mod init simple_key_gen
go mod tidy
go run generate_node_key.go
```

### Method 2: Using the Accumulated CLI (Not Yet Implemented)

The `accumulated` CLI tool is planned to have a command for generating node keys, but this functionality is not yet fully implemented. When available, it would work like:

```bash
accumulated init node --home=/path/to/node/directory
```

## Configuration

Once the key is generated, ensure your `accumulate.toml` file has the correct P2P configuration:

```toml
# P2P configuration
[p2p]
# The node_key.json file should be in the config directory
# No explicit configuration needed if using the default location
```

If you need to specify a custom location for the key file:

```toml
# P2P configuration
[p2p]
key-file = "path/to/node_key.json"
```

## Troubleshooting

### Common Errors

1. **Invalid Private Key Length**:
   ```
   panic in ed25519 signing: ed25519: bad private key length: XX
   ```
   
   **Solution**: The private key in `node_key.json` has an incorrect length. Generate a new key using the method described above.

2. **Key File Not Found**:
   ```
   failed to load node key: open config/node_key.json: no such file or directory
   ```
   
   **Solution**: Generate the key file or check the path specified in the configuration.

3. **Invalid Key Format**:
   ```
   failed to parse node key: invalid character 'X' looking for beginning of value
   ```
   
   **Solution**: The key file is not valid JSON. Generate a new key file using the method described above.

## Security Considerations

- The `node_key.json` file should have restricted permissions (0600) to prevent unauthorized access
- Keep backups of your node keys in a secure location
- Do not share your private keys with anyone
- Consider using different keys for different environments (testnet, mainnet)

## References

- [Tendermint P2P Documentation](https://docs.tendermint.com/v0.34/tendermint-core/validators.html)
- [Ed25519 Specification](https://ed25519.cr.yp.to/)
