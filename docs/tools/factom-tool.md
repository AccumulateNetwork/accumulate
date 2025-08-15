# Factom Migration Tools

## Overview

The `_factom` tools provide utilities for migrating data from Factom blockchain to the Accumulate Network. These tools handle address conversion, balance migration, and data transformation from Factom format to Accumulate format.

## Installation

```bash
# Build the factom tools
go build -o bin/factom ./tools/cmd/_factom

# Or build all tools
make tools
```

## Usage

```bash
./bin/factom [command] [flags]
```

## Commands

### Address Migration

```bash
# Convert Factom addresses to Accumulate format
./bin/factom addresses --input factom-addresses.csv --output accumulate-addresses.csv

# Validate address conversion
./bin/factom addresses --validate --input factom-addresses.csv
```

### Balance Migration

```bash
# Migrate balances from Factom to Accumulate
./bin/factom balances --input factom-balances.csv --output accumulate-balances.csv

# Apply conversion rate (5:1 ratio)
./bin/factom balances --input factom-balances.csv --rate 5
```

### Data Export

```bash
# Export Factom data for migration
./bin/factom export --output factom-export.json

# Export specific address ranges
./bin/factom export --start-address FA1... --end-address FA9... --output partial-export.json
```

## Configuration

### Input Formats

#### Factom Addresses CSV

```csv
address,balance,type
FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC,1000000,factoid
FA2jK5bCP2iKnpkQoebgXbPzRdCGvLa8jHF7Tp93jKMNNfKLoQzS,500000,factoid
```

#### Expected Output Format

```csv
address,balance,type,accumulate_address
FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC,1000000,factoid,acc://fa1zt4afpevcnpqpcigb3fvgu4q4mtxy22iiuv69dqe1pnhdf2mc
FA2jK5bCP2iKnpkQoebgXbPzRdCGvLa8jHF7Tp93jKMNNfKLoQzS,500000,factoid,acc://fa2jk5bcp2iknpkqoebgxbpzrdcgvla8jhf7tp93jkmnfkloqzs
```

### Environment Variables

```bash
# Set conversion rate (Factom to Accumulate)
export FACTOM_CONVERSION_RATE=5

# Set output format
export FACTOM_OUTPUT_FORMAT=csv

# Set validation mode
export FACTOM_VALIDATE=true
```

## Integration with Testing

### Test Data Generation

The Factom tools are used to generate test data for E2E tests like `TestFactomAddresses`:

```go
// In test setup
func TestFactomAddresses(t *testing.T) {
    // Initialize simulator with Factom addresses
    sim := simulator.NewWith(t, simulator.SimulatorOptions{
        BvnCount: 3,
        FactomAddresses: func() (io.Reader, error) {
            return strings.NewReader(testdata.FactomAddresses), nil
        },
    })
    sim.InitFromGenesis()
    
    // Test migrated balances
    factomAddresses, err := genesis.LoadFactomAddressesAndBalances(
        strings.NewReader(testdata.FactomAddresses))
    require.NoError(t, err)
    
    for _, addr := range factomAddresses {
        account := simulator.GetAccount[*protocol.LiteTokenAccount](sim, addr.Address)
        // Verify 5:1 conversion rate
        assert.Equalf(t, int(5*addr.Balance), int(account.Balance.Int64()),
            "Incorrect balance for %v", addr.Address)
    }
}
```

### Test Data Format

The test data in `testdata.FactomAddresses` follows this format:

```
FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC,1000000
FA2jK5bCP2iKnpkQoebgXbPzRdCGvLa8jHF7Tp93jKMNNfKLoQzS,500000
FA3cih2o2tjxuSsf5WkHgYAELzM2iCdqsqRFA6ffdn9WBSc8Qcb4,750000
```

## Migration Process

### Step 1: Data Collection

```bash
# Export Factom blockchain data
./bin/factom export --all --output factom-full-export.json

# Validate exported data
./bin/factom validate --input factom-full-export.json
```

### Step 2: Address Conversion

