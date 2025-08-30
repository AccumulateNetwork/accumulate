# Test Tier Organization

## Tier 1: Unit Tests (< 1 minute)
- **Build Tag**: (default - no tag needed)
- **Purpose**: Fast, isolated unit tests
- **Run**: `go test ./...`

## Tier 2: Integration Tests (< 5 minutes)  
- **Build Tag**: `integration` or `!short`
- **Purpose**: Component integration tests
- **Run**: `go test -tags=integration ./...`

## Tier 3: E2E Tests (< 15 minutes)
- **Build Tag**: `e2e` or `!testnet`
- **Purpose**: End-to-end tests with full simulator
- **Run**: `go test -tags=e2e ./...`

## Tier 4: Load Tests (> 15 minutes)
- **Build Tag**: `load` and `!testnet`
- **Purpose**: Performance and load testing
- **Run**: `go test -tags=load,testnet ./...`

## Usage in CI

```yaml
# Quick CI (on every push)
test-quick:
  script: go test -short ./...

# Standard CI (on merge requests)
test-standard:
  script: go test ./...

# Full CI (nightly/release)
test-full:
  script: go test -tags=testnet ./...
```

## Test File Organization

```
test/
├── unit/        # Tier 1 - No build tags needed
├── integration/ # Tier 2 - //go:build integration
├── e2e/         # Tier 3 - //go:build !testnet
└── load/        # Tier 4 - //go:build load && !testnet
```