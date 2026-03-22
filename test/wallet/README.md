# Test Wallet

A dedicated wallet for managing test accounts during load testing and integration testing.

## Overview

The test wallet provides a simple, file-based key management system for test accounts. It stores account keys persistently, allowing test accounts to be reused across multiple test runs without recreating them.

## Features

- Generate and store Ed25519 key pairs for test accounts
- Support for lite accounts, ADI token accounts, and ADI data accounts
- Persistent JSON storage
- Simple CLI for wallet management
- Easy integration with load generators and test scripts

## Installation

```bash
go install ./cmd/test-wallet
```

## Usage

### Create a new wallet

Create a wallet with default settings (1000 lite, 1000 ADI token, 1000 ADI data accounts):

```bash
test-wallet create
```

Create a custom wallet:

```bash
test-wallet create \
  -path ./my-wallet.json \
  -lite 100 \
  -adi-token 100 \
  -adi-data 100 \
  -funder acc://my-funder.acme/tokens
```

### View wallet information

```bash
test-wallet info ~/.accumulate/test-wallet.json
```

### Export keys

Export all keys as JSON:

```bash
test-wallet export-keys ~/.accumulate/test-wallet.json
```

Export just the funder private key (for use with load generator):

```bash
test-wallet export-keys ~/.accumulate/test-wallet.json | jq -r '.funder.privateKey'
```

### Get specific account

```bash
test-wallet get-account ~/.accumulate/test-wallet.json 0
```

## Account Types

The wallet generates three types of accounts:

1. **Lite accounts** - Simple token accounts identified by public key hash
   - Format: `acc://<hash>.acme/ACME`
   - No identity required

2. **ADI token accounts** - Token accounts under an ADI
   - Format: `acc://test-a00000001.acme/tokens`
   - Requires identity creation and key management

3. **ADI data accounts** - Data accounts under an ADI
   - Format: `acc://test-d00000001.acme/data`
   - Used for testing data storage features

## Integration with Load Generator

The test wallet can be used with the load generator by providing the funder key:

```bash
# Get funder key
FUNDER_KEY=$(test-wallet export-keys ~/.accumulate/test-wallet.json | jq -r '.funder.privateKey')

# Run load generator setup
load-generator -setup -funder-key "$FUNDER_KEY" -accounts 3000
```

## Storage Format

The wallet is stored as a JSON file with the following structure:

```json
{
  "funder": {
    "url": "acc://test-funder.acme/tokens",
    "publicKey": "...",
    "privateKey": "...",
    "type": "funder",
    "index": -1
  },
  "accounts": [
    {
      "url": "acc://...",
      "publicKey": "...",
      "privateKey": "...",
      "type": "lite|adi-token|adi-data",
      "index": 0
    }
  ]
}
```

Keys are hex-encoded Ed25519 keys.

## Security Note

This wallet is designed for testing purposes only. Do not use it to store keys for production accounts or accounts with real value.

The wallet file contains private keys in plaintext. Ensure proper file permissions (the tool sets 0600 automatically).
