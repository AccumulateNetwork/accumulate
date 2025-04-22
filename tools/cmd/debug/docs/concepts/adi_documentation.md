# Accumulate Digital Identities (ADIs)

```yaml
# AI-METADATA
document_type: concept_documentation
project: accumulate_network
component: digital_identities
version: current
authors:
  - accumulate_team
last_updated: 2023-11-15

# Core Concepts
key_concepts:
  - digital_identity:
      description: "Foundational identity structure in Accumulate that serves as a namespace for other objects"
      importance: critical
  - hierarchical_identity:
      description: "ADIs can have sub-ADIs, creating a hierarchical identity structure"
      importance: high
  - identity_authority:
      description: "ADIs use key books and pages to manage signing authority"
      importance: critical
  - identity_ownership:
      description: "ADIs separate identity from keys, allowing key rotation without identity changes"
      importance: high

# API Version Support
api_support:
  - v1:
      status: deprecated
      capabilities: [basic_adi_creation, simple_key_management, limited_queries]
      limitations: [no_key_books, no_sub_adis, basic_authority_only]
  - v2:
      status: stable
      capabilities: [enhanced_adi_creation, key_book_support, improved_queries]
      limitations: [limited_authority_delegation, partial_stateless_design]
  - v3:
      status: current
      capabilities: [comprehensive_adi_operations, advanced_key_management, full_sub_adi_support]
      limitations: [none]

# Related Components
related_components:
  - key_book:
      relationship: "Contains the signing authority for an ADI"
      importance: critical
  - key_page:
      relationship: "Contains public keys that can sign for an ADI"
      importance: high
  - token_account:
      relationship: "Can be created under an ADI to hold tokens"
      importance: high
  - data_account:
      relationship: "Can be created under an ADI to store data"
      importance: medium
  - api_versions:
      relationship: "Defines how ADIs are created and managed across different API versions"
      importance: high

# Technical Details
language: go
dependencies:
  - gitlab.com/accumulatenetwork/accumulate/protocol
  - gitlab.com/accumulatenetwork/accumulate/pkg/url
  - gitlab.com/accumulatenetwork/accumulate/pkg/api/v3
  - gitlab.com/accumulatenetwork/accumulate/internal/api/v2

# Implementation References
implementation_files:
  - /protocol/types_gen.go:
      contains: "ADI struct definition"
  - /protocol/account_auth.go:
      contains: "Account authorization logic"
  - /pkg/api/v3/adi.go:
      contains: "V3 API ADI operations"
  - /internal/api/v2/adi.go:
      contains: "V2 API ADI operations"
```

## Overview

Accumulate Digital Identities (ADIs) are the foundational identity structure in the Accumulate protocol. Unlike traditional blockchain systems where accounts are directly tied to public keys, ADIs separate identity from keys, creating a more flexible and enterprise-friendly identity model.

## Key Characteristics

### 1. Identity Separation

<!-- @concept identity_key_separation -->
<!-- @importance critical -->
<!-- @description ADIs separate identity from the keys that control them -->

ADIs fundamentally separate the identity itself from the cryptographic keys that control it. This separation allows:

- Key rotation without changing the identity
- Multiple keys to control a single identity
- Hierarchical key management structures
- Identity persistence even as security requirements evolve

### 2. Hierarchical Structure

<!-- @concept hierarchical_identity_structure -->
<!-- @importance high -->
<!-- @description ADIs can form hierarchies with parent-child relationships -->

ADIs can exist in hierarchical relationships:

- Parent ADIs can create and manage child ADIs
- Sub-ADIs inherit certain properties from parent ADIs
- Hierarchies can model organizational structures
- URL naming reflects these hierarchical relationships

### 3. Namespace Management

<!-- @concept namespace_management -->
<!-- @importance high -->
<!-- @description ADIs serve as namespaces for accounts and other objects -->

Each ADI functions as a namespace that can contain:

- Token accounts
- Data accounts
- Key books and pages
- Other sub-ADIs

## Technical Implementation

### ADI Structure

