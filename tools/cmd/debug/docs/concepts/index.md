# Accumulate Concepts Documentation

```yaml
# AI-METADATA
document_type: index
project: accumulate_network
component: concepts
version: current
authors:
  - accumulate_team
last_updated: 2023-11-15
key_concepts:
  - accumulate_digital_identity
  - key_management
  - url_addressing
  - api_versions
  - tendermint_integration
dependencies:
  - gitlab.com/accumulatenetwork/accumulate/protocol
  - gitlab.com/accumulatenetwork/accumulate/pkg/api/v3
  - gitlab.com/accumulatenetwork/accumulate/internal/api/v2
  - gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/deprecated/v1
  - github.com/cometbft/cometbft
related_files:
  - adi_documentation.md
  - adi_key_management.md
  - adi_url_addressing.md
  - adi_api_versions.md

# Knowledge Graph
relationships:
  - source: accumulate_digital_identity
    relation: uses
    target: key_management
  - source: accumulate_digital_identity
    relation: identified_by
    target: url_addressing
  - source: api_versions
    relation: implements
    target: accumulate_digital_identity
  - source: tendermint_integration
    relation: provides_consensus_for
    target: accumulate_digital_identity
  - source: key_management
    relation: secures
    target: accumulate_digital_identity
```

This documentation covers the core concepts of the Accumulate Network, including Accumulate Digital Identities (ADIs), key management, URL addressing, and API versions.

## Documentation Structure

The documentation is organized into the following sections:

### 1. [Accumulate Digital Identities (ADIs)](adi_documentation.md)
- Core identity structure in Accumulate
- Identity separation from keys
- Hierarchical identity management
- API version support and evolution

### 2. [Key Management](adi_key_management.md)
- Key books and key pages
- Authority delegation
- Signature validation
- Key rotation strategies

### 3. [URL Addressing](adi_url_addressing.md)
- URL structure and formats
- URL routing process
- URL normalization
- Common URL operations

### 4. [API Version Comparison](adi_api_versions.md)
- Evolution from v1 to v3
- Feature comparison across versions
- API method differences
- Migration guidance
- Tendermint/CometBFT integration

## Key Topics

The following key topics are covered across multiple documentation files:

- **[Identity Management](adi_documentation.md)**: Creating and managing digital identities
- **[Key Security](adi_key_management.md)**: Securing identities with sophisticated key management
- **[URL Routing](adi_url_addressing.md#url-routing)**: How URLs are routed through the network
- **[API Evolution](adi_api_versions.md#api-evolution-timeline)**: How APIs have evolved over time
- **[Tendermint Integration](adi_api_versions.md#tendermintcometbft-apis-and-accumulate)**: How Tendermint/CometBFT integrates with Accumulate

```yaml
# AI-METADATA
# Cross-Document Concept Linking
concept_links:
  - concept: accumulate_digital_identity
    defined_in: adi_documentation.md
    used_in: 
      - adi_key_management.md
      - adi_url_addressing.md
      - adi_api_versions.md
    related_types:
      - protocol.ADI
      - protocol.AccountAuth
      - protocol.AuthorityEntry
    importance: critical
  
  - concept: key_management
    defined_in: adi_key_management.md
    used_in:
      - adi_documentation.md
      - adi_api_versions.md
    related_components:
      - key_books
      - key_pages
      - authority_delegation
    importance: high
    key_characteristics: [hierarchical, flexible, secure]
  
  - concept: url_addressing
    defined_in: adi_url_addressing.md
    used_in:
      - adi_documentation.md
      - adi_api_versions.md
    related_functions:
      - protocol.ParseUrl
      - protocol.FormatUrl
      - protocol.PartitionUrl
    importance: high
    
  - concept: api_versions
    defined_in: adi_api_versions.md
    used_in:
      - adi_documentation.md
      - adi_key_management.md
      - adi_url_addressing.md
    related_versions:
      - v1
      - v2
      - v3
    importance: high
    
  - concept: tendermint_integration
    defined_in: adi_api_versions.md#tendermintcometbft-apis-and-accumulate
    used_in:
      - adi_documentation.md
    related_components:
      - consensus
      - transaction_submission
      - block_queries
    importance: high
    external_dependencies: [github.com/cometbft/cometbft]
```

## Implementation Guidelines

When implementing new features or extending existing ones, please follow these documentation guidelines:

1. **Documentation Location**: 
   - Place all concept documentation in this directory
   - Use consistent naming conventions (e.g., `adi_*.md` for ADI-related concepts)

2. **Documentation Structure**:
   - Each file should have a clear structure with headings and subheadings
   - Include metadata at the top of each file
   - Use concept tags to mark important sections

3. **Cross-References**:
   - Use relative links to reference other documentation files
   - Update this index when adding new documentation

4. **AI-Friendly Format**:
   - Include structured metadata with `<!-- @concept name -->` tags
   - Use `<!-- @importance level -->` tags to indicate importance
   - Add `<!-- @description text -->` tags for clear descriptions

## Related Documentation

- [Main Documentation Index](../index.md)
- [API Reference](../api/index.md)
- [Developer Guides](../guides/index.md)
