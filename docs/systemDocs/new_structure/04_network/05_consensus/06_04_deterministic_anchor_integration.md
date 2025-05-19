# Deterministic Anchor - Part 4: Integration

This document details how the deterministic anchor system integrates with other components of Accumulate.

## Integration with Consensus

The deterministic anchor system integrates with the CometBFT consensus engine:

1. **Block Finalization**: Anchors are generated during block finalization
2. **Consensus Parameters**: Anchor parameters are part of consensus parameters
3. **Validator Agreement**: All validators must agree on anchor generation
4. **Block Extension**: Anchor data is included in block extensions

## Integration with State Management

Anchors are tightly integrated with the state management system:

1. **State Roots**: Anchors include state roots from the state management system
2. **State Versions**: Each anchor corresponds to a specific state version
3. **State Synchronization**: Anchors enable efficient state synchronization
4. **State Verification**: Anchors provide a basis for state verification

## Integration with Network Topology

The anchor system is aware of the network topology:

1. **Chain Relationships**: Anchors reflect the relationships between chains
2. **Routing Rules**: Anchor routing follows network topology rules
3. **Network Changes**: The anchor system adapts to network topology changes
4. **Chain Discovery**: New chains are discovered through the anchor system

## Integration with Transaction Processing

Anchors interact with the transaction processing system:

1. **Synthetic Transactions**: Anchors are transmitted as synthetic transactions
2. **Transaction Validation**: Anchor transactions undergo special validation
3. **Transaction Execution**: Anchor execution updates the anchor ledger
4. **Transaction Ordering**: Anchor transactions have special ordering rules

## Integration with API Layer

The anchor system is exposed through the API layer:

1. **Query APIs**: APIs for querying anchors and their status
2. **Verification APIs**: APIs for verifying data against anchors
3. **Monitoring APIs**: APIs for monitoring anchor generation and propagation
4. **Management APIs**: APIs for managing anchor parameters (restricted to governance)

## Operational Considerations

Several operational aspects are important for the anchor system:

1. **Monitoring**: Monitoring anchor generation and propagation
2. **Alerting**: Alerting on anchor failures or delays
3. **Recovery**: Procedures for recovering from anchor failures
4. **Performance**: Measuring and optimizing anchor performance
5. **Security**: Protecting the integrity of the anchor system
