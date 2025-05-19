# Deterministic Anchor - Part 2: Design Details

This document provides detailed design information for the deterministic anchor system in Accumulate.

## Design Principles

The deterministic anchor system is designed with several key principles:

1. **Determinism**: The same inputs always produce the same outputs
2. **Minimal Coordination**: Nodes should require minimal coordination
3. **Fault Tolerance**: The system should be resilient to node failures
4. **Scalability**: The design should scale with network growth
5. **Verifiability**: Anchors should be easily verifiable

## Anchor Schedule

Anchors are created according to a deterministic schedule:

- **Block Height**: Anchors are created at specific block heights
- **Time-Based**: Anchors may also be created at specific time intervals
- **Hybrid Approach**: A combination of block height and time-based triggers

The schedule is defined by network parameters and can be adjusted through governance.

## Anchor Content

Each anchor contains:

- **State Root**: The Merkle root of the chain's state
- **Block Height**: The height at which the anchor was created
- **Timestamp**: The time at which the anchor was created
- **Chain ID**: The identifier of the source chain
- **Destination**: The identifier of the destination chain
- **Sequence Number**: A monotonically increasing sequence number

## Cross-Chain Routing

Anchors are routed between chains according to a deterministic routing table:

- **BVN to DN**: Business Validation Networks anchor to the Directory Network
- **DN to BVN**: The Directory Network anchors to Business Validation Networks
- **BVN to BVN**: Direct anchoring between Business Validation Networks (optional)

## Conflict Resolution

In case of conflicts or forks, the system includes resolution mechanisms:

- **Majority Rule**: Accept anchors confirmed by a majority of validators
- **Timestamp Ordering**: Resolve conflicts based on anchor timestamps
- **Fork Choice Rules**: Specific rules for choosing between competing chains
- **Recovery Procedures**: Methods for recovering from inconsistent states
