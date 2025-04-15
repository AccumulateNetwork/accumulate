# Error Handling Issues in Healing Code

This document analyzes the current state of error handling in the Accumulate healing code, focusing on identifying patterns that could lead to program termination, unhandled errors, or other issues that might prevent the healing process from continuing. The goal is to document these issues so they can be addressed in future updates to make the healing process more robust.

## Error Handling Patterns

The healing code uses several patterns for error handling, each with different implications for code robustness:

### 1. The `check` Function

The most common error handling pattern in the healing code is the `check` function, defined in `main.go`:

```go
func check(err error) {
    if err != nil {
        err = errors.UnknownError.Skip(1).Wrap(err)
        fatalf("%+v", err)
    }
}
```

This function wraps the error with additional context and then calls `fatalf`, which terminates the program. While this provides good error information, it prevents the healing process from continuing after an error.

#### Usage Examples

The `check` function is used extensively throughout the healing code:

- In `heal_common.go` for network initialization, database setup, and API client creation
- In `heal_synth.go` for pulling chains, accounts, and ledgers
- In `heal_anchor.go` for anchor-related operations

**Related Documentation**:
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)
- [heal_synth Transaction Creation](./transactions.md#heal_synth-transaction-creation)

### 2. Direct Panic Calls

Some parts of the code use direct `panic` calls for error handling:

```go
// In main.go
var currentUser = func() *user.User {
    u, err := user.Current()
    if err != nil {
        panic(err)
    }
    return u
}()
```

These panics also terminate the program but provide less context than the `check` function.

**Related Documentation**:
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)

### 3. Log and Continue

In some cases, errors are logged but the code continues execution:

```go
// In heal_synth.go
if err != nil {
    slog.Error("Failed to heal", "source", source, "destination", destination, "number", number, "error", err)
    if count < 3 {
        count++
        goto retry
    }
    return false
}
```

This pattern is more robust as it allows the healing process to continue despite errors in individual transactions.

**Related Documentation**:
- [heal_synth Transaction Creation: Retry Logic](./transactions.md#heal_synth-transaction-creation)
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)

### 4. Inconsistent Error Handling

The healing code uses different error handling approaches in different parts of the codebase:

- `heal_anchor.go` primarily uses the `check` function, which terminates the program on errors
- `heal_synth.go` uses a mix of `check` and "log and continue" approaches
- The shared code in `heal_common.go` mostly uses the `check` function

This inconsistency makes it difficult to predict how the code will behave when errors occur.

**Related Documentation**:
- [Implementation Guidelines: Consistency](./implementation.md#shared-components)

## Specific Issues

The following specific issues have been identified in the healing code:

### 1. Fatal Program Termination

The `check` function calls `fatalf`, which terminates the program immediately. This prevents the healing process from continuing after encountering an error, even if the error only affects a single transaction.

**Impact**: A single error in one transaction can prevent the healing of all other transactions.

**Examples**:
- Network initialization in `heal_common.go` uses `check` for all errors
- Database operations in `heal_synth.go` use `check` for all errors

**Related Documentation**:
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)

### 2. Missing Error Handling

Some parts of the code don't handle errors at all, potentially leading to unexpected behavior:

```go
// In heal_common.go
func (h *healer) loadNetworkStatus() {
    h.netInfo = &healing.NetworkInfo{}
    h.netInfo.Status = *h.C2.GetNetworkStatus(h.ctx)
    // No error checking
}
```

**Impact**: If the network status can't be retrieved, the code will continue with a partially initialized `netInfo` structure.

**Related Documentation**:
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)

### 3. Inconsistent Retry Logic

While some parts of the code implement retry logic, it's not consistently applied across all operations.

**Examples**:
- Synthetic transaction healing has retry logic for pending transactions
- Most other operations don't have retry logic

**Related Documentation**:
- [heal_synth Transaction Creation: Retry Logic](./transactions.md#heal_synth-transaction-creation)
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)

### 4. Limited Error Information

When errors occur, the error messages often don't provide enough context to understand the root cause:

```go
// In heal_synth.go
slog.Error("Failed to heal", "source", source, "destination", destination, "number", number, "error", err)
```

While this logs the basic information, it doesn't include details about what specific operation failed.