```go
// @struct ADI
// @description Core digital identity structure in Accumulate
// @purpose Provides identity namespace and authority management
type ADI struct {
    // @field Url
    // @description Unique URL identifier for this ADI
    // @format acc://<adi-name>.<domain>
    // @example acc://factom.acme
    Url *url.URL

    // @field AccountAuth
    // @description Authorization configuration for this ADI
    // @embedded Contains authorities that can sign for this ADI
    AccountAuth
}

// @struct AccountAuth
// @description Authorization configuration for accounts
// @purpose Defines which authorities can sign for an account
type AccountAuth struct {
    // @field Authorities
    // @description List of authority entries that can sign for this account
    // @typically Points to key books or pages
    Authorities []AuthorityEntry
}

// @struct AuthorityEntry
// @description Reference to an authority that can sign for an account
// @purpose Links accounts to their signing authorities
type AuthorityEntry struct {
    // @field Url
    // @description URL of the authority (typically a key book or page)
    // @example acc://factom.acme/book
    Url *url.URL
    
    // @field Disabled
    // @description If true, this authority is disabled
    // @security_note Disabled authorities allow anyone to sign, used for testing
    Disabled bool
}
```

## ADI Lifecycle

### Creation

<!-- @process adi_creation -->
<!-- @importance critical -->
<!-- @description Process for creating a new ADI -->

1. **Transaction Preparation**:
   - Create an ADI creation transaction
   - Specify the desired ADI name
   - Include key book creation parameters

2. **Validation**:
   - ADI name must follow naming rules (letters, numbers, dashes)
   - ADI name must not already exist
   - Transaction must be properly signed and have sufficient credits

3. **Execution**:
   - System creates the ADI record
   - System creates the initial key book
   - System links the key book as the ADI's authority

### Authority Management

<!-- @process authority_management -->
<!-- @importance high -->
<!-- @description How ADI authorities are managed -->

ADIs use a sophisticated authority system:

1. **Key Books**: Collections of key pages in priority order
2. **Key Pages**: Collections of keys with threshold requirements
3. **Authority Delegation**: ADIs can delegate authority to other ADIs

This structure allows for complex signing scenarios such as:
- Multi-signature requirements
- Hierarchical approval processes
- Role-based access control

## URL Addressing

<!-- @concept url_addressing -->
<!-- @importance high -->
<!-- @description How ADIs are addressed using URLs -->

ADIs use a URL-based addressing scheme:

```
acc://<adi-name>.<domain>
```

Examples:
- `acc://factom.acme` - A top-level ADI
- `acc://subdomain.factom.acme` - A sub-ADI

Sub-resources are addressed as paths:
- `acc://factom.acme/tokens` - A token account
- `acc://factom.acme/book` - A key book
- `acc://factom.acme/book/1` - A key page
- `acc://factom.acme/data` - A data account

## Practical Applications

### Enterprise Identity Management

ADIs enable enterprises to:
- Create a single root identity for the organization
- Create sub-identities for departments or projects
- Manage signing authority through organizational roles
- Rotate keys without changing identities

### Supply Chain Tracking

In supply chain scenarios, ADIs can:
- Represent each participant in the supply chain
- Create hierarchical relationships between participants
- Maintain consistent identities despite key rotations
- Provide namespaces for tracking data and tokens

### Decentralized Finance

For DeFi applications, ADIs offer:
- Persistent identities for financial institutions
- Hierarchical account structures
- Sophisticated multi-signature controls
- Separation of identity from signing authority

## Best Practices

1. **Plan Your Hierarchy**: Design your ADI hierarchy to match your organizational structure
2. **Key Management**: Implement proper key management procedures for ADI authorities
3. **Authority Design**: Design authority structures with appropriate thresholds and redundancy
4. **Sub-ADI Strategy**: Use sub-ADIs to delegate control while maintaining organizational relationships

## Implementation Considerations

### Security

- ADIs separate identity from keys, but proper key management remains essential
- Consider using multi-signature authorities for high-value ADIs
- Implement key rotation procedures for long-lived ADIs

### Performance

