---
title: Accumulate Digital Identifiers (ADIs)
description: Detailed explanation of ADIs, their structure, and functionality in Accumulate
tags: [ADI, identity, accounts, keys]
created: 2025-05-16
version: 1.0
updated: 2025-05-17
---

> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](./new_structure/02_architecture/02_digital_identifiers.md) for the most up-to-date version.

# Accumulate Digital Identifiers (ADIs)

## Introduction

Accumulate Digital Identifiers (ADIs) are the foundational identity structure in the Accumulate protocol. Unlike traditional blockchain addresses, ADIs serve as hierarchical containers that can hold various account types, key books, and other data structures.

## ADI Structure

An ADI consists of:

1. **URL** - A human-readable identifier (e.g., `acc://example`)
2. **Key Book** - Contains key pages that manage authorization
3. **Accounts** - Various account types that hold tokens or data
4. **Data Accounts** - Store arbitrary data
5. **Custom Tokens** - User-defined tokens

## Key Management

ADIs implement a sophisticated key management system:

### Key Books

A Key Book is a collection of Key Pages that define the authorization structure for an ADI.

### Key Pages

Key Pages contain:
- Public keys
- Signature thresholds
- Key priority levels

This hierarchical key structure allows for flexible security models, from simple single-key authorization to complex multi-signature schemes with different authorization levels.

## Account Types

ADIs can contain various account types:

1. **Token Accounts** - Hold ACME or custom tokens
2. **Data Accounts** - Store arbitrary data
3. **Staking Accounts** - Used for network validation
4. **Custom Token Issuers** - Create and manage custom tokens

## ADI Creation and Management

### Creation Process

1. Submit an ADI creation transaction
2. Specify the ADI URL and initial key book
3. Pay the required fee in ACME
4. Network validates and processes the creation

### Management Operations

- Add/remove accounts
- Update key books and pages
- Delegate authority to other ADIs
- Create sub-ADIs

## Cross-ADI Interactions

ADIs can interact with each other through:

1. **Token transfers** - Sending tokens between accounts
2. **Authorization** - Delegating signing authority
3. **Data references** - Referencing data from other ADIs

## Security Considerations

- Key rotation and recovery mechanisms
- Threshold signatures for critical operations
- Hierarchical security models

## References

- [Accumulate Overview](01_accumulate_overview.md)
- [Anchoring Process](03_anchoring_process.md)
- [Synthetic Transactions](04_synthetic_transactions.md)
