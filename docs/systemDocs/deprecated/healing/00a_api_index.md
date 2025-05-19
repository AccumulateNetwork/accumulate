# Accumulate Healing API Documentation Index

<!-- AI-METADATA
type: index
version: 1.0
topic: healing_apis
subtopics: ["api_layers", "api_implementation", "api_comparison"]
related_code: ["pkg/api/v3/api.go", "pkg/client/api/v2/query.go", "internal/core/healing/healing.go"]
critical_rules: ["never_fabricate_data", "follow_reference_implementation"]
tags: ["healing", "api", "documentation", "index", "ai_optimized"]
-->

## Overview

This index provides a comprehensive guide to the API-related aspects of the healing documentation. The Accumulate healing processes rely on multiple API layers with different capabilities and purposes. This document serves as a central reference point for all API-related documentation.

## API Documentation

- **Healing APIs Overview** (Primary: [02_healing_apis.md](./02_healing_apis.md))
  - *Purpose*: Overview of the healing APIs, their structure, and implementation
  - *Primary Code*: `internal/core/healing/healing.go`
  - *Related Documents*: [healing_api_comparison.md](./healing_api_comparison.md)
  - *Key Interfaces*:
    - `HealSyntheticArgs`: Parameters for synthetic transaction healing
    - `HealAnchorArgs`: Parameters for anchor healing
    - `SequencedInfo`: Metadata for sequenced operations
    - `ResolveSequenced<T>`: Generic resolver for sequenced items

- **P2P API Considerations** (Primary: [05a_p2p_api_considerations.md](./05a_p2p_api_considerations.md))
  - *Purpose*: Documents P2P API usage and plan to validate performance characteristics
  - *Primary Code*: `tools/cmd/debug/heal_common.go`, `pkg/api/v3/p2p/dial.go`
  - *Key Topics*:
    - Current P2P API usage in healing implementation
    - Plan for systematic performance testing
    - Benchmark methodology and metrics
    - Alternative APIs to evaluate (JSON-RPC, REST, gRPC)
    - Evidence-based optimization strategies

- **API Layers** (Primary: [05_api_layers.md](./05_api_layers.md))
  - *Purpose*: Documentation of the API layers used in the healing processes
  - *Contains*: Tendermint APIs, v1 APIs, v2 APIs, v3 APIs, scope property
  - *Primary Code*: `pkg/api/v3/api.go`, `pkg/client/api/v2/query.go`
  - *Layer Interactions*:
    - Tendermint RPC for low-level blockchain access
    - v1 APIs for legacy support
    - v2 APIs for current implementation
    - v3 APIs for enhanced functionality
    - Scope property for partition-specific queries
  - *Version-Specific Implementations*:
    - `buildSynthReceiptV1`: Receipt building for pre-Vandenberg versions
    - `buildSynthReceiptV2`: Receipt building for V2 Vandenberg and later
    - Version detection via `NetInfo.Status.ExecutorVersion.V2VandenbergEnabled()`
    - Different message formats based on `V2BaikonurEnabled()`

- **API Implementation Analysis** (Primary: [healing_api_comparison.md](./healing_api_comparison.md))
  - *Purpose*: Comparison between healing APIs and their implementation
  - *Contains*: Sequence API comparison, network status API comparison
  - *Supplements*: [02_healing_apis.md](./02_healing_apis.md), [05_api_layers.md](./05_api_layers.md)
  - *Critical Analysis*:
    - API version compatibility issues
    - Error handling differences between versions
    - Performance characteristics
    - Scope property implementation details

## API Code Reference

- **v3 API Definitions** (Primary: `pkg/api/v3/api.go`)
  - *Purpose*: Defines the v3 API interfaces used by healing processes
  - *Key Interfaces*:
    - Query interfaces for transaction retrieval
    - Network status interfaces
    - Chain access interfaces
    - Scope property implementation
  - *Related to*: [05_api_layers.md](./05_api_layers.md)

- **v2 Query Implementation** (Primary: `pkg/client/api/v2/query.go`)
  - *Purpose*: Implements v2 query functionality used in healing
  - *Key Functions*:
    - Transaction query methods
    - Chain state queries
    - Sequence information retrieval
    - Error handling and retries
  - *Related to*: [05_api_layers.md](./05_api_layers.md)

- **Error Definitions** (Primary: `pkg/errors/errors.go`)
  - *Purpose*: Defines structured error types used throughout healing
  - *Key Error Types*:
    - BadRequest: Invalid input parameters
    - NotFound: Resource not available
    - Internal: Unexpected system errors
    - Timeout: Operation exceeded time limit
  - *Error Patterns*:
    - Error wrapping with context
    - Error categorization
    - Detailed error messages
  - *Related to*: [09_error_handling.md](./09_error_handling.md)

## Topic Index

### API Related Topics
- **API Layers and Versioning** (Primary: [05_api_layers.md](./05_api_layers.md))
  - *Scope Property*: Partition-specific query mechanism
  - *Tendermint APIs*: Low-level blockchain access protocols
  - *v1 APIs*: Legacy support for backward compatibility
  - *v2 APIs*: Current implementation with enhanced features
  - *v3 APIs*: Next-generation APIs with improved performance
  - *Cross-Version Compatibility*: Handling version differences
- **API Implementation** (Primary: [02_healing_apis.md](./02_healing_apis.md))
  - *Interface Definitions*: Core healing API interfaces
  - *Parameter Structures*: Arguments for healing operations
  - *Return Types*: Structured response formats
  - *Error Handling*: API-specific error management
  - *Additional comparison details*: [healing_api_comparison.md](./healing_api_comparison.md)

## Common AI Queries

- **What are the different API layers used in healing?**
  See [05_api_layers.md](./05_api_layers.md) for a comprehensive breakdown of the API layers.

- **How do the healing APIs differ from their implementation?**
  See [healing_api_comparison.md](./healing_api_comparison.md) for a detailed comparison.

- **What are the key interfaces for healing operations?**
  See [02_healing_apis.md](./02_healing_apis.md) for interface definitions and [internal/core/healing/healing.go](../../../internal/core/healing/healing.go) for implementation.

- **How is error handling implemented in the healing APIs?**
  See [09_error_handling.md](./09_error_handling.md) for error handling patterns and [pkg/errors/errors.go](../../../../../pkg/errors/errors.go) for error definitions.

## Related Documents

- [Main Index](./00_index.md)
- [Healing Processes Index](./00b_healing_processes_index.md)
- [Technical Infrastructure Index](./00c_technical_index.md)
- [Command-Line Usage Guide](./00d_cli_usage.md)
