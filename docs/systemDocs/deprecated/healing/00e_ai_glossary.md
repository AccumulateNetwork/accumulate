# Accumulate Healing Glossary

<!-- AI-METADATA
type: glossary
version: 1.0
topic: healing_terminology
subtopics: ["synthetic_healing", "anchor_healing", "cryptography", "api"]
related_code: ["internal/core/healing/synthetic.go", "internal/core/healing/anchors.go"]
tags: ["healing", "glossary", "terminology", "ai_optimized"]
-->

## Overview

This glossary provides standardized definitions for terminology used throughout the Accumulate healing documentation. It is structured to be easily parsed by AI systems, with each term having a consistent format and cross-references to related concepts and code.

## Terminology

### Core Concepts

<dl id="glossary-core">
  <dt id="term-healing">Healing</dt>
  <dd>
    <p>The process of detecting and repairing inconsistencies in the Accumulate distributed ledger, ensuring data consistency across multiple partitions without destructive operations.</p>
    <p><strong>Related concepts:</strong> <a href="#term-synthetic-healing">Synthetic Healing</a>, <a href="#term-anchor-healing">Anchor Healing</a></p>
    <p><strong>Primary code:</strong> <code>internal/core/healing/healing.go</code></p>
  </dd>

  <dt id="term-synthetic-healing">Synthetic Healing</dt>
  <dd>
    <p>The process of detecting and repairing missing synthetic transactions between partitions, ensuring cross-partition transaction consistency.</p>
    <p><strong>Related concepts:</strong> <a href="#term-healing">Healing</a>, <a href="#term-transaction-discovery">Transaction Discovery</a></p>
    <p><strong>Primary code:</strong> <code>internal/core/healing/synthetic.go</code></p>
  </dd>

  <dt id="term-anchor-healing">Anchor Healing</dt>
  <dd>
    <p>The process of detecting and repairing missing anchors between partitions, ensuring cross-partition anchor consistency and cryptographic validation.</p>
    <p><strong>Related concepts:</strong> <a href="#term-healing">Healing</a>, <a href="#term-signature-collection">Signature Collection</a></p>
    <p><strong>Primary code:</strong> <code>internal/core/healing/anchors.go</code></p>
  </dd>

  <dt id="term-on-demand-transaction-fetching">On-Demand Transaction Fetching</dt>
  <dd>
    <p>The mechanism for retrieving specific transactions from the network only when needed, using sequence numbers for targeted fetching and never fabricating unavailable data.</p>
    <p><strong>Related concepts:</strong> <a href="#term-transaction-discovery">Transaction Discovery</a>, <a href="#term-data-integrity">Data Integrity</a></p>
    <p><strong>Reference implementation:</strong> <code>internal/core/healing/anchor_synth_report_test.go</code></p>
  </dd>

  <dt id="term-data-integrity">Data Integrity</dt>
  <dd>
    <p>The critical rule that data retrieved from the Protocol cannot be faked or fabricated, as doing so masks errors and leads to monitoring issues.</p>
    <p><strong>Related concepts:</strong> <a href="#term-on-demand-transaction-fetching">On-Demand Transaction Fetching</a></p>
    <p><strong>Reference implementation:</strong> <code>internal/core/healing/anchor_synth_report_test.go</code></p>
  </dd>
</dl>

### Architecture Components

<dl id="glossary-architecture">
  <dt id="term-directory-network">Directory Network (DN)</dt>
  <dd>
    <p>The central partition in Accumulate that maintains the global state and coordinates between multiple Blockchain Validation Networks.</p>
    <p><strong>Related concepts:</strong> <a href="#term-blockchain-validation-network">Blockchain Validation Network</a>, <a href="#term-partition">Partition</a></p>
  </dd>

  <dt id="term-blockchain-validation-network">Blockchain Validation Network (BVN)</dt>
  <dd>
    <p>A partition in Accumulate that validates and processes transactions for a subset of accounts.</p>
    <p><strong>Related concepts:</strong> <a href="#term-directory-network">Directory Network</a>, <a href="#term-partition">Partition</a></p>
  </dd>

  <dt id="term-partition">Partition</dt>
  <dd>
    <p>A logical division of the Accumulate network, either a Directory Network or a Blockchain Validation Network, each maintaining its own blockchain.</p>
    <p><strong>Related concepts:</strong> <a href="#term-directory-network">Directory Network</a>, <a href="#term-blockchain-validation-network">Blockchain Validation Network</a></p>
  </dd>

  <dt id="term-partition-pair">Partition Pair</dt>
  <dd>
    <p>A source and destination partition between which healing operations are performed, typically in the format "source:destination".</p>
    <p><strong>Related concepts:</strong> <a href="#term-partition">Partition</a>, <a href="#term-healing">Healing</a></p>
  </dd>
