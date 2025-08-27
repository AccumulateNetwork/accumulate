# TypedDataSignature Initiator Hash Solution

## Problem
The client is getting "has not been initiated" errors when submitting transactions with TypedDataSignature (EIP-712), even though the signature is cryptographically valid.

## Root Cause
The client calculated an incorrect initiator hash (`6478a772ddc6ede9e8654d23cc02ccf4c6669d2c86538991ef851ff70cb3ea83`) that doesn't match what Accumulate expects (`e54ca04f6a003e2e08aeeb2f52d813ce2f3e08007135cc88952cb404ae5aeec3`).

## Solution

### Required Fields for Initiator Hash
The initiator hash for TypedDataSignature must include ALL of these fields:
- `PublicKey` - The public key bytes
- `Signer` - The signer URL (e.g., `acc://0test1test01.acme/book/1`)
- `SignerVersion` - The version of the key page (e.g., 43)
- `Timestamp` - Unix timestamp in microseconds
- `ChainID` - The chain ID (281 for MainNet)
- `Vote` - Vote type (defaults to `accept`)
- `Memo` - Memo string (can be empty)
- `Data` - Additional data (can be nil)

Fields NOT included:
- `Signature` - The actual signature bytes
- `TransactionHash` - The transaction hash

### Correct Implementation

```go
// 1. Create TypedDataSignature with all metadata fields
sig := &TypedDataSignature{
    ChainID:       big.NewInt(281),  // MainNet
    PublicKey:     publicKeyBytes,
    Signer:        url.MustParse("acc://0test1test01.acme/book/1"),
    SignerVersion: 43,
    Timestamp:     1756135756786,
    // Vote, Memo, Data can be left as defaults
}

// 2. Calculate the correct initiator hash
initiatorHash := sig.Metadata().Hash()

// 3. Build transaction with the correct initiator
txn, _ := build.Transaction().For("acc://0test1test01.acme/tokens").
    SendTokens(10000000, 0).To("acc://0test1test01.acme/staking").
    Done()
copy(txn.Header.Initiator[:], initiatorHash)

// 4. Calculate EIP-712 hash for signing
eip712Hash, _ := protocol.EIP712Hash(txn, sig)

// 5. Sign the EIP-712 hash with private key
// (Client signs eip712Hash using their Ethereum-compatible signing method)

// 6. Set the signature on the TypedDataSignature
sig.Signature = signatureBytes

// 7. Submit transaction with signature
```

### Key Points
1. The initiator hash MUST be calculated using `sig.Metadata().Hash()`
2. This is a SHA256 hash of the binary encoding of the metadata fields
3. The deprecated `Initiator().MerkleHash()` method produces a different hash and should NOT be used
4. The ChainID field is REQUIRED and must be included (281 for MainNet)

### Test Results
- Client's (incorrect) initiator: `6478a772ddc6ede9e8654d23cc02ccf4c6669d2c86538991ef851ff70cb3ea83`
- Correct initiator using `Metadata().Hash()`: `e54ca04f6a003e2e08aeeb2f52d813ce2f3e08007135cc88952cb404ae5aeec3`
- EIP-712 hash that should be signed: `4255234879c44fe2ad471c18d4442b904d6516ed800dc787cca422cc399473a9`

The mismatch between these initiator hashes is what causes the "has not been initiated" error.