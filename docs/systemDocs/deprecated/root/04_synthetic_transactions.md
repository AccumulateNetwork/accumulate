---
title: Accumulate Synthetic Transactions
description: Detailed explanation of synthetic transactions in Accumulate, their purpose, and healing process
tags: [synthetic, transactions, healing, anchoring]
created: 2025-05-16
version: 1.0
updated: 2025-05-17
---

> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](./new_structure/02_architecture/04_synthetic_transactions.md) for the most up-to-date version.

# Accumulate Synthetic Transactions

## Introduction

Synthetic transactions are system-generated transactions in the Accumulate protocol that maintain cross-chain consistency and facilitate the anchoring process. Unlike user-initiated transactions, synthetic transactions are created automatically by the network to ensure proper state synchronization across different chains.

## Purpose and Types

Synthetic transactions serve several critical functions:

1. **Anchor Acknowledgments** - Confirm receipt of anchors between chains
2. **State Synchronization** - Ensure consistent state across the network
3. **Chain Healing** - Repair inconsistencies when detected
4. **System Operations** - Perform automated protocol functions

## Synthetic Transaction Flow

1. **Generation** - Created by the protocol in response to specific triggers
2. **Validation** - Verified by validators like normal transactions
3. **Execution** - Updates state according to transaction type
4. **Recording** - Stored in the ledger with special designation

## Anchoring and Synthetic Transactions

Synthetic transactions are integral to the anchoring process:

1. When BVN A anchors to the DN, the DN creates a synthetic acknowledgment
2. When the DN anchors to BVN B, BVN B creates a synthetic acknowledgment
3. These acknowledgments ensure bidirectional verification

## Healing Process

The healing process for synthetic transactions is a critical maintenance operation:

### Healing Command Parameters

- `--since`: Determines how far back in time to heal (default: 48 hours, 0 for forever)
- `--max-response-age`: Set to 336 hours (2 weeks) for longer data retrieval periods
- `--wait`: Flag to wait for transactions (enabled by default for heal-synth)

### Healing Implementation

The healing process follows these principles:

1. **No Data Fabrication** - Data retrieved from the Protocol CANNOT be faked
2. **Reference Implementation** - Follows exactly what's in the reference test code (anchor_synth_report_test.go)
3. **On-Demand Fetching** - Intercepts "key not found" errors to fetch transactions as needed
4. **Proper Fallbacks** - Implements fallback mechanisms as defined in the reference code
5. **Detailed Logging** - Tracks when transactions are fetched on-demand

### Healing Execution

The command is typically run with:
```
heal-synth [network-name] [optional-partition-pair] [optional-sequence-number]
```

## Transaction Verification

Synthetic transactions are verified through:

1. **Origin Verification** - Ensuring they come from authorized validators
2. **Sequence Verification** - Confirming they fit in the expected sequence
3. **Content Verification** - Validating the transaction data

## Security Considerations

- Synthetic transactions cannot be forged by external actors
- They maintain an auditable trail of system operations
- They provide evidence of proper network functioning

## Debugging and Monitoring

Synthetic transactions can be monitored and debugged using:

1. **Transaction Explorer** - Viewing synthetic transactions in the ledger
2. **Healing Tools** - Running healing operations to fix inconsistencies
3. **Debug Commands** - Using commands like `debug sync` for synchronization analysis

## References

- [Accumulate Overview](01_accumulate_overview.md)
- [ADIs](02_accumulate_digital_identifiers.md)
- [Anchoring Process](03_anchoring_process.md)