```bash
# Convert addresses to Accumulate format
./bin/factom addresses \
  --input factom-full-export.json \
  --output accumulate-addresses.csv \
  --format csv
```

### Step 3: Balance Migration

```bash
# Apply conversion rate and migrate balances
./bin/factom balances \
  --input accumulate-addresses.csv \
  --output final-migration.csv \
  --rate 5 \
  --validate
```

### Step 4: Genesis Integration

```bash
# Generate genesis file with migrated data
./bin/genesis create \
  --factom-addresses final-migration.csv \
  --output genesis-with-factom.json
```

## Validation and Testing

### Address Validation

```bash
# Validate address format conversion
./bin/factom validate addresses --input converted-addresses.csv

# Check for duplicate addresses
./bin/factom validate duplicates --input converted-addresses.csv

# Verify checksum integrity
./bin/factom validate checksums --input converted-addresses.csv
```

### Balance Validation

```bash
# Verify balance totals
./bin/factom validate balances --input final-migration.csv

# Check conversion rate application
./bin/factom validate conversion --input final-migration.csv --expected-rate 5

# Audit trail generation
./bin/factom audit --input final-migration.csv --output audit-report.json
```

### Integration Testing

```bash
# Test migration with simulator
./bin/simulator --factom-addresses final-migration.csv --port 8080 &

# Run migration tests
go test ./test/e2e/TestFactomAddresses -v

# Verify migrated accounts
curl http://localhost:8080/v2/account/acc://fa1zt4afpevcnpqpcigb3fvgu4q4mtxy22iiuv69dqe1pnhdf2mc
```

## Common Use Cases

### Development Testing

```bash
# Generate test data for development
./bin/factom generate-test-data \
  --count 100 \
  --output test-factom-addresses.csv

# Use in simulator
./bin/simulator --factom-addresses test-factom-addresses.csv
```

### Production Migration

```bash
# Full production migration workflow
./bin/factom export --production --output prod-factom.json
./bin/factom addresses --input prod-factom.json --output prod-addresses.csv
./bin/factom balances --input prod-addresses.csv --rate 5 --output final-prod.csv
./bin/factom validate --input final-prod.csv --strict
```

### Audit and Reporting

```bash
# Generate migration report
./bin/factom report \
  --input final-migration.csv \
  --output migration-report.html \
  --format html

# Export audit data
./bin/factom audit \
  --input final-migration.csv \
  --output audit.json \
  --include-checksums
```

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Invalid address format | Check input CSV format and headers |
| Balance mismatch | Verify conversion rate and input data |
| Duplicate addresses | Use `--deduplicate` flag |
| Checksum errors | Validate source data integrity |

### Debug Commands

```bash
# Debug address conversion
./bin/factom addresses --input test.csv --debug --verbose

# Trace balance calculations
./bin/factom balances --input test.csv --trace --output debug.log

# Validate step by step
./bin/factom validate --input test.csv --step-by-step
```

## File Formats

### Input CSV Format

```csv
address,balance,type,metadata
FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC,1000000,factoid,genesis
FA2jK5bCP2iKnpkQoebgXbPzRdCGvLa8jHF7Tp93jKMNNfKLoQzS,500000,factoid,migration
```

### Output JSON Format

```json
{
  "migration_info": {
    "timestamp": "2025-01-17T08:00:00Z",
    "conversion_rate": 5,
    "total_addresses": 1000,
    "total_balance_factom": 50000000,
    "total_balance_accumulate": 250000000
  },
  "addresses": [
    {
      "factom_address": "FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC",
      "accumulate_address": "acc://fa1zt4afpevcnpqpcigb3fvgu4q4mtxy22iiuv69dqe1pnhdf2mc",
      "factom_balance": 1000000,
      "accumulate_balance": 5000000,
      "status": "migrated"
    }
  ]
}
```

## See Also

- [Genesis Tool](genesis.md) - Genesis block creation utilities
- [E2E Tests](../../test/docs/e2e-tests.md) - End-to-end testing with Factom data
- [Simulator Tool](simulator.md) - Network simulation with Factom addresses
