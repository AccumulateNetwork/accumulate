---
title: Accumulate Overview
description: A concise introduction to the Accumulate blockchain protocol
tags: [overview, blockchain, protocol]
created: 2025-05-16
version: 1.0
updated: 2025-05-17
---

> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](./new_structure/02_architecture/01_overview.md) for the most up-to-date version.

# Accumulate Overview

## Introduction

Accumulate is a novel blockchain protocol designed to serve as a bridge between the traditional world of finance and the emerging world of digital assets. It employs a unique hierarchical structure that enables high throughput, scalability, and interoperability while maintaining security and decentralization.

## Core Concepts

### Identity-Based Architecture

Unlike traditional blockchains that are account-based or UTXO-based, Accumulate is identity-based. The fundamental unit in Accumulate is the ADI (Accumulate Digital Identifier), which serves as a container for various types of accounts and other data.

### Multi-Chain Structure

Accumulate employs a multi-chain architecture consisting of:

1. **Directory Network (DN)** - Manages the global state and coordinates between BVNs
2. **Block Validator Networks (BVNs)** - Process transactions and maintain ledgers
3. **Anchoring** - Secures the network through cross-chain validation

### Key-Value Database

Accumulate uses a key-value database to store all protocol data, which enables efficient data retrieval and state management.

## Transaction Processing

Transactions in Accumulate follow a unique flow:

1. Transaction submission to the network
2. Validation by the appropriate BVN
3. Execution and state update
4. Anchoring to secure the transaction

## Consensus Mechanism

Accumulate uses a consensus mechanism that combines elements of:

- Delegated Proof of Stake (DPoS)
- Byzantine Fault Tolerance (BFT)

This hybrid approach allows for high throughput while maintaining security.

## Network Security

Security in Accumulate is maintained through:

1. **Hierarchical Anchoring** - Cross-chain validation
2. **Synthetic Transactions** - System-generated transactions for cross-chain consistency
3. **Key Management** - Sophisticated key hierarchies and signature schemes

## References

- [Accumulate Technical Documentation](https://docs.accumulatenetwork.io/)
- Related documents: 
  - [ADIs](02_accumulate_digital_identifiers.md)
  - [Anchoring Process](03_anchoring_process.md)
  - [Synthetic Transactions](04_synthetic_transactions.md)
