# Accumulate Healing Documentation Index

<!-- AI-METADATA
type: main_index
version: 1.0
topic: healing
subtopics: ["synthetic_healing", "anchor_healing", "transaction_fetching", "data_integrity", "api_layers", "cryptography"]
related_code: ["internal/core/healing/synthetic.go", "internal/core/healing/anchors.go", "internal/core/healing/healing.go"]
critical_rules: ["never_fabricate_data", "follow_reference_implementation", "keep_documentation_in_debug_directory"]
tags: ["healing", "documentation", "index", "ai_optimized"]
-->

## Overview

This is the main index for the Accumulate healing documentation, optimized for both human readers and AI systems. The healing processes in Accumulate are sophisticated mechanisms that maintain data consistency across a distributed multi-partition network architecture. These processes involve complex cryptographic operations, precise state management, and rigorous error handling to ensure the integrity of the distributed ledger.

## AI-Optimized Documentation Structure

To make this documentation more accessible to AI systems, it has been organized into smaller, focused modules with consistent metadata headers and cross-references:

- **[API Documentation Index](./00a_api_index.md)** - Comprehensive guide to the API-related aspects of healing
- **[Healing Processes Index](./00b_healing_processes_index.md)** - Detailed information about synthetic and anchor healing processes
- **[Technical Infrastructure Index](./00c_technical_index.md)** - Documentation of database, cryptography, and error handling components
- **[Command-Line Usage Guide](./00d_cli_usage.md)** - Detailed guide to the healing command-line tools
- **[AI Glossary](./00e_ai_glossary.md)** - Machine-readable glossary with semantic anchors for key concepts
- **[Annotated Code Examples](./00f_annotated_code_examples.md)** - Code snippets with detailed annotations for AI understanding

Each document includes structured metadata, consistent tagging, and sections specifically designed to answer common AI queries.

## Important Note on Documentation Location

**CRITICAL**: All healing documentation MUST be kept under the `tools/cmd/debug/docs/healing` directory. This ensures that healing-related documentation is properly organized and associated with the debug tools that implement the healing functionality.

## Architectural Context

Accumulate's architecture consists of multiple partitions, including the Directory Network (DN) and multiple Blockchain Validation Networks (BVNs). This distributed design creates inherent challenges:

1. **Cross-Partition Consistency**: Transactions and anchors must be consistently delivered across partitions
2. **Cryptographic Validation**: Each cross-partition operation requires cryptographic proof of validity
3. **Fault Tolerance**: The system must handle network partitions, node failures, and message delivery failures
4. **Temporal Consistency**: Operations must be properly sequenced and timestamped across partitions

The healing processes address these challenges by detecting and repairing inconsistencies in a non-destructive, verifiable manner.

## Core Documents

- **Overview and Introduction**: [README.md](./README.md)
- **Problem Definition**: [01_healing_problem.md](./01_healing_problem.md)

## Specialized Documentation Indexes

For detailed information on specific aspects of the healing system, please refer to the following specialized indexes:

1. **[API Documentation Index](./00a_api_index.md)**
   - API layers and versioning
   - Interface definitions
   - Implementation details

2. **[Healing Processes Index](./00b_healing_processes_index.md)**
   - Synthetic transaction healing
   - Anchor healing
   - On-demand transaction fetching

3. **[Technical Infrastructure Index](./00c_technical_index.md)**
   - Database and caching
   - Cryptographic operations
   - Error handling
   - Testing and verification

4. **[Command-Line Usage Guide](./00d_cli_usage.md)**
   - Synthetic healing CLI
   - Anchor healing CLI
   - Usage patterns and examples

## Critical Rules and Best Practices

### Data Integrity
- **Primary Rule**: Data retrieved from the Protocol CANNOT be faked
  - *Rationale*: Doing so masks errors and leads to huge wastes of time for those monitoring the Network
  - *Implementation Standard*: The only acceptable implementation is to follow exactly what's in the reference test code (`anchor_synth_report_test.go`)

### On-Demand Transaction Fetching
- **Implementation Requirements**:
  - Intercept "key not found" errors when attempting to retrieve transaction data
  - Use the sequence number to fetch specific transactions from the network only when needed
  - Not fabricate or fake any data that isn't available from the network
  - Implement proper fallback mechanisms as defined in the reference test code
  - Add detailed logging to track when transactions are fetched on-demand

### Documentation Standards
- **Location Requirement**: All healing documentation MUST be kept under the `tools/cmd/debug/docs/healing` directory

## Common AI Queries

- **What are the healing processes in Accumulate?**
  See [Healing Processes Index](./00b_healing_processes_index.md) for details on synthetic and anchor healing.

