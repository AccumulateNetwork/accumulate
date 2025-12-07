# Integration Tests

This directory contains integration tests that test the full Accumulate system including devnet operations and follower deployment.

## Tests

### TestDeployFollowerWithDevnet

**File:** `follower_test.go`

Full end-to-end integration test for follower deployment:

1. Initializes and starts a local devnet
2. Waits for blocks to be produced
3. Takes snapshots from the running devnet
4. Deploys a follower using the deploy-follower tool
5. Verifies the follower syncs with the devnet

**Requirements:**
- Set `RUN_DEVNET_TEST=1` environment variable to enable
- Requires sufficient system resources (CPU, RAM, disk)
- Uses high ports (36656+) to avoid conflicts with running services

**Usage:**
```bash
RUN_DEVNET_TEST=1 go test -v -tags integration ./test/integration/...
```

### TestDeployFollowerDryRun

**File:** `follower_test.go`

Quick validation test that checks the file validation logic of the deploy-follower tool without requiring a full devnet. This test always runs (no environment variable needed).

**Usage:**
```bash
go test -v -tags integration ./test/integration/...
```

## Build Tags

All tests in this directory require the `integration` build tag:

```go
//go:build integration
```

This prevents them from running during normal `go test ./...` runs.

## Running Tests

```bash
# Run quick tests only
go test -v -tags integration ./test/integration/...

# Run full devnet integration test
RUN_DEVNET_TEST=1 go test -v -tags integration ./test/integration/...

# Run with timeout (devnet test may take several minutes)
RUN_DEVNET_TEST=1 go test -v -tags integration -timeout 10m ./test/integration/...
```

## Logs

Test logs are written to `/tmp/`:
- `/tmp/devnet-test.log` - Devnet output during TestDeployFollowerWithDevnet
