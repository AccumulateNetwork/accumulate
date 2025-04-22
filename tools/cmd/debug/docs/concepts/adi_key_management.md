# ADI Key Management in Accumulate

```yaml
# AI-METADATA
document_type: concept_documentation
project: accumulate_network
component: key_management
version: current
authors:
  - accumulate_team
last_updated: 2023-11-15

# Core Concepts
key_concepts:
  - key_book:
      description: "Collection of key pages that define the signing authority for an ADI"
      importance: critical
      related_patterns: [hierarchical_authority, priority_ordering]
  - key_page:
      description: "Collection of keys with a signing threshold"
      importance: high
      related_patterns: [threshold_signatures, key_rotation]
  - signature_threshold:
      description: "Number of signatures required from a key page to authorize a transaction"
      importance: high
      related_patterns: [multi_signature, security_levels]
  - authority_delegation:
      description: "Process of delegating signing authority to other ADIs or key books"
      importance: medium
      related_patterns: [organizational_hierarchy, role_based_access]

# Related Components
related_components:
  - adi:
      relationship: "Uses key books for authority management"
      importance: critical
  - transaction:
      relationship: "Validated against key books and pages"
      importance: high
  - signature:
      relationship: "Verified against keys in key pages"
      importance: high

# Technical Details
language: go
dependencies:
  - gitlab.com/accumulatenetwork/accumulate/protocol
  - gitlab.com/accumulatenetwork/accumulate/pkg/url
  - gitlab.com/accumulatenetwork/accumulate/pkg/crypto

# Implementation References
implementation_files:
  - /protocol/key_book.go:
      contains: "Key book and page implementations"
  - /protocol/signature.go:
      contains: "Signature validation logic"
```

## Overview

Key management in Accumulate is built around a hierarchical structure that separates identity from keys, allowing for sophisticated authority management. This document explains the key management architecture used by Accumulate Digital Identities (ADIs).

## Key Management Architecture

<!-- @concept key_management_architecture -->
<!-- @importance critical -->
<!-- @description Hierarchical structure of key management in Accumulate -->

The Accumulate key management system uses a three-tier architecture:

1. **ADI**: The digital identity that owns accounts and resources
2. **Key Book**: Collection of key pages that define signing authority
3. **Key Page**: Collection of keys with a signature threshold

This hierarchical approach allows for:
- Separation of identity from signing authority
- Complex multi-signature requirements
- Key rotation without changing identity
- Role-based access control

## Key Books

<!-- @struct KeyBook -->
<!-- @description Collection of key pages that define signing authority -->
<!-- @purpose Provides hierarchical authority management for ADIs -->

Key books are collections of key pages ordered by priority. They serve as the primary authority mechanism for ADIs.

```go
// @struct KeyBook
// @description Collection of key pages that define signing authority
// @purpose Provides hierarchical authority management for ADIs
type KeyBook struct {
    // @field Url
    // @description Unique URL identifier for this key book
    // @format acc://<adi-name>.<domain>/book
    // @example acc://factom.acme/book
    Url *url.URL

    // @field PageCount
    // @description Number of key pages in this book
    // @purpose Quick access to page count without loading all pages
    PageCount int

    // @field AccountAuth
    // @description Authorization configuration for this key book
    // @embedded Contains authorities that can manage this key book
    AccountAuth
}
```

### Key Book Characteristics

1. **Priority Order**: Pages are evaluated in order, with page 1 having highest priority
2. **Delegation**: Key books can delegate authority to other key books
3. **Management**: Key books can be managed by their own pages or by other authorities
4. **Ownership**: Each key book belongs to an ADI

### Key Book Operations

<!-- @process key_book_operations -->
<!-- @importance high -->
<!-- @description Operations that can be performed on key books -->

Key books support several operations:

1. **Creation**: Created as part of ADI creation or explicitly
2. **Page Addition**: New key pages can be added to a book
3. **Page Removal**: Existing key pages can be removed
4. **Authority Update**: The authorities that manage the book can be updated

## Key Pages

