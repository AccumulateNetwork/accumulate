# Signature Interoperability in Accumulate

## Metadata
- **Document Type**: Technical Implementation
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Transaction Processing, Security, Cross-Chain Integration
- **Code References**: `protocol/signature.go`, `protocol/ethereum.go`
- **Tags**: interoperability, cross_chain, ethereum, bitcoin, signature_conversion

## 1. Introduction

This document details how Accumulate's signature system interoperates with other blockchain ecosystems. The ability to use and verify signatures from different blockchain systems is a key feature of Accumulate, enabling seamless integration with existing blockchain infrastructure and wallets.

## 2. Interoperability Architecture

Accumulate achieves signature interoperability through a flexible signature interface system that can accommodate multiple signature types and verification methods. The core architecture consists of:

1. **Universal Signature Interface**: The `Signature` interface provides a common abstraction for all signature types
2. **Type-Specific Implementations**: Each blockchain's signature type has a dedicated implementation
3. **Conversion Utilities**: Functions for converting between different signature formats
4. **Verification Adapters**: Custom verification logic for each signature type

## 3. Ethereum Interoperability

### 3.1 Key and Address Formats

Accumulate supports Ethereum's secp256k1 ECDSA keys and addresses through the `ETHSignature` and `TypedDataSignature` types.

```go
// ETHhash returns the truncated hash (i.e. binary ethereum address)
func ETHhash(pubKey []byte) []byte {
    p, err := eth.UnmarshalPubkey(pubKey)
    if err != nil {
        p, err = eth.DecompressPubkey(pubKey)
        if err != nil {
            return nil
        }
    }
    a := eth.PubkeyToAddress(*p)
    return a.Bytes()
}

func ETHaddress(pubKey []byte) (string, error) {
    h := ETHhash(pubKey)
    if h == nil {
        return "", errors.BadRequest.With("invalid public key")
    }
    return eth.BytesToAddress(h).Hex(), nil
}
```

### 3.2 EIP-712 Typed Data Support

Accumulate implements Ethereum's EIP-712 standard for signing structured data, which improves the user experience when using Ethereum wallets with Accumulate:

```go
func EIP712Hash(txn *Transaction, sig Signature) ([]byte, error) {
    typedDataSig, ok := sig.(*TypedDataSignature)
    if !ok {
        return nil, errors.BadRequest.WithFormat("expected TypedDataSignature, got %T", sig)
    }

    if typedDataSig.ChainID == nil {
        return nil, errors.BadRequest.With("chain ID is required")
    }

    // Build the domain separator
    domain := apitypes.TypedDataDomain{
        Name:              "Accumulate",
        Version:           "1",
        ChainId:           typedDataSig.ChainID,
        VerifyingContract: "0x0000000000000000000000000000000000000000",
    }

    // Convert the transaction to a typed data message
    message, err := TransactionToTypedDataMessage(txn)
    if err != nil {
        return nil, err
    }

    // Generate the typed data
    typedData := apitypes.TypedData{
        Types:       EIP712Types,
        PrimaryType: "Transaction",
        Domain:      domain,
        Message:     message,
    }

    // Hash the typed data
    return typedData.HashStruct(typedData.PrimaryType, typedData.Message)
}
```

### 3.3 Ethereum Signature Verification

Ethereum signatures are verified using the Ethereum crypto library:

```go
// Verify returns true if this signature is a valid signature of the hash.
func (e *ETHSignature) Verify(sig Signature, msg Signable) bool {
    s := e.Signature
    if len(s) == 65 {
        // Extract RS from the RSV format
        s = s[:64]
    }
    
    return eth.VerifySignature(e.PublicKey, msg.Hash().Bytes(), s)
}
```

## 4. Bitcoin Interoperability

### 4.1 Key and Address Formats

Accumulate supports Bitcoin's key and address formats through the `BTCSignature` and `BTCLegacySignature` types:

