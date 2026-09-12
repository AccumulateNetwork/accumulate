# AIP-50 User Fee Examples

This directory contains executable examples demonstrating AIP-50 (User-Specified Transaction Fees) functionality.

## Prerequisites

1. **Devnet running** with AIP-50 code:
   ```bash
   # From repository root
   docker-compose up -d
   ```

2. **Go 1.21+** installed

## Quick Start

Run the scripts in order:

```bash
cd examples/aip-50

# Step 1: Create Alice and Bob ADIs with funded token accounts
go run alice_bob_fee_examples.go

# Step 2: Create the fee service ADI
go run service_setup.go

# Step 3: Run any enforcement model demo
go run model1_multisig_enforcement.go   # Multi-signature authority
go run model2_namespace_enforcement.go  # Namespace/custodial model
go run model4_delegation_enforcement.go # Delegation model
```

## Scripts

| Script | Description |
|--------|-------------|
| `alice_bob_fee_examples.go` | Creates Alice and Bob ADIs with token accounts, credits, and ACME balance |
| `service_setup.go` | Creates the fee service ADI with its own keypair and fee account |
| `model1_multisig_enforcement.go` | Demonstrates multi-sig authority enforcement (service must co-sign) |
| `model2_namespace_enforcement.go` | Demonstrates namespace/dual-authority enforcement (custodial model) |
| `model4_delegation_enforcement.go` | Demonstrates delegation (service signs on behalf of user) |
| `verify_userfee.go` | Verifies that AIP-50 UserFee support is active in the devnet |

## Configuration

Edit `testconfig/config.go` to change:

- **Version**: Increment to create fresh ADIs on re-run
- **ACC_API**: API endpoint (default: `http://127.0.0.1:26660/v3`)
- **Keys**: Fixed keys for reproducible testing

## Enforcement Models

### Model 1: Multi-Signature Authority

Service is added as an **authority** on the user's account. Both user AND service must sign every transaction. Service only signs if `UserFee` is included.

**Result**: User CANNOT transact without paying the fee.

### Model 2: Namespace Ownership

Service owns the namespace and creates user accounts under it. User accounts have **dual authority** (both user and service). This is the "custodial exchange" pattern.

**Result**: Perfect for exchanges - users can initiate but need service approval.

### Model 4: Delegation

User adds service as a **delegate** on their key page. Service can sign transactions on behalf of the user. However, delegation alone does NOT prevent bypass - user can still sign directly.

**Result**: Better UX but no enforcement. Combine with Model 1 for full enforcement.

## Troubleshooting

### "Cannot connect to devnet"
Ensure devnet is running: `docker-compose up -d`

### "UserFees unknown field" error
The devnet was built without AIP-50 code. Rebuild:
```bash
docker-compose build --no-cache
docker-compose up -d
```

### "Insufficient balance"
Run `alice_bob_fee_examples.go` first to fund the accounts.

### Scripts fail with "already exists"
Increment the `Version` in `testconfig/config.go` to use fresh ADI names.

## Related Documentation

- [AIP-50 Implementation Review](../../docs/aip-50-implementation-review.md)
- [User Fees Guide](../../docs/user-fees-guide.md)
