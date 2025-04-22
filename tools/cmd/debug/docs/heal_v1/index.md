# Accumulate Network Healing Documentation

## Overview

This document serves as an index to the detailed documentation on the healing processes in the Accumulate network. The documentation has been organized into focused documents to make it easier to find specific information.

## What is Healing? {#what-is-healing}

Healing is the process of ensuring that transactions are properly delivered across partitions in the Accumulate network. When a transaction needs to be processed by multiple partitions (e.g., a transaction originating in one BVN that affects an account in another BVN), the network uses anchors and synthetic transactions to ensure consistency across partitions.

Sometimes these cross-partition transactions can fail to be delivered due to network issues or other problems. The healing process identifies these failed deliveries and resubmits the necessary transactions to ensure they are properly processed.

## Documentation Structure {#documentation-structure}

The healing documentation is organized into the following documents:

1. [**Developer's Guide**](./developer_guide.md): A practical, code-focused guide for developers who need to implement, modify, or debug healing code.
   - [Quick Start for Developers](./developer_guide.md#quick-start-for-developers)
   - [Code Generation Templates](./developer_guide.md#code-generation-templates)
   - [Critical Implementation Details](./developer_guide.md#critical-implementation-details)

2. [**Healing Overview**](./overview.md): A high-level overview of the two healing approaches and their key characteristics.
   - [Two Approaches to Healing](./overview.md#two-approaches-to-healing)
   - [Common Components](./overview.md#common-components)
   - [Choosing an Approach](./overview.md#choosing-an-approach)

3. [**Chains and URLs**](./chains_urls.md): Detailed explanation of chain types and URL construction in the Accumulate network.
   - [Root Chains](./chains_urls.md#root-chains)
   - [Sequence Chains](./chains_urls.md#sequence-chains)
   - [URL Construction](./chains_urls.md#url-construction)

4. [**Transaction Creation**](./transactions.md): Detailed explanation of how transactions are created in the `heal_anchor` and `heal_synth` approaches, including shared code components and key differences.
   - [Shared Code Components](./transactions.md#shared-code-components)
   - [heal_anchor Transaction Creation](./transactions.md#heal_anchor-transaction-creation)
   - [heal_synth Transaction Creation](./transactions.md#heal_synth-transaction-creation)
   - [Key Differences Between Approaches](./transactions.md#key-differences-between-approaches)

5. [**Database Requirements**](./database.md): Documentation of the database requirements for healing, including interface definitions and implementation guidelines.
   - [Database Usage in Healing Approaches](./database.md#database-usage-in-healing-approaches)
   - [Database Interface Requirements](./database.md#database-interface-requirements)
   - [Current Database Implementation](./database.md#current-database-implementation)
   - [In-Memory Database Implementation](./database.md#in-memory-database-implementation)

6. [**Light Client Implementation**](./light_client.md): Details on the Light client implementation and its usage as infrastructure for the `heal_synth` approach.
   - [Light Client Overview](./light_client.md#light-client-overview)
   - [Light Client Initialization](./light_client.md#light-client-initialization)
   - [Light Client Usage in Synthetic Transaction Healing](./light_client.md#light-client-usage-in-synthetic-healing)
   - [Light Client Interfaces](./light_client.md#light-client-interfaces)

7. [**Implementation Guidelines**](./implementation.md): Practical guidelines and best practices for implementing and modifying the healing code.
   - [Shared Components](./implementation.md#shared-components)
   - [URL Construction](./implementation.md#url-construction)
   - [Error Handling](./implementation.md#error-handling)
   - [Caching](./implementation.md#caching)
   - [Version-Specific Code](./implementation.md#version-specific-code)

8. [**Error Handling Issues**](./issues.md): Analysis of error handling patterns in the healing code and recommendations for improvement.
   - [Error Handling Patterns](./issues.md#error-handling-patterns)
   - [Specific Issues](./issues.md#specific-issues)
   - [Recommendations](./issues.md#recommendations)

9. [**AI Assistant Guide**](./ai_guide.md): Special documentation to help AI assistants effectively navigate and utilize this documentation.
   - [Context Management for AI Assistants](./ai_guide.md#context-management-for-ai-assistants)
   - [Diagnostic Decision Trees](./ai_guide.md#diagnostic-decision-trees-for-ai-assistants)
   - [Code Pattern Recognition](./ai_guide.md#code-pattern-recognition-guide)
   - [Effective Prompting Guidance](./ai_guide.md#effective-prompting-guidance-for-humans)

## Key Considerations {#key-considerations}

When working with the healing code, keep these key considerations in mind:

### 1. URL Construction {#url-construction}

There is a fundamental difference in how URLs are constructed between different parts of the codebase:
- The anchor healing code constructs URLs using `protocol.PartitionUrl(partitionID).JoinPath(protocol.AnchorPool)` (e.g., `acc://bvn-Apollo.acme/anchors`)
- The sequence command uses raw partition URLs for tracking (e.g., `acc://bvn-Apollo.acme`)

This URL construction is critical for proper anchor healing:
- The code uses `srcUrl.JoinPath(protocol.AnchorPool)` to query for anchor signatures
- The destination URL is passed as a raw partition URL
- Inconsistencies in URL construction can cause "element does not exist" errors

See [Transaction Creation: URL Construction](./transactions.md#url-construction) for more details and [Common Pitfalls](./implementation.md#common-pitfalls) for ways to avoid related issues.

### 2. Database Dependency {#database-dependency}

The two healing approaches have different database requirements:
- `heal_anchor`: Does not require a database and explicitly disables the Light client by setting `lightDb = ""`
- `heal_synth`: Requires a database to build Merkle proofs and uses the Light client

The Light client is infrastructure used by synthetic transaction healing. It is not a healing approach itself.

See [Database Requirements: Database Usage in Healing Approaches](./database.md#database-usage-in-healing-approaches) for more details and [Light Client Implementation](./light_client.md) for information on how the Light client is used as infrastructure.

### 3. Caching System {#caching-system}

A caching system has been implemented for the anchor healing process to reduce redundant network requests. The caching system:

- Stores query results in a map indexed by a key composed of the URL and query type
- Helps avoid repeated queries for the same data, especially for account queries and chain queries
- Reduces "element does not exist" errors by remembering previous query results
- Tracks problematic nodes to avoid querying them for certain types of requests

This caching system is particularly important for the anchor healing process because it:
- Makes multiple queries to different validator nodes to collect signatures
- Needs to check the same accounts and chains multiple times during the healing process
- Must handle network partitions and unreliable nodes gracefully

See [Implementation Guidelines: Caching](./implementation.md#caching) for more details on the caching system implementation and [heal_anchor Transaction Creation](./transactions.md#heal_anchor-transaction-creation) for how it's used in the anchor healing process.

### 4. Version-specific Healing Logic {#version-specific-healing}

The anchor healing process includes version-specific logic to handle different network versions:

- V1 networks: Creates a batch with the transaction and all signatures
- V2 networks: Creates and submits individual `BlockAnchor` messages
- Special handling for Directory Network (DN) to Blockchain Validation Network (BVN) anchors in V2 networks

This version-specific logic ensures compatibility with different network deployments and allows for graceful upgrades.

See [Transaction Creation: Version-specific Healing Logic](./transactions.md#version-specific-healing) for more details.

## Related Topics {#related-topics}

- **Chain Types and URLs**: For detailed information on chain types and URL construction, see [Chains and URLs](./chains_urls.md).
- **URL Handling**: For more information on URL construction and handling, see [URL Construction](./transactions.md#url-construction) and [Common Pitfalls](./implementation.md#common-pitfalls).
- **Transaction Processing**: For details on how transactions are processed, see [Transaction Creation](./transactions.md) and [heal_anchor Transaction Creation](./transactions.md#heal_anchor-transaction-creation).
- **Database Usage**: For information on database requirements and implementation, see [Database Requirements](./database.md) and [In-Memory Database Implementation](./database.md#in-memory-database-implementation).
- **Light Client**: For details on the Light client implementation, see [Light Client Implementation](./light_client.md) and [Light Client Usage in Synthetic Transaction Healing](./light_client.md#light-client-usage-in-synthetic-healing).

## Next Steps {#next-steps}

For detailed information about specific aspects of healing, refer to the linked documents above. If you're implementing or modifying healing code, start with the [Implementation Guidelines](./implementation.md) for best practices and common pitfalls to avoid.
