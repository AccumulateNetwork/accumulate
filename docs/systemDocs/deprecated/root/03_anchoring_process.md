---
title: Accumulate Anchoring Process
description: Detailed explanation of the anchoring mechanism in Accumulate
tags: [anchoring, security, cross-chain, validation]
created: 2025-05-16
version: 1.0
updated: 2025-05-17
---

> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](./new_structure/02_architecture/03_anchoring_process.md) for the most up-to-date version.

# Accumulate Anchoring Process

## Introduction

Anchoring is a core security mechanism in Accumulate that ensures the integrity and consistency of data across the network's multiple chains. It creates cryptographic links between different parts of the network, establishing a verifiable chain of trust.

## Anchoring Hierarchy

Accumulate implements a hierarchical anchoring system:

1. **BVN to DN Anchoring** - Block Validator Networks anchor to the Directory Network
2. **DN to BVN Anchoring** - Directory Network anchors back to BVNs
3. **External Anchoring** - Anchoring to external blockchains (e.g., Bitcoin, Ethereum)

## Anchor Structure

An anchor contains:

1. **Root Hash** - Merkle root of the chain being anchored
2. **Timestamp** - When the anchor was created
3. **Sequence Information** - Block height or sequence number
4. **Signature** - Cryptographic proof of authenticity

## Anchoring Process

### BVN to DN Anchoring

1. BVN produces a block and calculates its Merkle root
2. BVN creates an anchor transaction containing this root
3. Anchor transaction is submitted to the DN
4. DN validates and includes the anchor in its state
5. DN acknowledges receipt via synthetic transactions

### DN to BVN Anchoring

1. DN aggregates anchors from multiple BVNs
2. DN produces its own block with a Merkle root
3. DN creates anchor transactions to each BVN
4. BVNs validate and include these anchors

### External Anchoring

1. DN aggregates network state into a single Merkle root
2. This root is submitted to external blockchains
3. External transaction IDs are recorded in Accumulate

## Anchor Validation

Anchors are validated through:

1. **Signature Verification** - Ensuring the anchor was created by authorized validators
2. **Chain Verification** - Confirming the anchor fits in the expected sequence
3. **Cross-Reference Verification** - Checking consistency across different chains

## Healing Process

When inconsistencies are detected:

1. **Detection** - Through regular validation or explicit healing commands
2. **Analysis** - Identifying the specific inconsistency
3. **Resolution** - Generating synthetic transactions to restore consistency
4. **Verification** - Confirming the healing was successful

## Security Implications

Anchoring provides:

1. **Tamper Evidence** - Any modification would break the anchor chain
2. **Byzantine Fault Tolerance** - Network can tolerate malicious actors
3. **Cross-Chain Verification** - Enables validation across different parts of the network

## References

- [Accumulate Overview](01_accumulate_overview.md)
- [ADIs](02_accumulate_digital_identifiers.md)
- [Synthetic Transactions](04_synthetic_transactions.md)
