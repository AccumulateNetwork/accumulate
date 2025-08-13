# Debugging Summary: V3 Transaction Delivery Issue

## Root Cause Identified
**The v3 API does not support the `acmeFaucet` transaction type.**

## Key Findings

### 1. V2 API Works
- The v2 API successfully processes faucet requests
- Account `acc://17d69fe619cd40ebc7b23396fc2ef6e56e8e406abd517c93/ACME` was funded with 10 ACME via v2

### 2. V3 API Faucet Failure
- Error: `"unsupported transaction type: acmeFaucet"`
- Status: `badRequest`
- The transaction is rejected immediately, not a cross-chain issue

### 3. Cross-Chain Not The Issue
- Even with a single BVN (no cross-chain complexity), v3 faucet fails
- The issue is at the API level, not the consensus/delivery layer

### 4. Implications for Load Testing
- All v3-based load tests that start with faucet operations will fail
- Need to either:
  1. Use v2 API for faucet, then v3 for other operations
  2. Pre-fund accounts via v2 before running v3 tests
  3. Use existing funded accounts

## Solution for Load Testing

To run successful load tests with v3 API:

1. **Initial Setup Phase (v2 API)**
   - Create lite accounts
   - Fund them via v2 faucet
   - Verify balances

2. **Main Test Phase (v3 API)**
   - Use pre-funded accounts
   - Create ADIs
   - Transfer tokens
   - Create data accounts
   - Write data

## Test Implementation
See `working_hybrid_test.go` for implementation that:
- Uses v2 API for faucet operations
- Uses v3 API for all other operations
- Successfully achieves 100% delivery rate