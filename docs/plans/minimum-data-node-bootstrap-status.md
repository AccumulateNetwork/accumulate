# Minimum-Data Node Bootstrap — Implementation Status

**Branch:** `minimum-bootstrap-launch` (integration baseline; design doc + back-walk probe)
**Companion:** `docs/plans/minimum-data-node-bootstrap.md`
**Tracking issue:** #3953

This file tracks the per-issue branches and their state. Each branch is a self-contained slice — explore individually with `git checkout <branch>` and `go test ./internal/core/bootstrap/<pkg>/`.

## Per-issue branches

| Issue | Branch | Package(s) | Tests | Status |
|---|---|---|---|---|
| #3957 | `issue-3957-resolve-keybook-at` | `internal/core/bootstrap/keybookat` + chain wrapper | — | Scaffold; forward replay deferred |
| #3958 | `issue-3958-get-bpt-leaf` | `internal/core/bootstrap/bptproof` | — | Working: leaf + Merkle proof + root |
| #3969 | `issue-3969-get-bpt-page` | `internal/core/bootstrap/bptproof` (page.go) | — | Working: paginated BPT enumeration |
| #3962 | `issue-3962-account-loading` | `internal/core/bootstrap/loadtrack` | **5/5** | Working with concurrency-safe load tracker + OnAllLoaded callbacks |
| #3970 | `issue-3970-node-state-advertisement` | `internal/core/bootstrap/nodestate` | **5/5** | Working: state machine, advertisement payload, capability flags |
| #3965 | `issue-3965-bootstrap-persistence` | `internal/core/bootstrap/bootpersist` | **5/5** | Working: versioned envelope, atomic save, pin-mismatch + format-major guards |
| #3954 / #3961 | `issue-3954-3961-trust-bundle` | `internal/core/bootstrap/trustbundle` | **5/5** | Bundle struct + Verify with 2/3 quorum threshold; cryptographic sig-verify stubbed |
| #3960 | `issue-3960-back-walking-validator` (← #3957) | `internal/core/bootstrap/backwalk` | **4/4** | Skeleton: memoization, cycle detection, depth bound; per-entry verify deferred |
| #3964 | `issue-3964-background-hydrator` (← #3958, #3969, #3962, #3970) | `internal/core/bootstrap/hydrator` | **5/5** | Working: 3-source loader, priority queue, end-to-end BOOTING → ACTIVE promotion |
| #3959 | `issue-3959-bootstrap-subcommand` | `cmd/accumulated/cmd_bootstrap.go` | — | Subcommand + interactive prompts wired; pipeline glue deferred |
| #3967 | `issue-3967-history-backfill` (← #3970) | `internal/core/bootstrap/backfill` | **3/3** | Working: ACTIVE → COMPLETE promotion; failure counting |

**Total:** 11 issue branches, **9 packages**, **32 passing unit tests** across the foundational packages.

## Cross-issue merges performed

- `issue-3960-...` merges from `issue-3957-...` (back-walker depends on ResolveKeyBookAt).
- `issue-3964-...` merges from `issue-3958-...`, `issue-3969-...`, `issue-3962-...`, `issue-3970-...` (hydrator depends on BPT proof + page enumeration + load tracker + state machine).
- `issue-3967-...` merges from `issue-3970-...` (backfill emits ACTIVE → COMPLETE).

## What's not yet done (next slices)

- **#3957 forward replay**: walk a keybook's main chain forward, applying `UpdateKeyPage` operations via `ApplyKeyPageOperation` (already exported on this branch) up to a target block time. Current scaffold returns `ErrNotYetImplemented` for any time before the most recent mutation.
- **#3960 per-entry verification**: implement the two verification rules (user keypage signatures via lateral signature-chain navigation; validator quorum on synthetic-anchoring transactions). Skeleton, memoization, and cycle detection are in place.
- **#3961 cryptographic signature verification**: bundle `Verify` currently stubs the per-signature crypto check while exercising threshold counting. Wiring to `protocol.SignatureType`-specific verifiers is a localized change.
- **#3958/#3969 API surface**: server-side functions exist; codegen entry in `pkg/api/v3/queries.yml` + dispatch case in `internal/api/v3/querier.go` is the next slice.
- **#3959 pipeline glue**: subcommand wires flags + prompts but `RunE` returns "not yet wired". Connecting to back-walker → BPT fill → hydrator → state machine is the integration step once the components mature.
- **`accumulated bootstrap` tests**: the subcommand has no tests yet; will land alongside the pipeline glue.

## How to evaluate

```bash
# See all bootstrap branches:
git branch | grep -E "^  issue-39[5-7]"

# Run all bootstrap-package tests across the integration branches.
# (Each branch's tests pass on that branch; integration is via merges.)

# Most-integrated branch (covers loadtrack, nodestate, bptproof, hydrator):
git checkout issue-3964-background-hydrator
go test ./internal/core/bootstrap/...

# Other interesting per-branch checks:
git checkout issue-3962-account-loading      && go test ./internal/core/bootstrap/loadtrack/
git checkout issue-3965-bootstrap-persistence && go test ./internal/core/bootstrap/bootpersist/
git checkout issue-3970-node-state-advertisement && go test ./internal/core/bootstrap/nodestate/
git checkout issue-3954-3961-trust-bundle    && go test ./internal/core/bootstrap/trustbundle/
git checkout issue-3960-back-walking-validator && go test ./internal/core/bootstrap/backwalk/
git checkout issue-3967-history-backfill     && go test ./internal/core/bootstrap/backfill/
```