</dl>

### Technical Processes

<dl id="glossary-processes">
  <dt id="term-transaction-discovery">Transaction Discovery</dt>
  <dd>
    <p>The process of identifying missing transactions between partitions, typically using a binary search algorithm to efficiently find gaps in transaction sequences.</p>
    <p><strong>Related concepts:</strong> <a href="#term-synthetic-healing">Synthetic Healing</a>, <a href="#term-on-demand-transaction-fetching">On-Demand Transaction Fetching</a></p>
    <p><strong>Primary code:</strong> <code>internal/core/healing/synthetic.go:discoverMissingTransactions</code></p>
  </dd>

  <dt id="term-signature-collection">Signature Collection</dt>
  <dd>
    <p>The process of gathering cryptographic signatures from validators to verify the authenticity of anchors between partitions.</p>
    <p><strong>Related concepts:</strong> <a href="#term-anchor-healing">Anchor Healing</a>, <a href="#term-cryptographic-validation">Cryptographic Validation</a></p>
    <p><strong>Primary code:</strong> <code>internal/core/healing/anchors.go:collectSignatures</code></p>
  </dd>

  <dt id="term-cryptographic-validation">Cryptographic Validation</dt>
  <dd>
    <p>The process of verifying the cryptographic integrity of transactions and anchors using digital signatures and Merkle proofs.</p>
    <p><strong>Related concepts:</strong> <a href="#term-signature-collection">Signature Collection</a>, <a href="#term-merkle-receipt">Merkle Receipt</a></p>
  </dd>

  <dt id="term-merkle-receipt">Merkle Receipt</dt>
  <dd>
    <p>A cryptographic proof that verifies the inclusion of a transaction or anchor in a blockchain, constructed using a Merkle tree.</p>
    <p><strong>Related concepts:</strong> <a href="#term-cryptographic-validation">Cryptographic Validation</a></p>
    <p><strong>Primary code:</strong> <code>internal/core/healing/synthetic.go:buildReceipt</code></p>
  </dd>
</dl>

### API and Infrastructure

<dl id="glossary-api">
  <dt id="term-api-layers">API Layers</dt>
  <dd>
    <p>The different API versions used in Accumulate, including Tendermint APIs, v1 APIs, v2 APIs, and v3 APIs, each with different capabilities and purposes.</p>
    <p><strong>Related concepts:</strong> <a href="#term-scope-property">Scope Property</a></p>
    <p><strong>Primary code:</strong> <code>pkg/api/v3/api.go</code>, <code>pkg/client/api/v2/query.go</code></p>
  </dd>

  <dt id="term-scope-property">Scope Property</dt>
  <dd>
    <p>A property used in API queries to specify the partition context for the query, enabling partition-specific operations.</p>
    <p><strong>Related concepts:</strong> <a href="#term-api-layers">API Layers</a>, <a href="#term-partition">Partition</a></p>
  </dd>

  <dt id="term-light-client-database">Light Client Database</dt>
  <dd>
    <p>A database used by healing processes to efficiently access blockchain data without maintaining a full node.</p>
    <p><strong>Related concepts:</strong> <a href="#term-lru-cache">LRU Cache</a>, <a href="#term-transaction-map">Transaction Map</a></p>
    <p><strong>Primary code:</strong> <code>tools/cmd/debug/heal_common.go</code></p>
  </dd>

  <dt id="term-lru-cache">LRU Cache</dt>
  <dd>
    <p>A fixed-size (1000 entries) cache that stores recently accessed transactions using a Least Recently Used eviction policy.</p>
    <p><strong>Related concepts:</strong> <a href="#term-light-client-database">Light Client Database</a>, <a href="#term-transaction-map">Transaction Map</a></p>
    <p><strong>Primary code:</strong> <code>tools/cmd/debug/heal_common.go</code></p>
  </dd>

  <dt id="term-transaction-map">Transaction Map</dt>
  <dd>
    <p>A hash-based lookup table for efficient transaction retrieval during healing operations.</p>
    <p><strong>Related concepts:</strong> <a href="#term-light-client-database">Light Client Database</a>, <a href="#term-lru-cache">LRU Cache</a></p>
    <p><strong>Primary code:</strong> <code>tools/cmd/debug/heal_common.go</code></p>
  </dd>
</dl>

## Usage for AI Systems

AI systems can use this glossary to:

1. **Understand domain-specific terminology** - Each term has a clear definition and context
2. **Navigate related concepts** - Cross-references help build a semantic understanding
3. **Locate relevant code** - Direct links to implementation files
4. **Resolve ambiguities** - Standardized definitions reduce confusion

When processing queries about Accumulate healing, AI systems should reference this glossary to ensure accurate and consistent terminology usage.
