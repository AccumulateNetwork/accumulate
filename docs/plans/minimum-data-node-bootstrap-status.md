# Minimum-Data Node Bootstrap — Implementation Status

**Branch:** `minimum-bootstrap-launch` (integration baseline; design doc + back-walk probe)
**Companion:** `docs/plans/minimum-data-node-bootstrap.md`
**Tracking issue:** #3953

This file tracks the per-issue branches and their state. Each branch is a self-contained slice — explore individually with `git checkout <branch>` and `go test ./internal/core/bootstrap/<pkg>/`.

## Per-issue branches

| Issue | Branch | Package(s) | Tests | Status |
|---|---|---|---|---|
| #3957 | `issue-3957-resolve-keybook-at` | `internal/core/bootstrap/keybookat` + chain wrapper | **7/7** | Working: forward replay, no-replay short-circuit, in-memory tests |
| #3958 | `issue-3958-get-bpt-leaf` | `internal/core/bootstrap/bptproof` | — | Working: leaf + Merkle proof + root with anchor sanity check |
| #3969 | `issue-3969-get-bpt-page` | `internal/core/bootstrap/bptproof` (page.go) | — | Working: paginated BPT enumeration |
| #3962 | `issue-3962-account-loading` | `internal/core/bootstrap/loadtrack` | **5/5** | Working: concurrency-safe load tracker + OnAllLoaded callbacks |
| #3970 | `issue-3970-node-state-advertisement` | `internal/core/bootstrap/nodestate` | **5/5** | Working: state machine, advertisement payload, capability flags |
| #3965 | `issue-3965-bootstrap-persistence` | `internal/core/bootstrap/bootpersist` | **5/5** | Working: versioned envelope, atomic save, pin-mismatch + format-major guards |
| #3954 / #3961 | `issue-3954-3961-trust-bundle` | `internal/core/bootstrap/trustbundle` | **5/5** | Bundle struct + Verify with 2/3 quorum threshold; cryptographic per-sig verify stubbed |
| #3960 | `issue-3960-back-walking-validator` (← #3957) | `internal/core/bootstrap/backwalk` | **13/13** | Working: Walk orchestration, user-sig verification with external-signer flow, validator-quorum check with crypto-faithful Verify, synthetic Cause traversal, genesis termination, multi-entry chain |
| #3964 | `issue-3964-background-hydrator` (← #3958, #3969, #3962, #3970) | `internal/core/bootstrap/hydrator` | **5/5** | Working: 3-source loader, priority queue, end-to-end BOOTING → ACTIVE promotion |
| #3959 | `issue-3959-bootstrap-subcommand` | `cmd/accumulated/cmd_bootstrap.go` | — | Subcommand + interactive prompts wired; pipeline glue deferred |
| #3967 | `issue-3967-history-backfill` (← #3970) | `internal/core/bootstrap/backfill` | **3/3** | Working: ACTIVE → COMPLETE promotion; failure counting |

**Total:** 11 issue branches, **10 packages**, **48 passing unit tests** across the foundational packages.

## Cross-issue merges performed

- `issue-3960-...` merges from `issue-3957-...` (back-walker depends on ResolveKeyBookAt; merged in twice — once for the initial scaffold, once after #3957 grew forward replay).
- `issue-3964-...` merges from `issue-3958-...`, `issue-3969-...`, `issue-3962-...`, `issue-3970-...` (hydrator depends on BPT proof + page enumeration + load tracker + state machine).
- `issue-3967-...` merges from `issue-3970-...` (backfill emits ACTIVE → COMPLETE).

## What's working end-to-end

The integration branches demonstrate three end-to-end flows:

1. **`issue-3964-background-hydrator`** — full BOOTING → ACTIVE promotion:
   loadtrack reaches zero unloaded → hydrator signals → nodestate.PromoteToActive → state == ACTIVE. Test exercises priority-queue draining across the three sources.

2. **`issue-3967-history-backfill`** — ACTIVE → COMPLETE promotion: backfill walks backward from rolling-window edge; on hitting target depth, nodestate.PromoteToComplete fires; test asserts the transition.

3. **`issue-3960-back-walking-validator`** — Walk against a synthetic chain: stores a SystemGenesis transaction on a page's main chain; calls walker.Walk; result has GenesisTerm=true, Synthetic=true, correct Account/TxHash, and is memoized. Proves the full pipeline: chain reading, entry extraction, message lookup, type classification, synthetic verification, genesis termination, memoization.

## What's still deferred (next slices)

- **#3960 pinned-genesis-manifest cross-check**: GenesisTerm currently fires on `SystemGenesis`-typed earliest entries. Future: also accept any account/keybook present in the pinned-snapshot manifest, with a hash check.
- **#3961 per-signature crypto verify**: trustbundle.Verify counts threshold but stubs the per-signature crypto check. Wiring to protocol.SignatureType-specific verifiers is a localized change once the canonical bundle hash is defined.
- **#3958/#3969 API surface**: server-side functions exist; codegen entry in `pkg/api/v3/queries.yml` + dispatch case in `internal/api/v3/querier.go` is the next slice.
- **#3959 pipeline glue**: subcommand wires flags + prompts but `RunE` returns "not yet wired". Connecting to back-walker → BPT fill → hydrator → state machine is the integration step.
- **Simulator-based end-to-end tests**: the integration tests use synthetic in-memory fixtures; running against a real multi-block, multi-validator network requires the simulator harness.

## How to evaluate

```bash
# See all bootstrap branches:
git branch | grep -E "^  issue-39[5-7]"

# Most-integrated branch (covers loadtrack, nodestate, bptproof, hydrator):
git checkout issue-3964-background-hydrator
go test ./internal/core/bootstrap/...

# Most-evolved branch (covers keybookat + backwalk: 19 tests):
git checkout issue-3960-back-walking-validator
go test ./internal/core/bootstrap/...

# Per-package quick checks:
git checkout issue-3957-resolve-keybook-at        && go test ./internal/core/bootstrap/keybookat/
git checkout issue-3962-account-loading            && go test ./internal/core/bootstrap/loadtrack/
git checkout issue-3965-bootstrap-persistence      && go test ./internal/core/bootstrap/bootpersist/
git checkout issue-3970-node-state-advertisement   && go test ./internal/core/bootstrap/nodestate/
git checkout issue-3954-3961-trust-bundle          && go test ./internal/core/bootstrap/trustbundle/
git checkout issue-3960-back-walking-validator     && go test ./internal/core/bootstrap/backwalk/
git checkout issue-3967-history-backfill           && go test ./internal/core/bootstrap/backfill/
```

## Test count breakdown (current)

| Branch | Package | Tests |
|---|---|---|
| #3957 | keybookat | 7 |
| #3962 | loadtrack | 5 |
| #3970 | nodestate | 5 |
| #3965 | bootpersist | 5 |
| #3954/#3961 | trustbundle | 5 |
| #3960 | backwalk | 13 (+ keybookat 7 via merge) |
| #3964 | hydrator | 5 (+ loadtrack/nodestate via merge) |
| #3967 | backfill | 3 (+ nodestate via merge) |

Total unique tests: **48**. End-to-end orchestration covered on three branches.
