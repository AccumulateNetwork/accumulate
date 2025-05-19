# Accumulate Healing Technical Infrastructure Documentation Index

<!-- AI-METADATA
type: index
version: 1.0
topic: healing_infrastructure
subtopics: ["database_caching", "cryptography", "error_handling", "testing"]
related_code: ["tools/cmd/debug/heal_common.go", "internal/core/healing/synthetic.go", "pkg/errors/errors.go"]
critical_rules: ["never_fabricate_data", "follow_reference_implementation"]
tags: ["healing", "infrastructure", "documentation", "index", "ai_optimized"]
-->

## Overview

This index provides a comprehensive guide to the technical infrastructure supporting the Accumulate healing processes. This includes database and caching mechanisms, cryptographic operations, error handling patterns, and testing frameworks that ensure the reliability and integrity of the healing processes.

## Technical Infrastructure Documentation

- **Database and Caching** (Primary: [06_database_caching.md](./06_database_caching.md))
  - *Purpose*: Documentation of the database and caching infrastructure
  - *Contains*: Light client database, chain access, LRU cache, transaction map, on-demand fetching
  - *Primary Code*: `tools/cmd/debug/heal_common.go`
  - *Related Issues*: [11_implementation_issues.md](./11_implementation_issues.md)
  - *Key Components*:
    - Light client database for efficient chain access
    - LRU cache with fixed size (1000 entries)
    - Transaction map for hash-based lookups
    - On-demand transaction fetching mechanism
  - **Critical Implementation**: On-demand transaction fetching
    - Intercepts "key not found" errors
    - Uses sequence numbers for targeted fetching
    - Never fabricates unavailable data
    - Implements fallback mechanisms per reference test
    - Includes detailed logging for fetched transactions

- **Cryptographic Infrastructure** (Primary: [07_receipt_signature.md](./07_receipt_signature.md))
  - *Purpose*: Documentation of receipt creation and signature gathering processes
  - *Contains*: Cryptographic receipts, signature gathering, verification, Merkle path
  - *Primary Code*: `internal/core/healing/synthetic.go`, `internal/core/healing/anchors.go`
  - *Key Components*:
    - Merkle receipt construction
    - Multi-part receipt combining
    - Signature validation
    - Chain-specific receipt building
  - *Advanced Topics*:
    - Merkle path verification
    - Multi-signature aggregation
    - Version-specific receipt formats
    - Cryptographic proof validation

- **Error Handling** (Primary: [09_error_handling.md](./09_error_handling.md))
  - *Purpose*: Documentation of error handling mechanisms and patterns
  - *Contains*: Structured errors, error wrapping, graceful degradation, retry logic
  - *Primary Code*: `pkg/errors/errors.go`, `tools/cmd/debug/heal_common.go`
  - *Key Patterns*:
    - Structured error types with context
    - Error wrapping for call stack preservation
    - Graceful degradation strategies
    - Multi-tiered retry logic:
      - Peer discovery with 5 retry attempts and increasing timeouts
      - Exponential backoff between retry attempts (1s, 2s, 4s, 8s)
      - Transaction submission with 3 retry attempts
      - Detailed logging of all retry attempts
    - Error classification and recovery
  - *Critical Errors*:
    - "Key not found" errors in transaction fetching
    - Network partition errors
    - Signature validation failures
    - Timeout and rate limiting errors
  - *Advanced Error Handling*:
    - Peer rotation when a peer fails to respond
    - Context cancellation handling
    - Client error vs. server error distinction
    - Response age validation (via `flagMaxResponseAge`)

- **Testing and Verification** (Primary: [10_testing_verification.md](./10_testing_verification.md))
  - *Purpose*: Documentation of testing and verification mechanisms
  - *Contains*: Unit tests, integration tests, reference tests, data integrity rules
  - *Primary Code*: `internal/core/healing/anchor_synth_report_test.go`
  - *Critical For*: Data integrity, on-demand transaction fetching
  - *Test Categories*:
    - Unit tests for individual components
    - Integration tests for system interaction
    - Reference tests for implementation standards
    - Simulation tests for network conditions
  - **Reference Implementation**: `anchor_synth_report_test.go`
    - Definitive standard for on-demand transaction fetching
    - Error interception patterns
    - Fallback mechanism implementation
    - Data integrity preservation techniques

- **Reporting System** (Primary: [08_reporting_system.md](./08_reporting_system.md))
  - *Purpose*: Documentation of the reporting system for monitoring healing progress
  - *Contains*: Progress reports, summary reports, partition pair statistics, ASCII formatting
  - *Primary Code*: `tools/cmd/debug/heal_common.go`
  - *Key Features*:
    - Real-time progress tracking
    - Partition pair statistics
    - Success/failure metrics
    - ASCII-formatted tables for terminal output
    - Configurable reporting intervals
  - *Configuration Options*:
    - `--report-interval`: Interval for detailed reports (default: 5 minutes)

## Implementation Issues and Recommendations

- **Implementation Issues** (Primary: [11_implementation_issues.md](./11_implementation_issues.md))
  - *Purpose*: Documentation of potential issues and recommendations for improvement
  - *Contains*: Observations, recommendations, critical rules enforcement
  - *Primary Code*: Various files in `tools/cmd/debug` and `internal/core/healing`
  - *Key Issues*:
    - Reference test code alignment
    - Error handling consistency
    - Data integrity preservation
    - Cache size limitations
    - Version-specific logic maintenance
    - Retry logic limitations
  - *Critical Recommendations*:
    - Follow reference test code exactly
    - Never fabricate unavailable data
    - Implement proper fallback mechanisms
    - Add detailed logging for on-demand fetching

## Technical Infrastructure Code Reference

- **Common Infrastructure** (Primary: `tools/cmd/debug/heal_common.go`)
  - *Purpose*: Provides shared functionality for healing command-line tools
  - *Key Components*:
    - Light client database configuration
    - LRU cache implementation (1000 entries)
    - Transaction map for efficient lookups
    - Reporting infrastructure with configurable intervals
    - On-demand transaction fetching mechanism
  - **Critical Implementation**: Follows reference test code exactly
  - *Contains*: Database access, caching mechanisms, reporting infrastructure
  - *Related to*: [06_database_caching.md](./06_database_caching.md), [08_reporting_system.md](./08_reporting_system.md)

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

## Common AI Queries

- **How is on-demand transaction fetching implemented?**
  See [06_database_caching.md](./06_database_caching.md) for the caching mechanism and [internal/core/healing/anchor_synth_report_test.go](../../../internal/core/healing/anchor_synth_report_test.go) for the reference implementation.

- **What cryptographic operations are used in healing?**
  See [07_receipt_signature.md](./07_receipt_signature.md) for details on Merkle receipts, signature gathering, and verification.

- **How are errors handled in the healing processes?**
  See [09_error_handling.md](./09_error_handling.md) for error handling patterns and [pkg/errors/errors.go](../../../../../pkg/errors/errors.go) for error definitions.

- **What testing mechanisms are in place for healing?**
  See [10_testing_verification.md](./10_testing_verification.md) for testing frameworks and [internal/core/healing/anchor_synth_report_test.go](../../../internal/core/healing/anchor_synth_report_test.go) for the reference implementation.

## Related Documents

- [Main Index](./00_index.md)
- [API Documentation Index](./00a_api_index.md)
- [Healing Processes Index](./00b_healing_processes_index.md)
- [Command-Line Usage Guide](./00d_cli_usage.md)