<!-- @struct KeyPage -->
<!-- @description Collection of keys with a signature threshold -->
<!-- @purpose Defines multi-signature requirements for authorization -->

Key pages contain a set of public keys and a signature threshold that defines how many signatures are required to authorize a transaction.

```go
// @struct KeyPage
// @description Collection of keys with a signature threshold
// @purpose Defines multi-signature requirements for authorization
type KeyPage struct {
    // @field Url
    // @description Unique URL identifier for this key page
    // @format acc://<adi-name>.<domain>/book/<page-number>
    // @example acc://factom.acme/book/1
    Url *url.URL

    // @field Keys
    // @description Public keys contained in this page
    // @typically ED25519 or ECDSA public keys
    Keys []PublicKey

    // @field SignatureThreshold
    // @description Number of signatures required from this page
    // @range 1 to len(Keys)
    // @default 1
    SignatureThreshold int

    // @field AccountAuth
    // @description Authorization configuration for this key page
    // @embedded Contains authorities that can manage this key page
    AccountAuth
}
```

### Key Page Characteristics

1. **Threshold Signatures**: Requires a minimum number of signatures to authorize
2. **Key Types**: Supports multiple key types (ED25519, ECDSA, etc.)
3. **Management**: Can be managed by its own keys or by other authorities
4. **Ordering**: Pages are numbered and evaluated in order of priority

### Key Page Operations

<!-- @process key_page_operations -->
<!-- @importance high -->
<!-- @description Operations that can be performed on key pages -->

Key pages support several operations:

1. **Creation**: Added to a key book with an initial set of keys
2. **Key Addition**: New keys can be added to a page
3. **Key Removal**: Existing keys can be removed from a page
4. **Threshold Update**: The signature threshold can be updated
5. **Authority Update**: The authorities that manage the page can be updated

## Signature Validation Process

<!-- @process signature_validation -->
<!-- @importance critical -->
<!-- @description How signatures are validated against key books and pages -->

When a transaction is submitted, the signature validation process follows these steps:

1. **Authority Resolution**:
   - Identify the authority required for the transaction
   - Typically this is the ADI's key book

2. **Key Book Evaluation**:
   - Evaluate key pages in priority order (starting with page 1)
   - For each page, check if there are sufficient valid signatures

3. **Key Page Validation**:
   - Count valid signatures from the keys in the page
   - If the count meets or exceeds the threshold, authorization is granted
   - If not, proceed to the next page

4. **Final Decision**:
   - If any page provides sufficient signatures, the transaction is authorized
   - If no page provides sufficient signatures, the transaction is rejected

## Key Rotation Strategies

<!-- @concept key_rotation -->
<!-- @importance high -->
<!-- @description Strategies for rotating keys without disrupting operations -->

Accumulate's architecture enables several key rotation strategies:

> **API Version Note**: Key rotation capabilities have evolved significantly across API versions. V1 had limited rotation capabilities with direct key replacement only. V2 introduced page-based rotation, while V3 provides comprehensive rotation strategies including delegated authority rotation. See [ADI API Version Comparison](adi_api_versions.md) for details.

### 1. Gradual Key Rotation

1. Add new keys to an existing page
2. Update the threshold if necessary
3. Test the new keys
4. Remove the old keys

### 2. Page-Based Rotation

1. Add a new key page with higher priority
2. Test the new page
3. Remove the old page

### 3. Book-Based Rotation

1. Create a new key book
2. Update the ADI to use the new key book
3. Remove the old key book

## Authority Delegation Patterns

<!-- @concept authority_delegation_patterns -->
<!-- @importance medium -->
<!-- @description Patterns for delegating authority in organizations -->

Accumulate's key management architecture enables sophisticated authority delegation patterns:

### 1. Hierarchical Authority

```
ADI: organization.acme
  ├── Key Book: /book
  │     ├── Page 1: Executive Team (2-of-3)
  │     └── Page 2: Backup Recovery (3-of-5)
  │
  ├── Sub-ADI: finance.organization.acme
  │     └── Key Book: /book
  │           ├── Page 1: Finance Team (2-of-4)
  │           └── Page 2: Executive Team (delegated)
  │
  └── Sub-ADI: operations.organization.acme
        └── Key Book: /book
              ├── Page 1: Operations Team (3-of-6)
              └── Page 2: Executive Team (delegated)
```

