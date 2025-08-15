# Load Generator Funding Specification

## Core Requirements
ACME tokens needed for: transaction fees (credits), account creation, token transfers for load testing.
System must run continuously without manual intervention.
Funding goroutine must acquire maximum ACME possible and maintain 1000 credits on ALL lite accounts and key pages.

## Architecture
**Primary Components:**
- `fundingAccount`: Central lite account receiving ACME from faucet
- `FundingManager`: Goroutine monitoring/distributing funds
- Flow: Faucet → Primary Account → Load Generator Accounts (ADIs, Lite accounts, Key pages)

## Key Data Structures
```
FundingManager:
  - targetCredits: 1000 (maintain on all accounts/key pages)
  - requestAmount: MAX_ALLOWED (request maximum from faucet)
  - faucetCooldown: 60 seconds
  - Metrics: successfulRequests, failedRequests

FundingMetrics:
  - Balance: current, lowest, highest
  - Faucet: requests, successes, failures, total ACME
  - Distribution: accounts topped up, credits distributed
  - Coverage: accounts below target, key pages below target
```

## Operational Flow
**Initial Setup:**
1. Generate ED25519 key pair for funding account
2. Request maximum ACME from faucet
3. Verify balance before proceeding

**Continuous Goroutine Loop:**
1. Request maximum ACME from faucet (respecting cooldown)
2. Iterate through ALL wallet lite accounts
3. Check each account's credit balance
4. If credits < 1000: add credits to bring to 1000
5. Iterate through ALL wallet key pages
6. Check each key page's credit balance
7. If credits < 1000: add credits to bring to 1000
8. Sleep briefly, then repeat

## Distribution Rules
**Credit Maintenance:** ALL lite accounts and key pages maintain exactly 1000 credits
**Acquisition Strategy:** Always request maximum allowed ACME from faucet
**Distribution Priority:** Credits before token balances

## Faucet Integration
**Request:** POST to faucet URL with lite account address and maximum amount
**Rate Limiting:** 60-second cooldown between requests
**Error Handling:** Count successes and failures, continue operation

## Critical Thresholds
- Target Credits: 1000 (maintain on ALL accounts)
- Request Strategy: ALWAYS request maximum from faucet
- Faucet Cooldown: 60 seconds between requests
- Check Interval: Continuous with brief sleep

## Environment Configuration
```
LOADGEN_FAUCET_URL=http://localhost:9660/faucet
LOADGEN_FAUCET_COOLDOWN=60s
LOADGEN_TARGET_CREDITS=1000
LOADGEN_MAX_FAUCET_REQUEST=10000000  # Request maximum allowed
LOADGEN_CHECK_INTERVAL=5s            # How often to check accounts
```

## Security Requirements
- Private keys in memory only (never logged/persisted)
- Client-side rate limiting to prevent faucet abuse
- Monitor unusual balance drainage patterns

## Monitoring Metrics
- Faucet request successes count
- Faucet request failures count
- Accounts maintained at target credits
- Total credits distributed

## Testing Requirements
- **NO MOCK TESTS EVER** - Mock tests hide API bugs and are useless
- **NO SIMULATED TESTS** - Must use actual network endpoints
- **ALL TESTS HIT REAL DEVNET** - Every test must make real transactions
- Tests run against local devnet (http://localhost:26660/v3)
- Only exception: Pure calculations with no protocol interaction
- Verify funding keeps pace with consumption using real faucet
- Test low-balance recovery with real transactions
- Ensure no deadlocks in funding loops with actual network delays

### Running Tests
1. **Start devnet first**: Tests require a running devnet at localhost:26660
2. **Run tests**: `go test -v ./loadgen/wallet`
3. **Skip devnet tests**: Use `-short` flag to skip devnet tests
4. Tests will automatically skip if devnet is not available

## Implementation Priorities
1. Continuous goroutine requesting maximum ACME from faucet
2. Loop through all lite accounts checking/adding credits to 1000
3. Loop through all key pages checking/adding credits to 1000
4. Count successes and failures (no retry logic needed)
5. Simple metrics reporting

## Success Criteria
- Continuous goroutine operation
- All lite accounts maintain 1000 credits
- All key pages maintain 1000 credits
- Maximum ACME acquisition from faucet
- Simple success/failure counting