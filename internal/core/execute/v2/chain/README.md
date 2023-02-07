## Chain Validator Design

Chain validators are organized by transaction type. The executor handles mundane
tasks that are common to all chain validators, such as authentication and
authorization.

In general, every transaction requires an origin record. Thus, the executor
validates and loads the origin before delegating to the chain validator.
However, certain transaction types, specifically synthetic transactions that
create records, may not need an extant origin. The executor has a specific
clause for these special cases.

## Chain Validator Implementation

Chain validators must satisfy the `TxExecutor` interface:

```go
type TxExecutor interface {
	Type() protocol.TransactionType
	Validate(*StateManager, *messaging.Envelope) error
}
```

**All state manipulation (mutating and loading) must go through the state
manager.** There are three methods that can be used to modify records and/or
create synthetic transactions:

- Implementing a user transaction executor
  + `Update(record)` - Update one or more existing records. Cannot be used to
    create records.
  + `Create(record)` - Create one or more new records. Produces a synthetic
    chain create transaction.
  + `Submit(url, body)` - Submit a synthetic transaction.
- Implementing a synthetic transaction executor
  + `Update(record)` - Create or update one or more existing records.
  + `Create(record)` - Cannot be used by synthetic transactions.
  + `Submit(url, body)` - Cannot be used by synthetic transactions.