```go
func BTCHash(pubKey []byte) []byte {
    hasher := ripemd160.New()
    hash := sha256.Sum256(pubKey[:])
    hasher.Write(hash[:])
    pubRip := hasher.Sum(nil)
    return pubRip[:]
}

func BTCaddress(pubKey []byte) string {
    pubRip := BTCHash(pubKey)
    versionedPayload := append([]byte{0x00}, pubRip...)
    newhash := sha256.Sum256(versionedPayload)
    newhash = sha256.Sum256(newhash[:])
    checkSum := newhash[:4]
    fullpayload := append(versionedPayload, checkSum...)
    address := base58.Encode(fullpayload)
    return address
}
```

### 4.2 Bitcoin Signature Verification

Bitcoin signatures are verified using the btcec library:

```go
func (b *BTCSignature) Verify(sig Signature, msg Signable) bool {
    pubKey, err := btc.ParsePubKey(b.PublicKey, btc.S256())
    if err != nil {
        return false
    }

    return verifySig(b, sig, false, msg, func(msg []byte) bool {
        btcSig, err := btc.ParseSignature(b.Signature, btc.S256())
        if err != nil {
            return false
        }
        return btcSig.Verify(msg, pubKey)
    })
}
```

## 5. Cross-Chain Signature Processing

### 5.1 Remote Signatures

The `RemoteSignature` type enables signatures to be forwarded across different partitions in the Accumulate network:

