# Accumulate Healing Documentation

## Overview

This directory contains comprehensive documentation about the healing processes in Accumulate. The documentation is divided into focused topics to make it easier for both humans and AI systems to understand the implementation details.

## Important Note on Documentation Location

**CRITICAL**: All healing documentation MUST be kept under the `tools/cmd/debug/docs/healing` directory. This ensures that healing-related documentation is properly organized and associated with the debug tools that implement the healing functionality.

## Table of Contents

1. [Healing Problem Definition](01_healing_problem.md) - Overview of the healing problem in Accumulate
2. [Healing APIs Overview](02_healing_apis.md) - Summary of the healing APIs and their purpose
3. [Synthetic Transaction Healing](03_synthetic_healing.md) - Details of the synthetic transaction healing process
4. [Anchor Healing](04_anchor_healing.md) - Details of the anchor healing process
5. [API Layers](05_api_layers.md) - Information about the various API layers (Tendermint, v1, v2, v3)
6. [Database and Caching](06_database_caching.md) - Database interactions and caching mechanisms
7. [Receipt and Signature](07_receipt_signature.md) - Cryptographic receipt creation and signature gathering
8. [Reporting System](08_reporting_system.md) - Reporting infrastructure and metrics
9. [Error Handling](09_error_handling.md) - Error handling and retry mechanisms
10. [Testing and Verification](10_testing_verification.md) - Testing and verification procedures
11. [Implementation Issues](11_implementation_issues.md) - Potential issues and recommendations for improvement
12. [Healing API Comparison](healing_api_comparison.md) - Comparison between healing APIs and their implementation
13. [Healing Processes Technical Documentation](healing_processes.md) - Technical details of healing processes

## Purpose

This documentation is designed to provide a complete understanding of the healing processes in Accumulate, including both the high-level concepts and the low-level implementation details. It is intended to be useful for:

1. **Developers** working on the Accumulate codebase
2. **AI Systems** analyzing or working with the code
3. **Operators** running Accumulate networks

## Recent Changes

The documentation includes information about recent changes to the healing processes, including:

1. Improvements to the reporting system
2. Fixes to ensure all valid partition pairs are included in reports
3. Updates to use ASCII characters for better terminal compatibility
4. Enhancements to timestamp formatting for readability
5. Documentation moved to the correct location under tools/cmd/debug/docs/healing
6. Implementation of on-demand transaction fetching for the anchor healing process

## How to Use This Documentation

For a comprehensive understanding of the healing processes, it is recommended to read the documents in order. However, each document is designed to be self-contained, so you can also jump directly to the topic you're interested in.

For AI systems, each document includes code snippets and references to the actual implementation, making it easier to understand how the concepts are applied in practice.

## Critical Rules and Best Practices

1. **Data Integrity Rule**: Data retrieved from the Protocol CANNOT be faked. Doing so masks errors and leads to huge wastes of time for those monitoring the Network. The only acceptable implementation is to follow exactly what's in the reference test code (`anchor_synth_report_test.go`). Any data collection must match the test implementation precisely, including fallback mechanisms, but no additional data fabrication should be added.

2. **On-Demand Transaction Fetching**: When implementing on-demand transaction fetching for the anchor healing process, we must ensure we follow the reference test code in `anchor_synth_report_test.go` exactly as specified. The implementation must:
   - Intercept "key not found" errors when attempting to retrieve transaction data
   - Use the sequence number to fetch specific transactions from the network only when needed
   - Not fabricate or fake any data that isn't available from the network
   - Implement proper fallback mechanisms as defined in the reference test code
   - Add detailed logging to track when transactions are fetched on-demand

3. **Healing Process Configuration**: The healing process for synthetic transactions is run with the following flags:
   - `--since`: How far back in time to heal (default: 48 hours, 0 for forever)
   - `--max-response-age`: Set to 336 hours (2 weeks) to allow for longer periods of data retrieval
   - `--wait`: Flag to wait for transactions (enabled by default for heal-synth)

4. **Documentation Location**: All healing documentation MUST be kept under the `tools/cmd/debug/docs/healing` directory.
