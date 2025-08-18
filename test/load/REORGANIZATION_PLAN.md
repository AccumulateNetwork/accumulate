# Test/Load Directory Reorganization Plan

## Current State
The test/load directory has become cluttered with various test implementations and documentation. We need to organize it properly.

## New Structure

```
test/load/
├── sl-load/                    # Streamlined Load Test (PRIMARY)
│   ├── README.md               # SL test documentation
│   ├── sl_test.go              # Main test entry
│   ├── sl_types.go             # Data structures
│   ├── sl_accounts.go          # Account management
│   ├── sl_credits.go           # Credit management
│   ├── sl_load.go              # Load generation
│   ├── sl_settlement.go        # Settlement verification
│   ├── sl_report.go            # Reporting
│   ├── sl_helpers.go           # Helper functions
│   └── devnet_endpoint.go      # Endpoint discovery
│
├── simple/                     # Simple preset tests
│   ├── README.md               # Simple test docs
│   ├── simple_50k_test.go      # 50k preset test
│   └── simple_100k_test.go     # 100k preset test
│
├── utilities/                  # Shared utilities
│   ├── devnet_endpoint.go      # Endpoint discovery
│   ├── devnet_smart_discovery.go # Smart discovery
│   ├── account_generator.go    # Account generation
│   └── account_funder.go       # Account funding
│
├── experimental/               # Experimental/outdated tests
│   ├── diagnostic_load_test.go
│   ├── devnet_load_test.go
│   ├── smart_devnet_test.go
│   └── consolidated_load_test.go
│
├── docs/                       # Documentation
│   ├── CONSOLIDATED_DOCS.md    # Main documentation
│   ├── LOAD_TEST_GUIDE.md      # Test guide
│   └── TPS_PERFORMANCE_REPORT.md # Performance report
│
└── README.md                   # Main README pointing to subdirectories
```

## Files to Move

### To sl-load/ (✅ Already Done)
- All sl_*.go files
- All sl_*.md files
- devnet_endpoint.go (copy)

### To simple/
- simple_50k_test.go
- simple_100k_test.go

### To utilities/
- devnet_endpoint.go
- devnet_smart_discovery.go
- account_generator.go
- account_funder.go

### To experimental/
- diagnostic_load_test.go
- devnet_load_test.go
- smart_devnet_test.go
- consolidated_load_test.go
- utilities_test.go

### To docs/
- CONSOLIDATED_DOCS.md
- LOAD_TEST_GUIDE.md
- TPS_PERFORMANCE_REPORT.md
- All other .md files

## Benefits

1. **Clear Organization**: Each directory has a specific purpose
2. **Primary Test Clear**: sl-load is the main functional test
3. **Simple Access**: Preset tests in simple/ for quick testing
4. **Shared Code**: Utilities available to all tests
5. **Clean Working Directory**: No clutter in main test/load

## Implementation Steps

```bash
# 1. Create directories
mkdir -p simple utilities experimental docs

# 2. Move simple tests
mv simple_50k_test.go simple_100k_test.go simple/

# 3. Move utilities
mv devnet_smart_discovery.go account_generator.go account_funder.go utilities/
cp devnet_endpoint.go utilities/

# 4. Move experimental
mv diagnostic_load_test.go devnet_load_test.go smart_devnet_test.go consolidated_load_test.go experimental/
mv utilities_test.go experimental/

# 5. Move documentation
mv CONSOLIDATED_DOCS.md LOAD_TEST_GUIDE.md TPS_PERFORMANCE_REPORT.md docs/
mv *.md docs/  # Move remaining .md files

# 6. Create new README
# Create a new README.md that points to the subdirectories
```

## Usage After Reorganization

```bash
# Run streamlined load test (primary)
cd sl-load && go test -v -run TestStreamlinedLoad

# Run simple preset tests
cd simple && go test -v -run TestSimple50K

# Use from parent directory
go test -v ./sl-load -run TestStreamlinedLoad -args -txs 10000 -tps 100
go test -v ./simple -run TestSimple100K
```

---

This reorganization will make the test/load directory much more manageable and clear.