### 2. Role-Based Access Control

```
ADI: organization.acme
  ├── Key Book: /admin-book
  │     └── Page 1: Administrators (2-of-3)
  │
  ├── Key Book: /finance-book
  │     └── Page 1: Finance Team (2-of-4)
  │
  ├── Key Book: /operations-book
  │     └── Page 1: Operations Team (3-of-6)
  │
  ├── Token Account: /tokens (Authority: /admin-book)
  ├── Data Account: /finance-data (Authority: /finance-book)
  └── Data Account: /operations-data (Authority: /operations-book)
```

## Security Considerations

<!-- @concept security_considerations -->
<!-- @importance critical -->
<!-- @description Security considerations for key management -->

1. **Threshold Selection**: Choose appropriate thresholds based on security requirements
2. **Key Storage**: Securely store private keys using hardware security modules when possible
3. **Recovery Planning**: Implement recovery mechanisms through backup key pages
4. **Authority Review**: Regularly review and audit authority structures
5. **Principle of Least Privilege**: Grant only necessary authorities to each key or role

## Best Practices

1. **Use Multiple Key Pages**: Create at least two key pages - one for daily use and one for recovery
2. **Implement Thresholds**: Use thresholds greater than 1 for important accounts
3. **Plan for Key Rotation**: Design key books with future key rotation in mind
4. **Document Authority Structure**: Maintain documentation of your authority structure
5. **Test Recovery Procedures**: Regularly test key recovery procedures

## Implementation Examples

### Basic ADI with Simple Key Management

```go
// Create a new ADI with a basic key book
func CreateBasicADI(name string, keys []PublicKey) (*protocol.ADI, error) {
    // Create the ADI
    adi := &protocol.ADI{
        Url: protocol.AccountUrl(name),
    }
    
    // Create a key book
    keyBook := &protocol.KeyBook{
        Url: protocol.FormatUrl(adi.Url, "book"),
    }
    
    // Create a key page with the provided keys
    keyPage := &protocol.KeyPage{
        Url: protocol.FormatUrl(keyBook.Url, "1"),
        Keys: keys,
        SignatureThreshold: 1,
    }
    
    // Link the key book to the ADI
    adi.AddAuthority(keyBook.Url)
    
    // Add the key page to the key book
    keyBook.AddPage(keyPage)
    
    return adi, nil
}
```

### Multi-Signature Key Management

```go
// Create an ADI with multi-signature key management
func CreateMultiSigADI(name string, dailyKeys []PublicKey, recoveryKeys []PublicKey) (*protocol.ADI, error) {
    // Create the ADI
    adi := &protocol.ADI{
        Url: protocol.AccountUrl(name),
    }
    
    // Create a key book
    keyBook := &protocol.KeyBook{
        Url: protocol.FormatUrl(adi.Url, "book"),
    }
    
    // Create a daily use key page (2-of-N)
    dailyPage := &protocol.KeyPage{
        Url: protocol.FormatUrl(keyBook.Url, "1"),
        Keys: dailyKeys,
        SignatureThreshold: 2,
    }
    
    // Create a recovery key page (3-of-N)
    recoveryPage := &protocol.KeyPage{
        Url: protocol.FormatUrl(keyBook.Url, "2"),
        Keys: recoveryKeys,
        SignatureThreshold: 3,
    }
    
    // Link the key book to the ADI
    adi.AddAuthority(keyBook.Url)
    
    // Add the key pages to the key book
    keyBook.AddPage(dailyPage)
    keyBook.AddPage(recoveryPage)
    
    return adi, nil
}
```

## Related Documentation

- [Accumulate Digital Identities (ADIs)](adi_documentation.md)
- [Transaction Signatures](transaction_signatures.md)
- [Security Best Practices](security_best_practices.md)
- [Enterprise Key Management](enterprise_key_management.md)
