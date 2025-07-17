# Token Account Query API for Lite Client

## Purpose
This API enables users to retrieve token account balances and transaction history through the lite client, after cryptographic state verification (Phase 1). It is designed for use with Accumulate token accounts only.

## Supported Operations
- **Get Balance**: Retrieve the current balance for a token account.
- **Get Transactions**: Retrieve all or the latest N transactions for a token account.
- **Get Balance and Transactions**: Retrieve both balance and transaction history in a single call.

## API Structure (Go)

```
type TokenAccountAPI interface {
    GetBalance(ctx context.Context, accountUrl string) (BalanceResult, error)
    GetTransactions(ctx context.Context, accountUrl string, limit int) ([]TransactionResult, error)
    GetBalanceAndTransactions(ctx context.Context, accountUrl string, limit int) (BalanceResult, []TransactionResult, error)
}

// See api.go for BalanceResult and TransactionResult structs.
```

## Usage Example

```
cl := NewLiteClient(server)
// After proof/caching phase
balance, err := cl.GetBalance(ctx, "acc://alice/tokens")
txs, err := cl.GetTransactions(ctx, "acc://alice/tokens", 10)
bal, txs, err := cl.GetBalanceAndTransactions(ctx, "acc://alice/tokens", 10)
```

## Querying Implementation Plan

### Balance Retrieval
- **v3 (preferred):**
    - Use `api.Querier.Query(ctx, scope, &api.AccountQuery{Url: ...})` where `scope` is the parsed account URL.
    - Parse the returned `*api.AccountRecord` and extract balance, token URL, and height from the embedded `*api.TokenAccount`.
- **v2 (fallback):**
    - Use `RequestAPIv2(ctx, "query", &v2.GeneralQuery{UrlQuery: v2.UrlQuery{Url: ...}}, ...)`.
    - Parse the returned struct for balance and metadata fields (actual field names may differ, see Go code for mapping).
- Only token accounts are supported; other account types will return an error.

### Transaction History Retrieval
- **v3 only:**
    - Query the account's main chain using `api.Querier.Query(ctx, scope, &api.ChainQuery{Name: "main"})`.
    - Parse the returned `*api.ChainRecord` to get transaction entries.
    - For each entry, query transaction details using `api.Querier.Query(ctx, txid, &api.TransactionQuery{TxID: ...})`.
    - Map protocol fields to the `TransactionResult` struct (see Go code for field mapping).
    - If `limit > 0`, fetch only the latest N transactions.

### Combined Retrieval
- Call both balance and transaction queries in a single method for efficiency.

### Error Handling & Fallbacks
- If v3 fails, fallback to v2 for balance only.
- If an account is not a token account, return an error.
- All errors are propagated to the caller with descriptive messages.

## Design Rationale
- All queries are cryptographically anchored by prior proof validation (see Phase 1).
- Only token accounts are supported for now; extension to other account types is possible.
- API is designed for easy integration with CLI, web, or other lite client consumers.

## Next Steps
- Implement the above interface in `api.go` as methods on `LiteClient`.
- Add error handling and logging for network/API failures.
- Support future extension for additional fields (pending transactions, token metadata, etc).
