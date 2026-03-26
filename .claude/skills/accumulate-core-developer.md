# Accumulate Core Developer Skill

You are an expert Accumulate protocol developer with deep knowledge of:

## Core Competencies

### Protocol Knowledge
- **Account Model**: ADIs (Accumulate Digital Identifiers), Lite Accounts, Token Accounts, Data Accounts, KeyBooks, KeyPages
- **Transaction Types**: All protocol transaction types including SendTokens, WriteData, UpdateKeyPage, CreateIdentity, CreateTokenAccount, etc.
- **Delegation**: SetLiteAccountDelegate for delegating account authority to KeyBooks
- **Consensus**: BVN (Block Validator Network) and DN (Directory Network) architecture
- **Anchoring**: Cross-chain anchoring and synthetic transactions

### Codebase Structure
```
accumulate/
├── cmd/                    # CLI tools
├── internal/
│   ├── api/               # API layer
│   ├── core/              # Core execution logic
│   │   ├── block/         # Block processing
│   │   ├── execute/       # Transaction execution
│   │   └── events/        # Event handling
│   ├── database/          # BadgerDB storage
│   └── node/              # Node infrastructure
├── pkg/
│   ├── api/               # Public API types
│   ├── client/            # API client
│   ├── types/             # Common types
│   └── url/               # URL handling
├── protocol/              # Protocol definitions (YAML → Go)
│   ├── accounts.yml       # Account type definitions
│   ├── transactions.yml   # Transaction type definitions
│   └── *.go              # Generated Go code
├── smt/                   # Sparse Merkle Tree
└── test/                  # Integration tests
```

### Key Files for Common Tasks

**Adding a new transaction type:**
1. `protocol/transactions.yml` - Define the transaction structure
2. `internal/core/execute/v2/chain/<tx_name>.go` - Implement executor
3. `protocol/` - Run code generation

**Adding a new account type:**
1. `protocol/accounts.yml` - Define account structure
2. Generate code with `go generate ./...`

**Modifying transaction execution:**
- `internal/core/execute/v2/chain/*.go` - Chain-specific executors
- `internal/core/block/shared/shared.go` - Shared execution logic (e.g., GetAccountAuthoritySet)

**API changes:**
- `pkg/api/` - API type definitions
- `internal/api/` - API implementation

### Build & Test Commands

```bash
# Build
go build ./...

# Run all tests
go test ./...

# Run specific package tests
go test -v ./internal/core/execute/...

# Run with race detection
go test -race ./...

# Generate protocol code
go generate ./protocol/...

# Run the node
./cmd/accumulated/accumulated run
```

### GitLab Workflow

**Repository**: `accumulatenetwork/accumulate`

```bash
# View issue
glab issue view <number> --repo accumulatenetwork/accumulate --comments

# Create MR
glab mr create --title "Description" --repo accumulatenetwork/accumulate

# List issues
glab issue list --repo accumulatenetwork/accumulate
```

## Development Guidelines

### Error Handling
- Use `errors.Is()` and `errors.As()` for error checking
- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- Never ignore errors without explicit comment

### Transaction Validation
- Validate in `Validate()` method - fast, syntactic checks
- Validate in `Execute()` - checks requiring state access
- Return appropriate error codes from `protocol/errors.go`

### Account State Changes
- Use `batch.Account()` to get account for modification
- Call `batch.Commit()` to persist changes
- Be careful with synthetic transactions and cross-partition effects

### Testing
- Unit tests in `*_test.go` files alongside implementation
- Integration tests in `test/` directory
- Use `simulator` package for end-to-end testing

### Code Generation
The protocol package uses code generation from YAML:
- `protocol/accounts.yml` → Account types
- `protocol/transactions.yml` → Transaction types
- `protocol/results.yml` → Result types

Run `go generate ./protocol/...` after YAML changes.

## Common Patterns

### Getting Account Authority
```go
auth, _, err := shared.GetAccountAuthoritySet(batch, account)
if err != nil {
    return nil, err
}
// auth.Authorities contains the signing authorities
```

### Checking Delegation
```go
if lta, ok := account.(*protocol.LiteTokenAccount); ok {
    if lta.Delegate != nil {
        // Account is delegated to lta.Delegate
    }
}
```

### Creating Synthetic Transactions
```go
delivery := new(chain.Delivery)
delivery.Transaction = new(protocol.Transaction)
delivery.Transaction.Body = &protocol.SyntheticDepositTokens{...}
// Add to pending synthetics
```

## Resources
- Protocol specification: `docs/`
- API documentation: `pkg/api/`
- Transaction executors: `internal/core/execute/v2/chain/`
