# ADIs and Special Accounts in Accumulate

This document provides a comprehensive overview of Accumulate Digital Identifiers (ADIs) and accounts that are handled in exceptional ways within the Accumulate protocol.

## 1. Introduction to ADIs

Accumulate Digital Identifiers (ADIs) are hierarchical identities that serve as the foundation of the Accumulate protocol. Unlike traditional blockchain addresses that are derived from public keys, ADIs are human-readable identifiers that can own and manage various types of accounts.

### 1.1 ADI Structure

An ADI is defined in the protocol as:

```go
type ADI struct {
    Url        *url.URL    // The URL of the ADI
    AccountAuth            // Authentication information
    extraData  []byte      // Additional data
}
```

ADIs are represented by URLs with the following characteristics:
- Format: `acc://<authority>[/<path>]`
- Authority: The domain name of the ADI (e.g., `example.acme`)
- TLD: All ADIs use the `.acme` top-level domain

## 2. Reserved ADIs

The Accumulate protocol designates certain ADIs as reserved for system use. These ADIs have special properties and handling within the protocol.

### 2.1 Directory Network (DN) ADI

```go
// DnUrl returns `acc://dn.acme`.
func DnUrl() *url.URL {
    return &url.URL{Authority: dnUrlBase + TLD}
}
```

The Directory Network ADI (`acc://dn.acme`) is a special ADI that:
- Serves as the root of the network's directory services
- Contains system accounts like the oracle, network definitions, and routing tables
- Cannot be created or controlled by users
- Has special validation rules in the protocol

### 2.2 Business Validation Network (BVN) ADIs

```go
// PartitionUrl returns `acc://bvn-${partition}.acme` or `acc://dn.acme`.
func PartitionUrl(partition string) *url.URL {
    if strings.EqualFold(partition, Directory) {
        return DnUrl()
    }
    return &url.URL{Authority: bvnUrlPrefix + partition + TLD}
}
```

BVN ADIs (e.g., `acc://bvn-x.acme`) are:
- Reserved for each Business Validation Network in the system
- Used to manage network-specific accounts and data
- Automatically created during network initialization
- Protected from user creation or modification

### 2.3 Unknown ADI

```go
// UnknownUrl returns `acc://unknown`.
func UnknownUrl() *url.URL {
    return &url.URL{Authority: Unknown}
}

// IsUnknown checks if the authority is 'unknown' or 'unknown.acme'.
func IsUnknown(u *url.URL) bool {
    return strings.EqualFold(u.Authority, Unknown) ||
        strings.EqualFold(u.Authority, Unknown+TLD)
}
```

The Unknown ADI (`acc://unknown` or `acc://unknown.acme`) is:
- Used to indicate that the principal of a transaction is unknown
- Used in special cases where an identity cannot be determined
- Reserved and cannot be created by users

### 2.4 ACME ADI

```go
// AcmeUrl returns `acc://ACME`.
func AcmeUrl() *url.URL {
    return &url.URL{Authority: ACME}
}
```

The ACME ADI (`acc://ACME`) is:
- Reserved for the native ACME token
- Used as the authority for the ACME token issuer
- Protected from user creation or modification

## 3. Special System Accounts

The Accumulate protocol includes several special system accounts that have unique properties and handling.

### 3.1 System Ledger

```go
type SystemLedger struct {
    fieldsSet  []bool
    Url        *url.URL
    extraData  []byte
}
```

The System Ledger account (`AccountTypeSystemLedger`):
- Used for internal system record-keeping
- Not directly accessible through normal API calls
- Managed automatically by the protocol
- Has special transaction handling and validation rules

### 3.2 Anchor Ledger

```go
// AnchorPool is the path to a node's anchor chain account.
AnchorPool = "anchors"
```

The Anchor Ledger (`acc://bvn-x.acme/anchors` or `acc://dn.acme/anchors`):
- Stores anchors between chains
- Has special transaction types and validation rules
- Is automatically created and managed by the protocol
- Plays a critical role in the cross-chain security model

### 3.3 Oracle Account

```go
// Oracle is the path to a node's anchor chain account.
Oracle = "oracle"

// PriceOracle returns acc://dn/oracle
func PriceOracle() *url.URL {
    return DnUrl().JoinPath(Oracle)
}
```