- **How is on-demand transaction fetching implemented?**
  See [Technical Infrastructure Index](./00c_technical_index.md) and reference implementation in `anchor_synth_report_test.go`.

- **What are the critical rules for healing?**
  Never fabricate data that isn't available from the network and follow the reference test code exactly.

- **How do I use the healing command-line tools?**
  See [Command-Line Usage Guide](./00d_cli_usage.md) for detailed instructions and examples.



## Summary

This documentation structure has been optimized for AI consumption by:

1. Breaking large files into smaller, focused modules
2. Adding structured metadata headers to each document
3. Implementing consistent tagging and cross-referencing
4. Creating an AI-specific navigation system
5. Adding sections for common AI queries

This approach makes the documentation more accessible to AI systems while maintaining its utility for human developers. The critical rules about data integrity and on-demand transaction fetching are emphasized throughout the documentation to ensure proper implementation.

### Healing Processes

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

### Technical Infrastructure

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

### Operational Aspects

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

- **Error Handling** (Primary: [09_error_handling.md](./09_error_handling.md))
  - *Purpose*: Documentation of error handling mechanisms and patterns
  - *Contains*: Structured errors, error wrapping, graceful degradation, retry logic
  - *Primary Code*: `pkg/errors/errors.go`
  - *Key Patterns*:
    - Structured error types with context
    - Error wrapping for call stack preservation
    - Graceful degradation strategies
    - Retry logic with exponential backoff
    - Error classification and recovery
  - *Critical Errors*:
    - "Key not found" errors in transaction fetching
    - Network partition errors
    - Signature validation failures
    - Timeout and rate limiting errors

### Quality Assurance

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

- **Implementation Issues and Recommendations** (Primary: [11_implementation_issues.md](./11_implementation_issues.md))
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

### Healing Processes
- **Synthetic Transaction Healing** (Primary: [03_synthetic_healing.md](./03_synthetic_healing.md))
  - *Process Flow*: End-to-end healing workflow
  - *Transaction Discovery*: Finding missing transactions
  - *Binary Search Algorithm*: Efficient search for gaps
  - *Receipt Building*: Transaction validation mechanism
  - *Transaction Submission*: Delivery with retry logic
  - *Version-Specific Logic*: V1 vs V2 implementation differences
  - *Configuration Options*: Time ranges, response age, wait flags
  - *Technical implementation details*: [healing_processes.md](./healing_processes.md)
- **Anchor Healing** (Primary: [04_anchor_healing.md](./04_anchor_healing.md))
  - *Process Flow*: End-to-end anchor healing workflow
  - *Anchor Path Validation*: DN→BVN and BVN→DN path rules
  - *Signature Collection*: Gathering validator signatures
  - *Cryptographic Verification*: Validating signature integrity
  - *Version-Specific Strategies*: Protocol version handling
  - *Fallback Mechanisms*: Recovery from network issues
  - *Critical Paths*: DN self-anchor, validator signatures
  - *Technical implementation details*: [healing_processes.md](./healing_processes.md)
- **On-Demand Transaction Fetching** (Primary: [06_database_caching.md](./06_database_caching.md))
  - *Error Interception*: "Key not found" handling
  - *Sequence-Based Fetching*: Using sequence numbers for targeted retrieval
  - *Data Integrity*: Never fabricating unavailable data
  - *Fallback Implementation*: Per reference test code
  - *Logging*: Detailed tracking of fetched transactions
  - *Reference Implementation*: `anchor_synth_report_test.go`
  - *Implementation issues and recommendations*: [11_implementation_issues.md](./11_implementation_issues.md)

### Data Infrastructure
- **Database and Caching** (Primary: [06_database_caching.md](./06_database_caching.md))
  - *Light Client Database*: Efficient chain access mechanism
  - *LRU Cache*: Fixed-size (1000 entries) caching strategy
  - *Transaction Map*: Hash-based transaction lookup
  - *Chain Access*: Efficient blockchain data retrieval
  - *Memory Management*: Optimizing memory usage
  - *Concurrency Control*: Thread-safe data access
  - *Performance Considerations*: Optimizing data access patterns

### Cryptography
- **Cryptographic Receipts and Signatures** (Primary: [07_receipt_signature.md](./07_receipt_signature.md))
  - *Merkle Receipt Construction*: Building cryptographic proofs
  - *Multi-Part Receipt Combining*: Merging proof chains
  - *Signature Validation*: Cryptographic verification
  - *Chain-Specific Receipt Building*: Network-specific proof formats
  - *Merkle Path Verification*: Validating inclusion proofs
  - *Multi-Signature Aggregation*: Combining validator signatures
  - *Version-Specific Receipt Formats*: Protocol version handling