```go
type RemoteSignature struct {
    fieldsSet       []bool
    Signature       []byte   `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
    Signer          *url.URL `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
    SourceNetwork   string   `json:"sourceNetwork,omitempty" form:"sourceNetwork" query:"sourceNetwork" validate:"required"`
    DestinationNetwork string `json:"destinationNetwork,omitempty" form:"destinationNetwork" query:"destinationNetwork" validate:"required"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    extraData       []byte
}
```

### 5.2 Partition Signatures

The `PartitionSignature` type is used for cross-partition transactions within the Accumulate network:

```go
type PartitionSignature struct {
    fieldsSet           []bool
    SourceNetwork       string   `json:"sourceNetwork,omitempty" form:"sourceNetwork" query:"sourceNetwork" validate:"required"`
    DestinationNetwork  string   `json:"destinationNetwork,omitempty" form:"destinationNetwork" query:"destinationNetwork" validate:"required"`
    SequenceNumber      uint64   `json:"sequenceNumber,omitempty" form:"sequenceNumber" query:"sequenceNumber" validate:"required"`
    TransactionHash     [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    SourceIndex         uint64   `json:"sourceIndex,omitempty" form:"sourceIndex" query:"sourceIndex" validate:"required"`
    Signature           []byte   `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
    extraData           []byte
}
```

## 6. Integration Patterns

### 6.1 Wallet Integration

Accumulate can integrate with various blockchain wallets through its signature interoperability features:

#### Ethereum Wallet Integration

```javascript
// Example: Signing an Accumulate transaction with MetaMask
async function signWithMetaMask(transaction) {
    const provider = new ethers.providers.Web3Provider(window.ethereum);
    const signer = provider.getSigner();
    const address = await signer.getAddress();
    
    // Create EIP-712 typed data
    const typedData = createTypedDataFromTransaction(transaction);
    
    // Sign with MetaMask
    const signature = await signer._signTypedData(
        typedData.domain,
        typedData.types,
        typedData.message
    );
    
    // Create TypedDataSignature for Accumulate
    return {
        type: "typedData",
        publicKey: await getPublicKeyFromMetaMask(),
        signature: signature,
        signer: transaction.principal,
        signerVersion: transaction.signerVersion,
        timestamp: Date.now(),
        chainID: typedData.domain.chainId
    };
}
```

#### Bitcoin Wallet Integration

```javascript
// Example: Signing an Accumulate transaction with a Bitcoin wallet
async function signWithBitcoinWallet(transaction, bitcoinWallet) {
    const messageHash = hashTransaction(transaction);
    
    // Sign with Bitcoin wallet
    const signature = await bitcoinWallet.sign(messageHash);
    const publicKey = await bitcoinWallet.getPublicKey();
    
    // Create BTCSignature for Accumulate
    return {
        type: "btc",
        publicKey: publicKey,
        signature: signature,
        signer: transaction.principal,
        signerVersion: transaction.signerVersion,
        timestamp: Date.now()
    };
}
```

### 6.2 Cross-Chain Transaction Patterns

#### Ethereum to Accumulate

```javascript
// Example: Creating an Accumulate transaction from an Ethereum transaction
async function createAccumulateTransactionFromEthereum(ethTx, accPrincipal) {
    // Extract relevant data from Ethereum transaction
    const { to, value, data } = ethTx;
    
    // Create Accumulate transaction
    const accTx = {
        type: "sendTokens",
        principal: accPrincipal,
        recipient: convertEthAddressToAccURL(to),
        amount: convertWeiToAcme(value),
        metadata: data ? { ethereumData: data } : undefined
    };
    
    // Sign with Ethereum wallet
    const signature = await signWithEthereumWallet(accTx);
    
    return {
        transaction: accTx,
        signatures: [signature]
    };
}
```

## 7. Implementation Considerations

### 7.1 Security Considerations

When implementing cross-chain signature interoperability, consider these security aspects:

1. **Key Derivation**: Different blockchains use different key derivation paths. Document the paths used for each blockchain to ensure consistent key generation.

2. **Signature Malleability**: Some blockchain signatures (particularly Bitcoin's ECDSA) are malleable. Implement proper validation to prevent signature malleability attacks.

3. **Chain IDs**: For EIP-712 signatures, always include the correct chain ID to prevent replay attacks across different networks.

4. **Address Format Validation**: Implement strict validation for address formats when converting between different blockchain address schemes.

### 7.2 Performance Considerations

Different signature verification algorithms have different performance characteristics:

| Signature Type | Verification Speed | Key Size | Security Level |
|----------------|-------------------|----------|---------------|
| ED25519        | Very Fast         | 32 bytes | High          |
| ECDSA (secp256k1) | Moderate      | 33-65 bytes | High        |
| RSA-SHA256     | Slow             | 256+ bytes | High (with sufficient key size) |

Choose the appropriate signature type based on your application's performance requirements.

### 7.3 Compatibility Considerations

When implementing signature interoperability, consider these compatibility issues:

1. **Key Format Compatibility**: Ensure proper handling of compressed vs. uncompressed public keys, particularly for secp256k1 keys.

2. **Signature Format Compatibility**: Handle different signature formats correctly (e.g., Bitcoin's DER encoding, Ethereum's RSV format).

3. **Hash Function Compatibility**: Use the correct hash function for each signature type (e.g., keccak256 for Ethereum, double SHA-256 for Bitcoin).

## 8. Future Interoperability Directions

Accumulate plans to extend signature interoperability to additional blockchain ecosystems:

1. **Cosmos Ecosystem**: Support for Cosmos signatures to enable interoperability with the Cosmos ecosystem.

2. **Polkadot Ecosystem**: Support for Polkadot signatures to enable interoperability with the Polkadot ecosystem.

3. **Post-Quantum Signatures**: Research and implementation of post-quantum signature schemes for future-proofing the protocol.

## Related Documents

- [Keybooks and Signatures](../../02_architecture/02_signatures/01_keybooks_and_signatures.md)
- [Advanced Signature Validation](../../02_architecture/02_signatures/02_advanced_signature_validation.md)
- [Advanced Signature Types](./02_advanced_signature_types.md)
- [Signature Validation Implementation](./01_signature_validation_implementation.md)
- [Signature System Comparison](../../02_architecture/02_signatures/05_signature_system_comparison.md)
