# Go Files Found in Docs Directory - Cleanup Plan

## Problem
Found 11 `.go` files in the docs directory that should be elsewhere:

### Test Files (should go to test directories)
- `debug-lite-client-test.go` - Lite client test
- `debug/lite_client_test.go` - Another lite client test

### Implementation Files (appear to be duplicates/examples)
- `crosschain/core-v2-crosschain/*.go` (6 files) - Looks like copies of actual crosschain implementation
- `crosschain/core-crosschain/*.go` (2 files) - More crosschain implementation copies
- `package-2-error-example/main.go` - Example code

## Action Plan

### 1. Move Test Files
```bash
# Move lite client tests to appropriate test directory
mv debug-lite-client-test.go ../test/testing/
mv debug/lite_client_test.go ../test/testing/
```

### 2. Remove Duplicate Implementation Files
The crosschain/*.go files appear to be copies of the actual implementation in:
- `internal/core/execute/v2/crosschain/`

These should be removed from docs as they're documentation duplicates.

### 3. Move Example Code
```bash
# Move example to examples directory or remove if outdated
mv package-2-error-example/ ../examples/
```

## Why These Don't Belong in Docs

1. **Test files** - Should be with other tests, not in documentation
2. **Implementation copies** - Documentation should reference code, not duplicate it
3. **Examples** - Should be in an examples directory, not docs

## Commands to Execute

```bash
# Create backup first
mkdir -p _backup_go_files
cp -r crosschain/core-v2-crosschain/*.go _backup_go_files/
cp -r crosschain/core-crosschain/*.go _backup_go_files/
cp debug-lite-client-test.go _backup_go_files/
cp debug/lite_client_test.go _backup_go_files/

# Move test files
mv debug-lite-client-test.go ../../test/
mv debug/lite_client_test.go ../../test/

# Remove implementation duplicates (after verifying they're duplicates)
rm -rf crosschain/core-v2-crosschain/*.go
rm -rf crosschain/core-crosschain/*.go

# Handle example
mv package-2-error-example ../../examples/
```