**Related Documentation**:
- [Implementation Guidelines: Logging](./implementation.md#error-handling)

## Specific Instances of Missing Error Checks

The following are specific instances of missing error checks, nil handling, and other safety issues in the healing code:

### Missing Nil Checks

1. **Line ~120-130 in `heal_synth.go`**: The `pullSynthDirChains` function doesn't check if `account` is nil before accessing it:
   ```go
   account, err := batch.Account(dirUrl).Main().Get()
   check(err)
   
   // No nil check before accessing account
   dirChains := account.(*protocol.DirectoryAccount).Chains
   ```

2. **Line ~158-167**: In `pullSynthLedger`, the returned ledger is not checked for nil:
   ```go
   var ledger *protocol.SyntheticLedger
   check(batch.Account(part.JoinPath(protocol.Synthetic)).Main().GetAs(&ledger))
   return ledger
   ```

**Related Documentation**:
- [Implementation Guidelines: Defensive Programming](./implementation.md#error-handling)

### Missing Range Checks

1. **Line ~200-210 in `heal_synth.go`**: The `healSynth` function doesn't check if the slice index is valid:
   ```go
   for _, si := range sis {
       if healSingleSynth(h, si.Source, si.Destination, si.Number, si.ID) {
           healed++
       }
   }
   ```

2. **Line ~250-260**: The `pullSynthSrcChains` function doesn't check array bounds:
   ```go
   chains := make([]*protocol.ChainMetadata, len(account.(*protocol.DirectoryAccount).Chains))
   for i, c := range account.(*protocol.DirectoryAccount).Chains {
       // No check if i is valid for chains slice
       chains[i] = c
   }
   ```

**Related Documentation**:
- [Implementation Guidelines: Defensive Programming](./implementation.md#error-handling)

### Unsafe Database Queries

1. **Line ~300-310 in `heal_synth.go`**: The `buildSynthReceiptV1` function doesn't validate database query results:
   ```go
   batch := args.Light.OpenDB(false)
   defer batch.Discard()
   
   // No validation of batch operations
   uSrc := protocol.PartitionUrl(si.Source)
   uSrcSys := uSrc.JoinPath(protocol.Ledger)
   ```

**Related Documentation**:
- [Database Requirements: Error Handling](./database.md#database-interface-requirements)
- [Light Client Implementation: Error Handling](./light_client.md#light-client-usage-in-synthetic-healing)

### Goroutine Error Handling

1. **Line ~50-60 in `heal_common.go`**: The goroutine for closing the Light client doesn't handle errors:
   ```go
   go func() { <-ctx.Done(); _ = h.light.Close() }()
   ```

**Related Documentation**:
- [Implementation Guidelines: Concurrency](./implementation.md#shared-components)

### Inconsistent Error Handling

1. **Line ~150-160 in `heal_anchor.go`** vs **Line ~200-210 in `heal_synth.go`**: Different approaches to handling errors:
   ```go
   // In heal_anchor.go
   err := h.HealAnchor(h.ctx, healing.HealAnchorArgs{...})
   check(err) // Terminates program on error
   
   // In heal_synth.go
   err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{...})
   if err != nil {
       slog.Error("Failed to heal", ...) // Logs and continues
   }
   ```

**Related Documentation**:
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)
- [heal_anchor Transaction Creation](./transactions.md#heal_anchor-transaction-creation)
- [heal_synth Transaction Creation](./transactions.md#heal_synth-transaction-creation)

## Recommendations

Based on the analysis of error handling in the healing code, the following recommendations are made to improve robustness:

1. **Replace `check` with Graceful Error Handling**: Instead of terminating the program on errors, log the error and continue with the next transaction.

2. **Add Nil Checks**: Add explicit nil checks before accessing objects that might be nil.

3. **Expand Retry Logic**: Implement retry logic for all network operations.

4. **Improve Logging**: Ensure all errors are properly logged before any program termination.

5. **Refactor Error Handling**: Replace the `check` function with more nuanced error handling.

6. **Add Defensive Checks**: Implement consistent nil checks, range checks, and type assertion checks.

7. **Standardize URL Construction**: Adopt a consistent approach to URL construction across the codebase.

**Related Documentation**:
- [Implementation Guidelines: Error Handling](./implementation.md#error-handling)
- [Database Requirements: Error Handling](./database.md#database-interface-requirements)
- [URL Construction Considerations](./implementation.md#url-construction)
