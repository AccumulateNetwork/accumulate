# Advanced Signature Types in Accumulate

## Metadata
- **Document Type**: Technical Implementation
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Transaction Processing, Security, Identity Management
- **Code References**: `protocol/signature.go`, `protocol/types_gen.go`
- **Tags**: signature_types, implementation, ethereum_compatibility, interoperability

## 1. Introduction

This document details the advanced signature types supported by Accumulate that were not fully covered in the main signature documentation. These signature types extend Accumulate's interoperability with other blockchain ecosystems and provide specialized functionality for specific use cases.

## 2. Extended Signature Type Overview

Accumulate supports a comprehensive range of signature types beyond the core ED25519 signatures. The complete list of signature types is defined in `protocol/enums_gen.go`:

```go
// SignatureTypeUnknown is used when the signature type is not known.
const SignatureTypeUnknown SignatureType = 0

// SignatureTypeLegacyED25519 represents a legacy ED25519 signature.
const SignatureTypeLegacyED25519 SignatureType = 1

// SignatureTypeED25519 represents an ED25519 signature.
const SignatureTypeED25519 SignatureType = 2

// SignatureTypeRCD1 represents an RCD1 signature.
const SignatureTypeRCD1 SignatureType = 3

// SignatureTypeReceipt represents a receipt signature.
const SignatureTypeReceipt SignatureType = 4

// SignatureTypePartition represents a partition signature.
const SignatureTypePartition SignatureType = 5

// SignatureTypeSet is used when forwarding multiple signatures.
const SignatureTypeSet SignatureType = 6

// SignatureTypeRemote is used when forwarding a signature from one partition to another.
const SignatureTypeRemote SignatureType = 7

// SignatureTypeBTC represents a BTC signature.
const SignatureTypeBTC SignatureType = 8

// SignatureTypeBTCLegacy represents a BTC signature with uncompressed public key.
const SignatureTypeBTCLegacy SignatureType = 9

// SignatureTypeETH represents an ETH signature.
const SignatureTypeETH SignatureType = 10

// SignatureTypeDelegated represents a delegated signature.
const SignatureTypeDelegated SignatureType = 11

// SignatureTypeInternal represents an internal signature.
const SignatureTypeInternal SignatureType = 12

// SignatureTypeAuthority represents an authority signature.
const SignatureTypeAuthority SignatureType = 13

// SignatureTypeRsaSha256 represents an RSA SHA256 signature.
const SignatureTypeRsaSha256 SignatureType = 14

// SignatureTypeEcdsaSha256 represents an ECDSA SHA256 signature.
const SignatureTypeEcdsaSha256 SignatureType = 15

// SignatureTypeTypedData represents an EIP-712 typed data signature.
const SignatureTypeTypedData SignatureType = 16
```

## 3. Ethereum-Compatible Signatures

### 3.1 ETH Signature

The `ETHSignature` type enables compatibility with Ethereum's secp256k1 ECDSA signatures, allowing Ethereum private keys to be used with Accumulate.

```go
type ETHSignature struct {
    fieldsSet       []bool
    PublicKey       []byte   `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
    Signature       []byte   `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
    Signer          *url.URL `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
    SignerVersion   uint64   `json:"signerVersion,omitempty" form:"signerVersion" query:"signerVersion" validate:"required"`
    Timestamp       uint64   `json:"timestamp,omitempty" form:"timestamp" query:"timestamp"`
    Vote            VoteType `json:"vote,omitempty" form:"vote" query:"vote"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    Memo            string   `json:"memo,omitempty" form:"memo" query:"memo"`
    Data            []byte   `json:"data,omitempty" form:"data" query:"data"`
    extraData       []byte
}
```

#### Implementation Details

The ETH signature verification process uses the Ethereum crypto library:

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

### 3.2 TypedData Signature (EIP-712)

The `TypedDataSignature` type implements Ethereum's EIP-712 standard for signing structured data, providing enhanced security and user experience when interacting with Ethereum-compatible wallets.

```go
type TypedDataSignature struct {
    fieldsSet       []bool
    PublicKey       []byte   `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
    Signature       []byte   `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
    Signer          *url.URL `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
    SignerVersion   uint64   `json:"signerVersion,omitempty" form:"signerVersion" query:"signerVersion" validate:"required"`
    Timestamp       uint64   `json:"timestamp,omitempty" form:"timestamp" query:"timestamp"`
    Vote            VoteType `json:"vote,omitempty" form:"vote" query:"vote"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    Memo            string   `json:"memo,omitempty" form:"memo" query:"memo"`
    Data            []byte   `json:"data,omitempty" form:"data" query:"data"`
    ChainID         *big.Int `json:"chainID,omitempty" form:"chainID" query:"chainID" validate:"required"`
    extraData       []byte
}
```