- **Verification Mechanisms** (Primary: [07_receipt_signature.md](./07_receipt_signature.md))
  - *Cryptographic Proof Validation*: Ensuring data integrity
  - *Signature Verification*: Validator signature checking
  - *Chain Integrity Verification*: Ensuring consistent state
  - *Testing aspects*: [10_testing_verification.md](./10_testing_verification.md)

### Operational
- **Reporting System** (Primary: [08_reporting_system.md](./08_reporting_system.md))
  - *Real-Time Progress Tracking*: Monitoring healing operations
  - *Partition Pair Statistics*: Cross-partition metrics
  - *Success/Failure Metrics*: Operational performance tracking
  - *ASCII-Formatted Tables*: Terminal-friendly output
  - *Configurable Reporting Intervals*: Customizable feedback
  - *Command-Line Interface*: User interaction mechanisms
  - *Report Interval Configuration*: `--report-interval` option
- **Error Handling** (Primary: [09_error_handling.md](./09_error_handling.md))
  - *Structured Error Types*: Context-rich error information
  - *Error Wrapping*: Call stack preservation
  - *Graceful Degradation*: Handling partial failures
  - *Retry Logic*: Exponential backoff strategies
  - *Error Classification*: Categorizing error types
  - *Recovery Mechanisms*: Handling transient failures
  - *Critical Error Paths*: Key error handling scenarios

### Quality Assurance
- **Testing and Verification** (Primary: [10_testing_verification.md](./10_testing_verification.md))
  - *Unit Tests*: Component-level verification
  - *Integration Tests*: System interaction testing
  - *Reference Tests*: Implementation standards
  - *Simulation Tests*: Network condition testing
  - *Data Integrity Validation*: Ensuring data correctness
  - *Performance Testing*: Scalability and efficiency
  - *Reference Implementation*: `anchor_synth_report_test.go`
- **Implementation Issues** (Primary: [11_implementation_issues.md](./11_implementation_issues.md))
  - *Reference Test Code Alignment*: Ensuring standard compliance
  - *Error Handling Consistency*: Unified error management
  - *Data Integrity Preservation*: Preventing data fabrication
  - *Cache Size Limitations*: Memory usage considerations
  - *Version-Specific Logic*: Protocol version maintenance
  - *Retry Logic Limitations*: Handling persistent failures
  - *Critical Rules*: Never fabricating data, following reference code

## Code Reference Index

### Core Healing Code
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

- **Core Interfaces and Types** (Primary: `internal/core/healing/healing.go`)
  - *Purpose*: Defines the core interfaces and types for healing processes
  - *Key Interfaces*:
    - `HealSyntheticArgs`: Parameters for synthetic transaction healing
    - `HealAnchorArgs`: Parameters for anchor healing
    - `SequencedInfo`: Metadata for sequenced operations
    - `ResolveSequenced<T>`: Generic resolver for sequenced items
  - *Design Patterns*: Generic programming, dependency injection
  - *Related to*: [02_healing_apis.md](./02_healing_apis.md)

### Command-Line Tools
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

- **Synthetic Healing CLI** (Primary: `tools/cmd/debug/heal_synth.go`)
  - *Purpose*: Command-line interface for synthetic transaction healing
  - *Key Flags*:
    - `--since`: Time range for healing (default: 48 hours, 0 for forever)
    - `--max-response-age`: Data retrieval window (336 hours/2 weeks)
    - `--wait`: Wait for transaction delivery (default: enabled)
  - *Usage Pattern*: `heal-synth [network] [partition-pair] [sequence]`
  - *Related to*: [03_synthetic_healing.md](./03_synthetic_healing.md)

- **Anchor Healing CLI** (Primary: `tools/cmd/debug/heal_anchor.go`)
  - *Purpose*: Command-line interface for anchor healing
  - *Key Flags*:
    - `--since`: Time range for healing
    - `--partition-pair`: Specific partition pair to heal
    - `--sequence`: Starting sequence number
  - *Usage Pattern*: `heal-anchor [network] [options]`
  - *Related to*: [04_anchor_healing.md](./04_anchor_healing.md)

### Testing Code
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

- **Unit Tests** (Primary: `internal/core/healing/synthetic_test.go`)
  - *Purpose*: Tests individual components of the synthetic healing process
  - *Key Test Cases*:
    - Transaction discovery algorithm
    - Receipt building and validation
    - Error handling scenarios
    - Edge cases in transaction processing
  - *Related to*: [10_testing_verification.md](./10_testing_verification.md)

- **Integration Tests** (Primary: `internal/core/healing/integration_test.go`)
  - *Purpose*: Tests interaction between healing components and the network
  - *Key Test Scenarios*:
    - Multi-partition healing
    - Network error handling
    - Version compatibility
    - Performance under load
  - *Related to*: [10_testing_verification.md](./10_testing_verification.md)

