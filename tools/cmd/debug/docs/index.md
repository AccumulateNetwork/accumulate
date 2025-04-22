# Accumulate Debug Tools Documentation

```yaml
# AI-METADATA
document_type: index
project: accumulate_network
component: debug_tools
version: current
key_concepts:
  - peer_discovery
  - network_status
  - healing_utilities
  - stateless_design
  - url_normalization
  - multiaddress_parsing
  - error_handling
dependencies:
  - github.com/multiformats/go-multiaddr
  - github.com/cometbft/cometbft/p2p
  - github.com/spf13/cobra
  - gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc
related_files:
  - dev_v3/DEVELOPMENT_PLAN.md
  - dev_v4/peer_discovery_analysis.md
  - code_examples.md

# Knowledge Graph
relationships:
  - source: peer_discovery
    relation: uses
    target: multiaddress_parsing
  - source: url_normalization
    relation: depends_on
    target: protocol.PartitionUrl
  - source: healing_utilities
    relation: implements
    target: stateless_design
  - source: network_status
    relation: depends_on
    target: peer_discovery
  - source: error_handling
    relation: enhances
    target: healing_utilities

# Version Compatibility
version_compatibility:
  - version: v1
    status: deprecated
    features: [basic_healing, routing, url_handling]
    directory: heal_v1
  - version: v2
    status: legacy
    features: [synthetic_chains, transaction_healing, fast_sync]
    directory: dev_v2
  - version: v3
    status: current
    features: [stateless_design, minimal_caching, url_standardization]
    directory: dev_v3
  - version: v4
    status: development
    features: [enhanced_peer_discovery, address_directory, peer_state]
    directory: dev_v4

# Implementation Status
implementation_status:
  components:
    - name: peer_discovery
      status: complete
      version: v4
    - name: url_normalization
      status: complete
      version: v3
    - name: stateless_healing
      status: in_progress
      version: v3
      remaining_tasks: [error_handling_improvements, performance_testing]
    - name: address_directory
      status: complete
      version: v4
```

This documentation covers the debug tools available in the Accumulate Network, including peer discovery, network status, and healing utilities.

## Documentation Structure

The documentation is organized into the following sections:

### 1. [Core Concepts](concepts/index.md)
- Accumulate Digital Identities (ADIs)
- Key management and security
- URL addressing and routing
- API version comparison
- Tendermint/CometBFT integration

### 2. [Development Version 4 (Latest)](dev_v4/index.md)
- Latest implementation of peer discovery and address directory
- Enhanced peer discovery utility with multiple extraction methods
- Comprehensive testing and integration guidelines

### 3. [Development Version 3](dev_v3/DEVELOPMENT_PLAN.md)
- Network status improvements
- Development plans and implementation details

### 4. [Development Version 2](dev_v2/index.md)
- Transaction healing process
- Synthetic chains implementation
- Fast sync and memory database optimizations

### 5. [Healing Version 1](heal_v1/index.md)
- Original healing implementation
- Routing and URL handling
- API issues and solutions

### 6. [Code Examples](code_examples.md)
- Example code snippets for common debug tasks
- Usage patterns and best practices

## Key Topics

The following key topics are covered across multiple documentation files:

- **[URL Handling and Normalization](code_examples.md#url-normalization)**: Standardizing URL formats to ensure consistent routing
- **[Peer Discovery](dev_v4/peer_discovery_analysis.md)**: Extracting host information from various address formats
- **[Multiaddress Handling](dev_v3/network_status.md#multiaddress-handling)**: Working with libp2p multiaddresses
- **[API Integration](dev_v3/network_status.md#api-usage)**: Using Accumulate APIs for network operations
- **[Address Directory](dev_v4/address_design.md)**: Managing network peers and addresses
- **[Stateless Design](dev_v3/DEVELOPMENT_PLAN.md#caching-system-implementation)**: New implementation uses a stateless approach with minimal caching
- **[Accumulate Digital Identities](concepts/adi_documentation.md)**: Core identity structure in Accumulate
- **[API Version Comparison](concepts/adi_api_versions.md)**: Evolution of APIs from v1 to v3
- **[Tendermint/CometBFT Integration](concepts/adi_api_versions.md#tendermintcometbft-apis-and-accumulate)**: How Tendermint/CometBFT provides consensus for Accumulate

```yaml
# AI-METADATA
# Cross-Document Concept Linking
concept_links:
  - concept: url_normalization
    defined_in: code_examples.md#url-normalization
    used_in: 
      - dev_v3/DEVELOPMENT_PLAN.md#url-construction-standardization
      - dev_v4/peer_discovery_analysis.md
    related_functions:
      - normalizeUrl
      - protocol.PartitionUrl
      - protocol.FormatUrl
    importance: critical
  
  - concept: stateless_design
    defined_in: dev_v3/DEVELOPMENT_PLAN.md#caching-system-implementation
    used_in:
      - code_examples.md#stateless-design
    related_components:
      - anchor_healing
      - peer_discovery
    importance: high
    key_characteristics: [no_persistent_database, minimal_caching, no_retry_mechanism]
  
  - concept: multiaddress_parsing
    defined_in: dev_v4/peer_discovery_analysis.md
    used_in:
      - dev_v4/peer_discovery_testing.md
      - dev_v3/network_status.md#multiaddress-handling
    related_functions:
      - ExtractHostFromMultiaddr
      - multiaddr.NewMultiaddr
    importance: high
    external_dependencies: [github.com/multiformats/go-multiaddr]
  
  - concept: error_handling
    defined_in: dev_v3/DEVELOPMENT_PLAN.md#error-handling-implementation
    used_in:
      - code_examples.md
    related_patterns:
      - error_wrapping
      - nil_checking
      - descriptive_errors
    importance: medium
```

> **IMPORTANT DESIGN NOTE:** The new healing implementation (v3+) uses a **stateless design** with very limited caching, unlike previous versions. There is no persistent database, and the only caching is to avoid regenerating rejected healing transactions. There is also no automatic retry mechanism.

## Implementation Guidelines

When implementing new features or utilities, please follow these documentation guidelines:

1. **Documentation Location**: 
   - Place all documentation in the appropriate subdirectory under `/tools/cmd/debug/docs/`
   - Create new subdirectories for major features or components
   - Use version-specific directories (e.g., `dev_v4`, `heal_v1`) for different implementations

2. **Documentation Structure**:
   - Each subdirectory should have an `index.md` file
   - Use consistent formatting and structure across all documentation
   - Include examples, API references, and testing guidelines

3. **Cross-References**:
   - Use relative links to reference other documentation files
   - Update the main index when adding new documentation

4. **AI-Friendly Format**:
   - Include structured metadata where appropriate
   - Use consistent code block formatting
   - Provide comprehensive API references

## Related Code

The documentation in this directory corresponds to the code in the following locations:

- [Debug Tools](../)
- [Network Healing](../new_heal/)
- [Peer Discovery](../new_heal/peerdiscovery/)