- ADI lookups are optimized in the Accumulate protocol
- Sub-ADI operations may require validation against parent ADIs
- Authority validation complexity scales with the complexity of the authority structure

### Scalability

- ADIs are designed to scale to millions of identities
- Hierarchical structures help manage complexity
- URL-based addressing provides efficient routing

## Comparison with Other Identity Systems

| Feature | Accumulate ADIs | Traditional Blockchain | Centralized Identity |
|---------|----------------|------------------------|----------------------|
| Identity-Key Separation | Yes | No | Sometimes |
| Hierarchical Structure | Yes | No | Sometimes |
| Key Rotation | Simple | Requires new identity | Simple |
| Multi-signature | Sophisticated | Basic | Varies |
| Namespace Management | Built-in | Limited | Varies |

## API Version Support

<!-- @concept api_version_support -->
<!-- @importance high -->
<!-- @description How ADIs are supported across different API versions -->

Accumulate has evolved through multiple API versions, each with different levels of support for ADI functionality. For a comprehensive comparison, see [ADI API Version Comparison](adi_api_versions.md).

### API v1 (Deprecated)

The initial API implementation provided basic ADI functionality:

- **Basic ADI Creation**: Simple creation with a single key
- **Limited Key Management**: Direct key addition/removal without hierarchical structure
- **Basic Query Capabilities**: Simple queries with limited filtering
- **No Sub-ADI Support**: Limited hierarchical identity management

```go
// API v1: Create ADI example
func createADI(client *v1.Client, name string, publicKey []byte) error {
    status, err := client.CreateADI(ctx, name, publicKey)
    return err
}
```

### API v2 (Stable)

The v2 API introduced significant improvements to ADI management:

- **Enhanced ADI Creation**: Support for key books during creation
- **Improved Key Management**: Introduction of key books and pages
- **Better Query Options**: Enhanced filtering and pagination
- **Basic Sub-ADI Support**: Initial support for hierarchical identities

```go
// API v2: Create ADI with key book
func createADI(client *v2.Client, name string, keys []protocol.PublicKey, threshold int) error {
    status, err := client.CreateADI(ctx, name, keys, threshold)
    return err
}
```

### API v3 (Current)

The current API provides comprehensive ADI functionality:

- **Stateless Design**: Fully stateless API requiring no state between operations
- **Comprehensive ADI Operations**: Complete support for all ADI operations
- **Advanced Key Management**: Sophisticated key book and page management
- **Full Sub-ADI Support**: Complete hierarchical identity management
- **Advanced Query Capabilities**: Comprehensive filtering, sorting, and pagination

```go
// API v3: Create ADI with full options
func createADI(client *v3.Client, name string, options *v3.CreateADIOptions) error {
    adiUrl, _ := protocol.ParseUrl("acc://" + name + ".acme")
    
    options.Url = adiUrl
    _, err := client.CreateADI(ctx, options)
    return err
}
```

### Historical Context

The evolution of the API reflects the maturing understanding of identity management in blockchain systems:

- **v1 to v2**: Recognized the need for hierarchical key management and more sophisticated identity structures
- **v2 to v3**: Implemented lessons learned about stateless design, optimized routing, and comprehensive identity management

This progression demonstrates Accumulate's commitment to building a robust identity layer that can support enterprise-grade applications while maintaining flexibility and security.

### Tendermint/CometBFT Integration

All Accumulate API versions operate on top of Tendermint (now known as CometBFT) consensus. While the Accumulate APIs provide the primary interface for working with ADIs, some advanced use cases may require direct interaction with the underlying Tendermint APIs. For more details on this integration, see [ADI API Version Comparison](adi_api_versions.md#tendermintcometbft-apis-and-accumulate).

## Related Documentation

- [ADI API Version Comparison](adi_api_versions.md) - Detailed comparison of ADI functionality across API versions
- [Key Books and Pages](key_management.md) - Detailed explanation of ADI key management
- [Token Accounts](token_accounts.md) - How token accounts relate to ADIs
- [Data Accounts](data_accounts.md) - How data accounts relate to ADIs
- [URL Addressing](url_addressing.md) - How ADIs are addressed using URLs
