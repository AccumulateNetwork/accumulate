# Test Data Generator Testing Guide

## Overview

The gen-testdata tool generates test vectors for SDK testing. It includes comprehensive unit tests to validate the generated data.

## Running Tests

All tests are unit tests and require no external dependencies:

```bash
go test -v
```

## Test Coverage

The test suite validates:

### Test Data Generation
- Generates valid JSON test data files
- Creates ledger test vectors
- Produces expected transaction types
- Includes all account types

### Transaction Test Groups
Expected transaction types:
- CreateIdentity
- CreateTokenAccount
- SendTokens
- CreateDataAccount
- WriteData
- AcmeFaucet
- CreateToken
- And more...

### Account Test Groups
Expected account types:
- Identity
- TokenIssuer
- TokenAccount
- LiteTokenAccount
- KeyPage
- KeyBook
- DataAccount

### Test Case Validation
Each test case must have:
- Non-empty binary data
- Valid JSON representation
- Proper structure

### Simple Hash Mode
Tests verify both:
- Simple hash enabled (default)
- Simple hash disabled

## Output Verification

Tests verify that generated files:
1. Exist on disk
2. Contain valid JSON
3. Include expected test groups
4. Have non-empty test cases

## Running Specific Tests

```bash
# Test data generation only
go test -v -run TestGenerateTestData

# Test transaction cases
go test -v -run TestTransactionTestCases

# Test account cases
go test -v -run TestAccountTestCases

# Test simple hash mode
go test -v -run TestSimpleHashMode
```

## CI/CD Integration

For CI pipelines:

```bash
# Run all tests
go test -v ./test/cmd/gen-testdata

# With coverage
go test -v -cover ./test/cmd/gen-testdata
```

## Test Performance

All tests complete in <1 second:
- No network I/O
- No disk I/O (uses TempDir)
- Pure computation

## Troubleshooting

Tests should never fail. If they do:

1. Check that imports are correct
2. Verify test data structures match SDK expectations
3. Ensure JSON marshaling works
4. Validate binary encoding

All test failures indicate bugs in the generator code that must be fixed before use.