#### Implementation Details

The TypedData signature is created using the EIP-712 hashing algorithm:

```go
func SignEip712TypedData(sig *TypedDataSignature, privateKey []byte, outer Signature, txn *Transaction) error {
    if outer == nil {
        outer = sig
    }
    hash, err := EIP712Hash(txn, outer)
    if err != nil {
        return err
    }

    priv, err := eth.ToECDSA(privateKey)
    if err != nil {
        return err
    }

    sig.TransactionHash = txn.Hash()
    sig.Signature, err = eth.Sign(hash, priv)
    return err
}
```

Verification follows the EIP-712 standard:

```go
func (e *TypedDataSignature) Verify(sig Signature, msg Signable) bool {
    txn, ok := msg.(*Transaction)
    if !ok {
        // EIP-712 cannot be used to sign something that isn't a transaction
        return false
    }

    if sig == nil {
        sig = e
    }
    typedDataTxnHash, err := EIP712Hash(txn, sig)
    if err != nil {
        return false
    }

    s := e.Signature
    if len(s) == 65 {
        //extract RS of the RSV format
        s = s[:64]
    }
    return eth.VerifySignature(e.PublicKey, typedDataTxnHash, s)
}
```

## 4. Bitcoin-Compatible Signatures

### 4.1 BTC Signature

The `BTCSignature` type enables compatibility with Bitcoin's secp256k1 ECDSA signatures, allowing Bitcoin private keys to be used with Accumulate.

```go
type BTCSignature struct {
    fieldsSet       []bool
    PublicKey       []byte   `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
    Signature       []byte   `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
    Signer          *url.URL `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
    SignerVersion   uint64   `json:"signerVersion,omitempty" form:"signerVersion" query:"signerVersion" validate:"required"`
    Timestamp       uint64   `json:"timestamp,omitempty" form:"timestamp" query:"timestamp"`
    Vote            VoteType `json:"vote,omitempty" form:"vote" query:"vote"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    Memo            string   `json:"memo,omitempty" form:"memo" query:"memo"`
    Data            []byte   `json:"data,omitempty" form:"data" query:"data"`
    extraData       []byte
}
```

#### Implementation Details

BTC signatures use the btcec library for verification:

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

### 4.2 BTCLegacy Signature

The `BTCLegacySignature` type supports Bitcoin's legacy uncompressed public key format.

## 5. Standard Cryptographic Signatures

### 5.1 ECDSA SHA-256 Signature

The `EcdsaSha256Signature` type implements ECDSA signatures using the SHA-256 hash function, providing a standardized signature option compatible with many cryptographic libraries.

```go
type EcdsaSha256Signature struct {
    fieldsSet       []bool
    PublicKey       []byte   `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
    Signature       []byte   `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
    Signer          *url.URL `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
    SignerVersion   uint64   `json:"signerVersion,omitempty" form:"signerVersion" query:"signerVersion" validate:"required"`
    Timestamp       uint64   `json:"timestamp,omitempty" form:"timestamp" query:"timestamp"`
    Vote            VoteType `json:"vote,omitempty" form:"vote" query:"vote"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    Memo            string   `json:"memo,omitempty" form:"memo" query:"memo"`
    Data            []byte   `json:"data,omitempty" form:"data" query:"data"`
    extraData       []byte
}
```

#### Implementation Details

ECDSA SHA-256 signatures are verified using Go's standard crypto libraries:

```go
func (e *EcdsaSha256Signature) Verify(sig Signature, msg Signable) bool {
    //Convert public ANS.1 encoded key into and associated public key struct
    pubKey, err := x509.ParsePKIXPublicKey(e.PublicKey)
    if err != nil {
        return false
    }

    pub, ok := pubKey.(*ecdsa.PublicKey)
    if !ok {
        return false
    }

    return verifySig(e, sig, false, msg, func(msg []byte) bool {
        return ecdsa.VerifyASN1(pub, msg, e.Signature)
    })
}
```

### 5.2 RSA SHA-256 Signature

The `RsaSha256Signature` type implements RSA signatures using the SHA-256 hash function, providing compatibility with RSA-based systems and certificates.