### API Code
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

## Command-Line Usage

### Synthetic Transaction Healing

```bash
heal-synth [network] [partition-pair] [sequence] [flags]
```

#### Arguments
- `network`: Name of the network to heal (required)
- `partition-pair`: Optional partition pair in format "source:destination"
- `sequence`: Optional starting sequence number

#### Flags
- `--since=<duration>`: How far back in time to heal (default: 48 hours, 0 for forever)
- `--max-response-age=<duration>`: Set to 336 hours (2 weeks) to allow for longer periods of data retrieval
- `--wait`: Flag to wait for transactions (enabled by default for heal-synth)
- `--report-interval=<duration>`: Interval for detailed progress reports (default: 5 minutes)

#### Examples
```bash
# Heal synthetic transactions for the last 48 hours on mainnet
heal-synth mainnet

# Heal synthetic transactions for all time on testnet
heal-synth testnet --since=0

# Heal synthetic transactions for a specific partition pair
heal-synth devnet bvn1:directory

# Heal synthetic transactions starting from a specific sequence
heal-synth mainnet bvn2:directory 12345
```

### Anchor Healing

```bash
heal-anchor [network] [flags]
```

#### Arguments
- `network`: Name of the network to heal (required)

#### Flags
- `--since=<duration>`: How far back in time to heal
- `--partition-pair=<source:destination>`: Specific partition pair to heal
- `--sequence=<number>`: Starting sequence number
- `--report-interval=<duration>`: Interval for detailed progress reports (default: 5 minutes)

#### Examples
```bash
# Heal anchors for the last 48 hours on mainnet
heal-anchor mainnet

# Heal anchors for a specific partition pair
heal-anchor testnet --partition-pair=directory:bvn1

# Heal anchors starting from a specific sequence
heal-anchor devnet --sequence=5000
```

### Common Patterns

- For routine maintenance, run with default settings (48-hour window)
- For recovery after extended downtime, use `--since=0` to heal all history
- For targeted healing, specify partition pairs and sequence numbers
- For long-running operations, consider adjusting the report interval

## Critical Rules and Best Practices

### Data Integrity
- **Primary Rule**: Data retrieved from the Protocol CANNOT be faked
  - *Rationale*: Doing so masks errors and leads to huge wastes of time for those monitoring the Network
  - *Implementation Standard*: The only acceptable implementation is to follow exactly what's in the reference test code (`anchor_synth_report_test.go`)
  - *Requirements*: Any data collection must match the test implementation precisely, including fallback mechanisms
  - *Prohibition*: No additional data fabrication should be added
  - *Related Documents*: [10_testing_verification.md](./10_testing_verification.md), [11_implementation_issues.md](./11_implementation_issues.md)

### On-Demand Transaction Fetching
- **Implementation Requirements**:
  - *Error Handling*: Intercept "key not found" errors when attempting to retrieve transaction data
  - *Efficiency*: Use the sequence number to fetch specific transactions from the network only when needed
  - *Data Integrity*: Not fabricate or fake any data that isn't available from the network
  - *Fallback Mechanisms*: Implement exactly as defined in the reference test code
  - *Observability*: Add detailed logging to track when transactions are fetched on-demand
  - *Reference Implementation*: `anchor_synth_report_test.go`
  - *Related Documents*: [06_database_caching.md](./06_database_caching.md), [11_implementation_issues.md](./11_implementation_issues.md)

### Documentation Standards
- **Location Requirement**: All healing documentation MUST be kept under the `tools/cmd/debug/docs/healing` directory
  - *Rationale*: Ensures documentation is properly organized and associated with the debug tools

## Command-Line Usage

### Healing Commands
- **Synthetic Healing** (Primary command for healing synthetic transactions)
  ```bash
  go run ./tools/cmd/debug heal-synth [network] [source:destination] [sequence]
  ```
  - *Related Document*: [03_synthetic_healing.md](./03_synthetic_healing.md)

- **Anchor Healing** (Primary command for healing anchors)
  ```bash
  go run ./tools/cmd/debug heal-anchor [network] [source:destination] [sequence]
  ```
  - *Related Document*: [04_anchor_healing.md](./04_anchor_healing.md)

### Configuration Options
- **Time Range Options**:
  - `--since`: How far back in time to heal (default: 48 hours, 0 for forever)
  - `--max-response-age`: Set to 336 hours (2 weeks) to allow for longer periods of data retrieval

- **Process Control**:
  - `--wait`: Flag to wait for transactions (enabled by default for heal-synth)

- **Reporting Options**:
  - `--report-interval`: Interval for printing detailed reports (default: 5 minutes)
  - *Related Document*: [08_reporting_system.md](./08_reporting_system.md)
