# Basic Concepts in Accumulate

## Metadata
- **Document Type**: User Guide
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Core Protocol, Identity Management, Transactions
- **Tags**: concepts, beginners, fundamentals

## 1. Introduction

This guide introduces the fundamental concepts of the Accumulate blockchain protocol. Understanding these concepts is essential for effectively using Accumulate and building applications on the platform.

## 2. Accumulate Digital Identifiers (ADIs)

Accumulate Digital Identifiers (ADIs) are the foundation of Accumulate's identity system. Unlike traditional blockchain addresses, ADIs provide a hierarchical, human-readable identity structure.

### 2.1 Structure of an ADI

An ADI consists of:
- A unique, human-readable name
- One or more key books for authentication
- Associated data and token accounts

Example ADI URL:
```
acc://myidentity
```

### 2.2 Benefits of ADIs

- **Human-readable**: Easy to remember and share
- **Hierarchical**: Can organize accounts in a logical structure
- **Extensible**: Can add accounts and subidentities as needed
- **Secure**: Separated signing authority from transaction execution

## 3. Key Management

Accumulate uses a sophisticated key management system that separates signing authority from transaction execution.

### 3.1 Key Books

Key books store public keys used to authenticate transactions:

- Multiple keys can be organized in pages
- Each page can have different threshold requirements
- Keys can be rotated without changing the identity

### 3.2 Key Pages

Key pages organize keys within a key book:

- Each page can have different threshold requirements
- Pages can be restricted to specific transaction types
- Keys can be rotated without affecting the entire key book

## 4. Account Types

Accumulate supports various account types, each serving different purposes:

### 4.1 Token Accounts

Store tokens and track balances:
```
acc://myidentity/tokens
```

### 4.2 Data Accounts

Store arbitrary data:
```
acc://myidentity/data
```

### 4.3 Lite Accounts

Simplified accounts that don't require an ADI:
```
acc://ACME/ACME1234...
```

## 5. Transaction Basics

### 5.1 Transaction Structure

Accumulate transactions consist of:
- Header: Contains metadata about the transaction
- Body: Contains the specific operation to perform
- Signatures: Authorizes the transaction

### 5.2 Common Transaction Types

- **Create ADI**: Establishes a new digital identity
- **Create Token Account**: Creates an account to hold tokens
- **Send Tokens**: Transfers tokens between accounts
- **Add Credits**: Adds credits to an account for fees
- **Update Key Page**: Modifies keys in a key page

### 5.3 Transaction Fees

Transactions require credits to execute:
- Credits are purchased with ACME tokens
- Different transaction types require different amounts of credits
- Credits are burned when used

## 6. Anchoring and Validation

### 6.1 Anchoring

Accumulate uses a unique anchoring system:
- Transactions are grouped into blocks
- Blocks are anchored to each other using Merkle trees
- Directory Network (DN) anchors validate Block Validator Network (BVN) anchors

### 6.2 Synthetic Transactions

Synthetic transactions are automatically generated to:
- Propagate state changes across partitions
- Maintain consistency across the network
- Enable cross-chain operations

## 7. Next Steps

Now that you understand the basic concepts of Accumulate, you can:
- [Get started](./01_getting_started.md) with creating your first ADI
- Learn how to use the [CLI](./03_cli_usage.md)
- Explore the [architecture](../02_architecture/01_overview.md) in more detail

## Related Documents

- [Getting Started](./01_getting_started.md)
- [CLI Usage](./03_cli_usage.md)
- [Architecture Overview](../02_architecture/01_overview.md)
