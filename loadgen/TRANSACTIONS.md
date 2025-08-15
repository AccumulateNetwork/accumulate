# Load Generator Transaction Specifications

## Overview
Load generator creates realistic transaction mix to stress test Accumulate network while maintaining proper account relationships and balances.

## Transaction Categories & Distribution

### Infrastructure (30%)
- create_adi (5%): Create new ADI identity with initial KeyBook
- create_key_book (3%): Add KeyBook to existing ADI for key management
- create_key_page (3%): Add KeyPage to KeyBook for holding keys
- update_key_page (2%): Add/remove keys, update thresholds
- create_token_account (7%): Create ADI-controlled token account
- create_data_account (3%): Create ADI-controlled data storage account
- create_lite_account (5%): Create lightweight token account
- add_credits (2%): Convert ACME to credits for transaction fees

### Value Transfer (50%)
- send_tokens_adi (20%): Transfer between ADI accounts
- send_tokens_lite (15%): Transfer between lite accounts
- send_tokens_mixed (10%): Cross-type transfers (lite↔ADI)
- burn_tokens (3%): Permanently destroy tokens
- lock_account (2%): Time-lock account until block height

### Data Operations (15%)
- write_data (10%): Store data entry in ADI data account
- write_data_to_lite (3%): Store data in lite data account
- scratch_data (2%): Temporary data storage without permanent account

### Token Issuance (5%)
- create_token (2%): Define new token type with properties
- issue_tokens (2%): Mint tokens to recipient accounts
- update_token_issuer (1%): Modify token issuer properties

## Transaction Specifications

### Create ADI
Prerequisites: Funding account, unique name, control key
Outputs: New ADI with initial KeyBook and KeyPage

### Create Key Book
Prerequisites: ADI ownership, authority, credits
Outputs: New KeyBook in ADI with empty pages

### Create Key Page
Prerequisites: KeyBook, authority, credits
Outputs: New KeyPage with initial keys and threshold

### Update Key Page
Prerequisites: KeyPage, threshold authority, credits
Outputs: Modified key set and/or threshold

### Create Token Account
Prerequisites: ADI, creation authority, credits
Outputs: New token account linked to token type

### Create Data Account
Prerequisites: ADI, creation authority, credits
Outputs: New data storage account

### Create Lite Account
Prerequisites: Funding source, unique key
Outputs: Lite identity and token account

### Add Credits
Prerequisites: ACME balance, valid oracle rate
Outputs: Credits added, ACME deducted

### Send Tokens (ADI)
Prerequisites: Balance, both accounts exist, authority, credits
Outputs: Balance transferred, transaction recorded

### Send Tokens (Lite)
Prerequisites: Balance, both accounts exist, credits
Outputs: Balance transferred

### Send Tokens (Mixed)
Prerequisites: Balance, compatible tokens, credits
Outputs: Transfer completed, synthetic if cross-partition

### Burn Tokens
Prerequisites: Balance, burn authority, credits
Outputs: Tokens destroyed, supply reduced

### Lock Account
Prerequisites: Lock authority, credits
Outputs: Account locked until specified height

### Write Data
Prerequisites: Data account, write authority, credits
Outputs: Data entry added, hash recorded

### Write Data to Lite
Prerequisites: Lite identity credits
Outputs: Lite data account created/updated

### Scratch Data
Prerequisites: Any signer with credits
Outputs: Temporary data written

### Create Token
Prerequisites: ADI, unique symbol, credits
Outputs: Token issuer created

### Issue Tokens
Prerequisites: Issuer authority, recipient account, within supply limit
Outputs: Tokens created and sent

### Update Token Issuer
Prerequisites: Update authority, credits
Outputs: Issuer properties modified

## Load Profiles

### Setup Profile
Initial network state creation focusing on infrastructure (70% ADI/account creation)

### Steady State Profile
Normal operations (60% transfers, 20% data, 20% infrastructure)

### Stress Test Profile
Maximum load with cheapest transactions (70% lite transfers, 30% data)

### Token Economy Profile
Token-focused operations (65% token transfers, 20% issuance, 15% burns/locks)

## Configuration Parameters

### Performance
- Target TPS: Transactions per second goal
- Burst Size: Maximum concurrent transactions
- Ramp Time: Duration to reach target rate

### Limits
- Max ADIs: Total ADI limit
- Max Accounts/ADI: Account creation limit
- Max Keys/Page: Key limit per page
- Max Data Size: Entry size limit

### Behavior
- Multi-Sig Preference: Use multi-signature when available
- Cross-Partition Rate: Percentage crossing partitions
- Failure Injection: Artificial failure rate for testing

## Error Handling

### Transient Errors (Retry)
- Network timeouts
- Temporary unavailability
- Rate limiting

### Permanent Errors (Skip)
- Insufficient balance
- Invalid authority
- Non-existent accounts

### Fatal Errors (Stop)
- Unable to fund
- Network unreachable
- Configuration errors

## Metrics Tracking

### Performance Metrics
- Transaction counts by type (attempted/succeeded/failed)
- Latency percentiles (P50/P95/P99)
- Current/peak/average TPS
- Error counts by category

### State Tracking
- Pending transactions by type
- Created entities (ADIs/accounts/keys)
- Active workers
- Consecutive error count

## Transaction Selection Algorithm
Weighted random selection based on configured distribution profile with cumulative probability calculation for efficient type selection.

## Prerequisites Gathering
Validates transaction feasibility by checking account existence, balances, authorities, and credits before attempting execution.

## Rate Control
- Token bucket for TPS limiting
- Exponential backoff for retries
- Circuit breaker for overload protection
- Jitter for retry distribution

## Worker Pool Management
Concurrent workers process transaction queue with configurable pool size, work distribution, and result aggregation.

## Summary
Comprehensive transaction coverage exercising all Accumulate protocol operations with configurable distributions, robust error handling, detailed metrics, and complete lifecycle tracking for realistic network load generation.