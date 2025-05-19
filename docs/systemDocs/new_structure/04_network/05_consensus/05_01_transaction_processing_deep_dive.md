# Transaction Processing Deep Dive - Part 1: Overview

This document provides a detailed examination of transaction processing in Accumulate with CometBFT integration.

## Transaction Flow Overview

The transaction processing flow in Accumulate involves several stages:

1. **Submission**: Transactions are submitted to the network via API endpoints
2. **Validation**: Initial validation occurs during CheckTx
3. **Consensus**: Transactions are ordered and agreed upon by validators
4. **Execution**: Transactions are executed during DeliverTx
5. **Commitment**: Results are committed to the blockchain state
6. **Anchoring**: State changes are anchored across chains

## ABCI Integration

Accumulate integrates with CometBFT through the ABCI (Application Blockchain Interface):

- **CheckTx**: Performs initial validation before adding to the mempool
- **DeliverTx**: Executes transactions after consensus
- **Commit**: Finalizes state changes and creates new blocks
- **Query**: Provides read access to blockchain state

## Transaction Types

Accumulate supports various transaction types, each with specific processing requirements:

- **Account Transactions**: Operations on accounts (create, update, etc.)
- **Token Transactions**: Token transfers and management
- **Data Transactions**: Data storage and retrieval
- **Synthetic Transactions**: System-generated transactions for cross-chain operations
- **Anchor Transactions**: Transactions that anchor state between chains

## Processing Contexts

Transaction processing occurs in different contexts:

- **Local Processing**: Initial validation and execution
- **Consensus Processing**: Ordered execution across validators
- **Cross-Chain Processing**: Coordination between BVNs and the DN

## Next Sections

- [Part 2: Validation Process](./05_02_validation_process.md)
- [Part 3: Execution Engine](./05_03_execution_engine.md)
- [Part 4: State Management](./05_04_state_management.md)
