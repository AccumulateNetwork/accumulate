# Accumulate Healing Processes Documentation Index

<!-- AI-METADATA
type: index
version: 1.0
topic: healing_processes
subtopics: ["synthetic_healing", "anchor_healing", "on_demand_transaction_fetching"]
related_code: ["internal/core/healing/synthetic.go", "internal/core/healing/anchors.go", "internal/core/healing/anchor_synth_report_test.go"]
critical_rules: ["never_fabricate_data", "follow_reference_implementation", "use_sequence_numbers_for_fetching"]
tags: ["healing", "processes", "documentation", "index", "ai_optimized"]
-->

## Overview

This index provides a comprehensive guide to the healing processes implemented in Accumulate. The healing processes are sophisticated mechanisms that maintain data consistency across the distributed multi-partition network architecture. This document focuses specifically on the synthetic transaction healing and anchor healing processes.

## Healing Processes Documentation

- **Synthetic Transaction Healing** (Primary: [03_synthetic_healing.md](./03_synthetic_healing.md))
  - *Purpose*: Detailed documentation of the synthetic transaction healing process
  - *Contains*: Process flow, implementation details, transaction discovery and fetching
  - *Primary Code*: `internal/core/healing/synthetic.go`
  - *Technical Details*: [healing_processes.md](./healing_processes.md)
  - *Key Algorithms*:
    - Binary search for missing transactions
    - Receipt building for transaction validation
    - Transaction submission with retry logic
    - Version-specific logic (V1 vs V2)
  - *Configuration Options*:
    - `--since`: Time range for healing (default: 48 hours, 0 for forever)
    - `--max-response-age`: Data retrieval window (336 hours/2 weeks)
    - `--wait`: Wait for transaction delivery (default: enabled)

- **Anchor Healing** (Primary: [04_anchor_healing.md](./04_anchor_healing.md))
  - *Purpose*: Detailed documentation of the anchor healing process
  - *Contains*: Process flow, implementation details, signature collection and validation
  - *Primary Code*: `internal/core/healing/anchors.go`
  - *Technical Details*: [healing_processes.md](./healing_processes.md)
  - *Key Algorithms*:
    - Anchor path validation (DN→BVN and BVN→DN only)
    - Signature collection from validators
    - Cryptographic verification of signatures
    - Version-specific healing strategies
    - Fallback mechanisms for different network conditions
  - *Critical Paths*:
    - DN self-anchor verification
    - Validator signature gathering
    - Cryptographic validation

- **Technical Implementation Details** (Primary: [healing_processes.md](./healing_processes.md))
  - *Purpose*: Technical details of healing processes implementation
  - *Contains*: Algorithms, data structures, implementation patterns
  - *Supplements*: [03_synthetic_healing.md](./03_synthetic_healing.md), [04_anchor_healing.md](./04_anchor_healing.md)
  - *Advanced Topics*:
    - State machine implementation
    - Concurrency control mechanisms
    - Memory management strategies
    - Protocol version compatibility
    - Performance optimization techniques

## Core Implementation Reference

- **Synthetic Transaction Implementation** (Primary: `internal/core/healing/synthetic.go`)
  - *Purpose*: Implements the synthetic transaction healing process
  - *Key Functions*: 
    - `HealSynthetic`: Main entry point for synthetic healing
    - `discoverMissingTransactions`: Finds gaps in transaction sequences
    - `fetchTransaction`: Retrieves transactions from the network
    - `buildReceipt`: Creates cryptographic proof for transactions
  - *Critical Patterns*: On-demand transaction fetching, error interception
  - *Related to*: [03_synthetic_healing.md](./03_synthetic_healing.md)

- **Anchor Healing Implementation** (Primary: `internal/core/healing/anchors.go`)
  - *Purpose*: Implements the anchor healing process between partitions
  - *Key Functions*:
    - `HealAnchors`: Main entry point for anchor healing
    - `collectSignatures`: Gathers validator signatures
    - `verifySignatures`: Validates cryptographic signatures
    - `processAnchor`: Handles anchor data between partitions
  - *Critical Paths*: DN→BVN and BVN→DN anchor validation
  - *Related to*: [04_anchor_healing.md](./04_anchor_healing.md)

- **Reference Test Implementation** (Primary: `internal/core/healing/anchor_synth_report_test.go`)
  - *Purpose*: Definitive reference implementation for on-demand transaction fetching
  - *Critical Patterns*:
    - "Key not found" error interception
    - Sequence-based transaction fetching
    - Fallback mechanism implementation
    - Data integrity preservation
    - No data fabrication
  - **CRITICAL**: This is the canonical implementation that MUST be followed exactly
  - *Related to*: [10_testing_verification.md](./10_testing_verification.md), [11_implementation_issues.md](./11_implementation_issues.md)

## Critical Rules for Healing Processes

### On-Demand Transaction Fetching
- **Implementation Requirements**:
  - *Error Handling*: Intercept "key not found" errors when attempting to retrieve transaction data
  - *Efficiency*: Use the sequence number to fetch specific transactions from the network only when needed
  - *Data Integrity*: Not fabricate or fake any data that isn't available from the network
  - *Fallback Mechanisms*: Implement exactly as defined in the reference test code
  - *Observability*: Add detailed logging to track when transactions are fetched on-demand
  - *Reference Implementation*: `anchor_synth_report_test.go`
  - *Related Documents*: [06_database_caching.md](./06_database_caching.md), [11_implementation_issues.md](./11_implementation_issues.md)

### Data Integrity
- **Primary Rule**: Data retrieved from the Protocol CANNOT be faked
  - *Rationale*: Doing so masks errors and leads to huge wastes of time for those monitoring the Network
  - *Implementation Standard*: The only acceptable implementation is to follow exactly what's in the reference test code (`anchor_synth_report_test.go`)
  - *Requirements*: Any data collection must match the test implementation precisely, including fallback mechanisms
  - *Prohibition*: No additional data fabrication should be added
  - *Related Documents*: [10_testing_verification.md](./10_testing_verification.md), [11_implementation_issues.md](./11_implementation_issues.md)

## Common AI Queries

- **How does synthetic transaction healing work?**
  See [03_synthetic_healing.md](./03_synthetic_healing.md) for the process flow and [internal/core/healing/synthetic.go](../../../internal/core/healing/synthetic.go) for implementation details.

- **What is the anchor healing process?**
  See [04_anchor_healing.md](./04_anchor_healing.md) for the process flow and [internal/core/healing/anchors.go](../../../internal/core/healing/anchors.go) for implementation details.

- **How is on-demand transaction fetching implemented?**
  See [internal/core/healing/anchor_synth_report_test.go](../../../internal/core/healing/anchor_synth_report_test.go) for the reference implementation and [11_implementation_issues.md](./11_implementation_issues.md) for critical rules.

- **What are the critical rules for healing processes?**
  The most critical rule is to never fabricate data that isn't available from the network. All implementations must follow the reference test code exactly.

## Related Documents

- [Main Index](./00_index.md)
- [API Documentation Index](./00a_api_index.md)
- [Technical Infrastructure Index](./00c_technical_index.md)
- [Command-Line Usage Guide](./00d_cli_usage.md)
