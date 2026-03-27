# Changelog - Issue #3862

## Changes Made

### 1. Go Version Alignment (2026-03-26)

**Problem**: go.mod specified Go 1.25.0 but local tooling and CI used Go 1.24.4

**Solution**: Downgraded to Go 1.24.0 with toolchain directive
- Updated `go.mod`: Go 1.25.0 → 1.24.0
- Added `toolchain go1.24.4`
- Updated `Dockerfile`: golang:1.22 → golang:1.24
- Regenerated `go.sum` with `go mod tidy`

**Reason**: Ensure consistent builds across development, CI, and Docker environments

**Files Modified**:
- `go.mod`
- `go.sum`
- `Dockerfile`

### 2. BPT Sharding Configuration (2026-03-26)

**Added**: Binary Patricia Tree (BPT) sharding configuration for testing

**Changes**:
- `internal/node/config/types.yml`: Added BPT and BPTSharding types
- `test/docker/docker-compose.yml`: Added environment variables
  - `ACC_BPT_SHARDING_ENABLED=true`
  - `ACC_BPT_SHARDING_DEPTH=4`

**Verification**: BPT active with no errors (confirmed via log monitoring)

### 3. Docker Port Mapping (2026-03-26)

**Problem**: Using popular ports (8080-8083) causes conflicts with other services

**Solution**: Remapped to 20000+ range
- BVN0 API: 8080 → 20080
- BVN1 API: 8081 → 20081
- BVN2 API: 8082 → 20082
- DN API:   8083 → 20083
- P2P:      26656-26670 → 20656-20670
- Metrics:  26660-26674 → 20660-20674

**Files Modified**:
- `test/docker/docker-compose.yml`

### 4. API Listen Address Fix (2026-03-27)

**Problem**: HTTP APIs bound to 127.0.0.1 (container-internal only)

**Solution**: Changed to 0.0.0.0 (all interfaces)
- Modified `cmd/accumulated/run/devnet.go` line 35
- Changed `localHost = "/ip4/127.0.0.1"` → `localHost = "/ip4/0.0.0.0"`

**Impact**: APIs can now be accessed from Docker host (though devnet doesn't expose them properly - see below)

### 5. Binary Exclusions (2026-03-26)

**Added to `.gitignore`**:
- `accumulated-dagbft` - Test binary
- `test/docker/loadtest-12k` - Load test binary (40MB)

### 6. Critical Devnet Warnings (2026-03-27)

**Problem**: Devnet mode was incorrectly used for load testing, wasting hours of debugging

**Discovery**: `accumulated run devnet` does NOT create HTTP API services for BVN validators
- Only bootstrap node gets HTTP service (metrics port 26660)
- BVN nodes missing HttpService and RouterService
- Architectural gap: devnet.go vs core_validator.go

**Solution**: Added prominent warnings
- `cmd/accumulated/run/devnet.go`: 30-line warning comment
- `test/docker/docker-compose.yml`: Warning header
- `docs/deployment/devnet-warnings.md`: Full documentation

**Warning Message**: NEVER use devnet for load/performance/API testing

## Lessons Learned

1. **Read the code before deployment** - Devnet architecture was clearly single-process
2. **Verify assumptions** - Test that APIs actually work before extensive setup
3. **Document limitations** - Add warnings to prevent future misuse
4. **Use proper tools** - Distributed mode exists for multi-node testing

## Remaining Work

- [ ] Determine proper deployment method for 12-node load testing
- [ ] Test distributed docker-compose mode
- [ ] Verify HTTP APIs work in distributed mode
- [ ] Run actual load tests with BPT sharding
- [ ] Document correct deployment procedure

## Commits

- `5e8ed3060` - Add BPT sharding configuration and downgrade Go to 1.24
- `3cade06ff` - Add critical warnings about NEVER using devnet mode

## Related Issues

- #3862 - Fix consensus-testnet panic: non-positive interval for NewTicker