```go
type RsaSha256Signature struct {
    fieldsSet       []bool
    PublicKey       []byte   `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
    Signature       []byte   `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
    Signer          *url.URL `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
    SignerVersion   uint64   `json:"signerVersion,omitempty" form:"signerVersion" query:"signerVersion" validate:"required"`
    Timestamp       uint64   `json:"timestamp,omitempty" form:"timestamp" query:"timestamp"`
    Vote            VoteType `json:"vote,omitempty" form:"vote" query:"vote"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    Memo            string   `json:"memo,omitempty" form:"memo" query:"memo"`
    Data            []byte   `json:"data,omitempty" form:"data" query:"data"`
    extraData       []byte
}
```

#### Implementation Details

RSA SHA-256 signatures are verified using Go's standard crypto libraries:

```go
func (r *RsaSha256Signature) Verify(sig Signature, msg Signable) bool {
    //Convert public ANS.1 encoded key into and associated public key struct
    pubKey, err := x509.ParsePKIXPublicKey(r.PublicKey)
    if err != nil {
        return false
    }

    pub, ok := pubKey.(*rsa.PublicKey)
    if !ok {
        return false
    }

    return verifySig(r, sig, false, msg, func(msg []byte) bool {
        err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, msg, r.Signature)
        return err == nil
    })
}
```

## 6. System Signatures

### 6.1 Authority Signature

The `AuthoritySignature` type is used for system-level authorization, particularly in the context of validator operations and network governance.

```go
type AuthoritySignature struct {
    fieldsSet       []bool
    Authority       *url.URL `json:"authority,omitempty" form:"authority" query:"authority" validate:"required"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    extraData       []byte
}
```

### 6.2 Internal Signature

The `InternalSignature` type is used for internal system operations and is not intended for user transactions.

```go
type InternalSignature struct {
    fieldsSet       []bool
    Cause           [32]byte `json:"cause,omitempty" form:"cause" query:"cause"`
    TransactionHash [32]byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash"`
    extraData       []byte
}
```

## 7. Signature Type Selection Guidelines

When implementing applications on Accumulate, consider these guidelines for selecting the appropriate signature type:

### 7.1 Interoperability Considerations

- **Ethereum Compatibility**: Use `ETHSignature` or `TypedDataSignature` for applications that need to interact with Ethereum wallets or smart contracts.
- **Bitcoin Compatibility**: Use `BTCSignature` or `BTCLegacySignature` for applications that need to interact with Bitcoin wallets.

### 7.2 Security Considerations

- **Modern Applications**: For new applications without specific compatibility requirements, `ED25519Signature` offers the best combination of security and performance.
- **Hardware Security Module (HSM) Support**: For applications using HSMs, `EcdsaSha256Signature` or `RsaSha256Signature` may be more widely supported by hardware.
- **Enterprise Integration**: For integration with existing PKI infrastructure, `RsaSha256Signature` provides compatibility with X.509 certificates.

### 7.3 Performance Considerations

- **High-Performance Applications**: `ED25519Signature` offers the best verification performance for most applications.
- **Compatibility-Focused Applications**: When interoperability is more important than performance, choose the signature type that matches your target ecosystem.

## 8. Public Key Hash Calculation

Different signature types use different methods to calculate the public key hash, which is critical for key identification and validation:

```go
func PublicKeyHash(key []byte, typ SignatureType) ([]byte, error) {
    switch typ {
    case SignatureTypeED25519,
        SignatureTypeLegacyED25519,
        SignatureTypeRsaSha256,
        SignatureTypeEcdsaSha256:
        return doSha256(key), nil

    case SignatureTypeRCD1:
        return GetRCDHashFromPublicKey(key, 1), nil

    case SignatureTypeBTC,
        SignatureTypeBTCLegacy:
        return BTCHash(key), nil

    case SignatureTypeETH,
        SignatureTypeTypedData:
        return ETHhash(key), nil

    case SignatureTypeReceipt,
        SignatureTypePartition,
        SignatureTypeSet,
        SignatureTypeRemote,
        SignatureTypeDelegated,
        SignatureTypeInternal:
        return nil, errors.BadRequest.WithFormat("%v is not a key type", typ)

    default:
        return nil, errors.NotAllowed.WithFormat("unknown key type %v", typ)
    }
}
```

## Related Documents

- [Keybooks and Signatures](../../02_architecture/02_signatures/01_keybooks_and_signatures.md)
- [Advanced Signature Validation](../../02_architecture/02_signatures/02_advanced_signature_validation.md)
- [Signature Validation Implementation](./01_signature_validation_implementation.md)
- [Signature Interoperability](./03_signature_interoperability.md)
- [Transaction Processing](../03_transaction_processing.md)
