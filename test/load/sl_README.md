# Streamlined Load Test Suite (sl_*)

All streamlined load test files are prefixed with `sl_` for easy identification.

## Files

- **sl_load_test.go** - Main streamlined load test implementation
- **sl_credits_test.go** - Test for AddCredits functionality  
- **sl_simple_test.go** - Simplified single-account test
- **sl_design.md** - Complete design document
- **sl_README.md** - This file

## Running Tests

```bash
# Run main streamlined load test
go test -v ./test/load -run TestStreamlinedLoad -timeout 5m

# Run simple credits test
go test -v ./test/load -run TestSimpleCredits -timeout 2m

# Run all sl_ tests
go test -v ./test/load -run "TestStreamlined|TestSimple" -timeout 5m
```

## Configuration

Edit constants in `sl_load_test.go`:
```go
const (
    numSenders   = 3    // Number of sender accounts (k1-kN)
    numReceivers = 3    // Number of receiver accounts (a1-aN)  
    numTxs       = 100  // Total transactions to send
    txAmount     = 0.001 * 1e8  // ACME per transaction
)
```

## Key Features

1. **Clean Accounting** - Each sender starts with exactly 100 ACME
2. **Unique Accounts** - Uses timestamp-based seeds to avoid interference
3. **Fail-Fast** - Aborts early if setup fails
4. **Detailed Reporting** - Shows expected vs actual for all accounts
5. **Issue Detection** - Identifies when transactions aren't debited/credited

## Known Issues

The tests have revealed that the devnet:
- Accepts transactions at high rates (3000-10000 TPS)
- Does not debit senders under load
- Does not credit receivers under load  
- Provides no error feedback when dropping transactions

See `sl_design.md` for complete documentation.