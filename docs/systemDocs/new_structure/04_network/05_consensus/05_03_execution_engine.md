# Transaction Processing Deep Dive - Part 3: Execution Engine

This document details the execution engine for transactions in Accumulate.

## DeliverTx Execution

After a transaction has been validated and included in a block through consensus, it enters the execution phase during DeliverTx:

1. **Execution Preparation**
   - Load the full transaction context
   - Initialize the execution environment
   - Set up state access and logging

2. **Core Execution**
   - Execute the transaction's operations
   - Update account states
   - Process token transfers
   - Generate events and receipts

3. **Post-Execution**
   - Update sequence numbers
   - Generate synthetic transactions if needed
   - Prepare anchor data
   - Update indices and caches

## Transaction Executor

The transaction executor is responsible for processing transactions:

```go
type Executor struct {
    State       *State
    BlockTime   time.Time
    BlockHeight uint64
    Logger      *zap.Logger
}

func (e *Executor) Execute(tx *protocol.Transaction) (*protocol.TransactionStatus, error) {
    // Initialize execution context
    ctx := NewExecutionContext(e.State, tx, e.BlockTime, e.BlockHeight)
    
    // Execute the transaction
    status, err := ctx.Execute()
    if err != nil {
        return nil, err
    }
    
    // Update state and generate synthetic transactions
    err = e.State.Update(ctx.Changes())
    if err != nil {
        return nil, err
    }
    
    // Return the execution status
    return status, nil
}
```

## Execution Contexts

Different transaction types have specialized execution contexts:

- **Account Transactions**: Manage account state changes
- **Token Transactions**: Handle token transfers and updates
- **Data Transactions**: Process data storage and retrieval
- **Synthetic Transactions**: Execute system-generated operations
- **Anchor Transactions**: Process cross-chain anchoring

## State Management

During execution, the state is managed carefully:

- Changes are accumulated in a batch
- State is only updated after successful execution
- Rollback is possible if errors occur
- State changes are tracked for anchoring

## Concurrency and Parallelism

The execution engine employs concurrency strategies:

- Parallel execution of independent transactions
- Sequential execution of dependent transactions
- Optimistic concurrency control for state access
- Lock-free algorithms where possible