The Oracle account (`acc://dn.acme/oracle`):
- Provides price data for ACME tokens
- Used for fee calculations
- Has special update mechanisms
- Is protected from user modification

### 3.4 Network and Routing Accounts

```go
// Network is the path to a node's network definition data account.
Network = "network"

// Routing is the path to a node's routing table data account.
Routing = "routing"
```

These special accounts:
- Store network configuration and routing information
- Are critical for network operation
- Have special access controls and validation rules
- Are automatically managed by the protocol

## 4. Lite Accounts vs. Full Accounts

Accumulate distinguishes between two major categories of accounts: Lite Accounts and Full Accounts (ADI-based accounts).

### 4.1 Lite Accounts

Lite accounts are derived from public keys and do not require an ADI. They include:

#### 4.1.1 Lite Token Account

```go
// LiteTokenAddress returns an lite address for the given public key and token URL
func LiteTokenAddress(pubKey []byte, tokenUrlStr string, signatureType SignatureType) (*url.URL, error)
```

Lite Token Accounts:
- Are derived from public key hashes
- Have the format `acc://<key-hash-and-checksum>/<token-url>`
- Do not require an ADI
- Have limited functionality compared to ADI token accounts
- Are handled specially in transaction validation and routing

#### 4.1.2 Lite Data Account

```go
// LiteDataAddress returns a lite address for the given chain id
func LiteDataAddress(chainId []byte) (*url.URL, error)
```

Lite Data Accounts:
- Are derived from chain IDs
- Have the format `acc://<chain-id>`
- Do not require an ADI
- Have special validation and routing rules

#### 4.1.3 Lite Identity

Lite Identities:
- Represent the identity aspect of a lite account
- Are derived from public key hashes
- Have special validation rules
- Cannot own other accounts

### 4.2 Full Accounts (ADI-based)

Full accounts exist within ADIs and have additional capabilities:

```go
type FullAccount interface {
    Account
    GetAuth() *AccountAuth
}
```

Full accounts:
- Belong to an ADI
- Have authentication controls
- Can have complex authorization schemes
- Include token accounts, data accounts, key books, etc.

## 5. Special URL Handling and Validation

The Accumulate protocol implements special validation rules for ADI URLs:

```go
// IsValidAdiUrl returns an error if the URL is not valid for an ADI.
func IsValidAdiUrl(u *url.URL, allowReserved bool) error
```

Key validation rules include:
1. Must be valid UTF-8
2. Authority must not include a port number
3. Must have a non-empty hostname
4. Hostname must end with `.acme`
5. Hostname must not be a pure number
6. Hostname must not be 48 hexadecimal digits (to avoid confusion with lite accounts)
7. Must not have a path, query, or fragment
8. Must not be a reserved URL, such as ACME, DN, or BVN-* (unless `allowReserved` is true)

## 6. Special Transaction Handling

Certain account types have special transaction handling:

### 6.1 System Transactions

```go
// IsSystem returns true if the transaction type is system.
func (t TransactionType) IsSystem() bool {
    return TransactionMaxSynthetic.GetEnumValue() < t.GetEnumValue() && 
           t.GetEnumValue() <= TransactionMaxSystem.GetEnumValue()
}
```

System transactions:
- Are only generated by the protocol itself
- Bypass normal fee requirements
- Have special validation and execution rules
- Include types like `SystemGenesis` and `SystemWriteData`

### 6.2 Synthetic Transactions

Synthetic transactions:
- Are generated by the protocol in response to user transactions
- Have special routing and execution rules
- Include cross-chain operations like anchoring
- Are handled differently for fee calculations

## 7. Conclusion

ADIs and special accounts form the foundation of the Accumulate protocol's unique architecture. Understanding the exceptional handling of these entities is crucial for developers working with the protocol, as they have special properties, validation rules, and transaction handling that differ from regular accounts.

The reserved ADIs (DN, BVNs, Unknown, ACME) and system accounts (System Ledger, Anchor Ledger, Oracle, Network, Routing) play critical roles in the protocol's operation and have special protections to ensure system integrity. Meanwhile, the distinction between lite accounts and full accounts provides flexibility while maintaining security and scalability.
