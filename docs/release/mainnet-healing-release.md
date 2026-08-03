# Mainnet healing release — where we are

**Status:** blocked on a branching decision. Do not tag `main` as-is.
**Date:** 2026-07-25
**Goal:** cut a version of `accumulate` that carries the synthetic/anchor
self-healing work, and deploy it to mainnet.

---

## TL;DR

`main` is **not** a descendant of what mainnet is running. The deployed binary is
`v1.4.4.2`, which sits on a separate release lineage carrying 106 commits that
`main` does not have — including the entire tested healing implementation
(#4064/#4065/#4066/#4067). `main` has its own, *different* healing port
(cbd0ae4cc, #4058) plus 12 commits of its own.

Tagging `main` today and shipping it to mainnet would be a **regression**: it
would roll back the P2P/bootstrap fixes, the follower memory fix, and all four
soak-discovered healing defect fixes that are live right now.

Two lineages must be reconciled before anything is tagged. The merge is
tractable — 4 small conflicts, all in test/build files — and is in progress.

---

## The lineage split

```
                        2983b2f79  (merge base)
                       /          \
      12 commits      /            \      106 commits
                     /              \
              cbd0ae4cc          796a20696
              = main HEAD        = v1.4.4.2  ←  DEPLOYED ON MAINNET
              (#4058 healing     (#4064-#4067 healing,
               port, default-on)  P2P/bootstrap, soak fixes)
```

- `git merge-base main v1.4.4.2` → `2983b2f79`
- `git merge-base --is-ancestor v1.4.4.2 HEAD` → **false**
- `main` is 12 commits ahead of the merge base, `v1.4.4.2` is 106 ahead.
- `git describe` on `main` returns `v1.4.1-snapshot-35-gcbd0ae4cc` — the newest
  tag reachable from `main` is `v1.4.1-snapshot`. The whole `v1.4.4.x` series is
  off-lineage.

### What is actually deployed

From the fleet poller (`pop-os`, "Mainnet (local)", role `follower`):

| binary | version |
|---|---|
| `accumulated` | `v1.4.4.2` |
| `accman-superv` | `v1.4.42-3-g74eafcf` |
| `follower-monitor` | `v1.4.42` |

The follower is healthy and synced (100%, DN block 31773062, ~59 blocks/min),
deployed 2026-07-24 11:00. Its config is `accumulate.toml` — the **new `run`
framework** format, which matters below.

> Not yet confirmed: the versions on the actual mainnet *validators*.
> `darkwingdog` was unreachable at last poll. Confirm before scheduling a
> restart.

---

## Which healing lives where

These are two genuinely different mechanisms, not two copies of one.

### `v1.4.4.2` (deployed) — conductor receiver-pull, #4064

- `internal/core/crosschain/synthetic.go` (file does **not exist** on `main`)
- Config: `enable-synthetic-healing` / `enable-anchor-healing`, plumbed through
  `coreValidator` → `run/consensus.go` → conductor. Both default **off**.
- Observability: `ConsensusStatus.syntheticHeals` / `anchorHeals`, plus the
  `accumulate_crosschain_heals_total` metric.
- Validated: simulator e2e, plus a real multi-container docker deployment
  (2 validators/partition, 6 nodes + own bootstrap) proving wedge → heal.

### `main` (cbd0ae4cc) — executor-side paced healer, #4058

- Reuses the pre-existing `requestMissingSyntheticTransactions` in
  `internal/core/execute/v2/block/block_end.go`.
- Adds `healNeeded()` (fire only on a real nil-gap in a pending window) and
  `shouldAttemptHealing()` (one scan per `HealInterval`, default 10s, CAS-paced).
- Flips `internal/node/config` `Healing.Enable` to `*bool`, nil ⇒ on.
- Deliberately excludes the Kourou-gated collection-proof range recovery
  (#4048/#4056) and the late-anchor hold (#4070) as breaking.

`main` has **no** reference to `EnableSyntheticHealing` anywhere in the tree.

---

## Two problems found in `main`'s healing port

### 1. "Default-on" does not reach mainnet nodes

cbd0ae4cc sets the default in `internal/node/config/config.go` — the **legacy**
config path, consumed by `internal/node/daemon/run.go`:

```go
EnableHealing: d.Config.Accumulate.Healing.Enable == nil || *d.Config.Accumulate.Healing.Enable,
```

But mainnet nodes run the **new `run` framework** (`accumulate.toml`), and that
path is untouched by the commit:

- `cmd/accumulated/run/consensus.go:416` — `setDefaultPtr(&c.EnableHealing, false)`
- `cmd/accumulated/run/consensus.go:505` — `EnableAnchorHealing: Ptr(false)`,
  hardcoded, with a `// TODO Fix the flooding issues` comment and **no config
  exposure at all**.

So on the config format mainnet actually uses, `main`'s headline change is a
no-op: healing stays off, and anchor healing cannot even be turned on.
`v1.4.4.2` by contrast exposes both flags through `coreValidator`.

**This needs fixing before any release built from `main` is meaningful.**

### 2. The #4067 dispatcher race is present but not armed

`exp/tendermint/dispatcher.go` on `main` still has an unguarded map queue —
the defect that crashed four validators in 15 minutes during the soak. It is
currently *latent* on `main` because there is only ever one submitter:
conductor anchor healing is off, conductor synthetic healing does not exist, and
the executor healer allocates its own dispatcher per scan (`x.NewDispatcher()`
is a factory, so the healer's queue is goroutine-local and not shared with the
conductor's).

It is a cheap, obviously-correct fix and it comes along with the merge. It must
not be dropped — enabling a second submitter at any point arms the crash.

---

## The four soak-discovered defects, and `main`'s exposure

The 12-node/2-TPS chaos soak found four defects, all fixed on `v1.4.4.2`:

| # | Defect | Fixed on `v1.4.4.2` | State on `main` |
|---|---|---|---|
| 1 | Dispatcher concurrent-map race | `6fd74777e` | present, latent (see above) |
| 2 | Anchor-heal flood (1.24M resubmits / 25 min) | `812cdf254` | avoided — anchor healing hardcoded off |
| 3 | Heal storm (Delivered-based check-then-fire) | `bcb0a774e` | N/A — `main` keys on nil-gap, not Delivered |
| 4 | Permanent wedge on a transient error | `3b7f7d4a3` | not present — `main`'s loop uses `continue` |

`main`'s healer avoids #3 and #4 by construction and dodges #2 by leaving anchor
healing off. Only #1 carries over as real (latent) risk.

Caveat carried forward from the release line: **no completed 24h soak** validates
any of this yet. The campaign was stopped mid-flight while the shared driver was
being rewritten.

---

## Merge reconciliation — done, green, not pushed

Branch **`merge/release-1.4.4.2-into-main`** (commit `e0243d5fe`) holds the
completed merge. Local only — not tagged, not pushed, `main` untouched.

Gates all pass on the merged tree:

- `go build ./...` — clean
- `go vet` over `internal/database/...`, `test/validate/...`,
  `internal/core/execute/v2/block/...`, `exp/tendermint/...` — clean
- unit tests green: `internal/core/execute/v2/block`, `exp/tendermint`,
  `internal/node/config`

`git merge v1.4.4.2` into `main` produced **4 conflicts**, all in test/build
files. Every healing-relevant file auto-merged cleanly
(`executor.go`, `internal/node/config/config.go`, `internal/node/daemon/run.go`).

> **Auto-merged hunks need review too.** The merge silently dropped the
> `RUN go install github.com/go-delve/delve/cmd/dlv@latest` line from the
> Dockerfile build stage — it took the build stage cleanly from HEAD while the
> *conflicted* COPY line (resolved to the v1.4.4.2 side) depends on
> `/go/bin/dlv`. No conflict marker flagged it; the image build would simply
> have failed. Restored in `e0243d5fe`. Given 106 commits of drift, assume other
> clean auto-merges also warrant a read before release.

| file | resolution |
|---|---|
| `Dockerfile` | take `v1.4.4.2` (adds `accumulated-http`, `dlv`); verify the build stage actually produces `/go/bin/dlv` |
| `internal/database/snapshot/restore.go` | **union** — each side added a different field (`providedBatch`, `SkipHashVerification`); keep both |
| `test/validate/main_test.go` | take `v1.4.4.2` both hunks (`err =` not `err :=`, since `err` is already declared; faucet `Amount: 50000`) |
| `internal/database/state_test.go` | take `v1.4.4.2` if `sim.CreateLiteTokenAccount` is available on that type (it does not use the faucet, so it satisfies `main`'s #3860 concern); otherwise keep `main`'s explicit construction |

**Design question the merge surfaces:** the merged tree contains *both* healers.
They overlap but do not obviously conflict — and the dispatcher mutex arrives
with the merge, so the composition is not crash-prone. Still, running two
independent healers on the same ledgers should be a deliberate choice, not a
merge artifact. Recommend gating one of them off by default for the first
release and picking a single mechanism deliberately afterward.

---

## Versioning mechanics

There is no version constant to edit. Version is derived at build time:

- `Makefile`: `GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)`
- injected via ldflags into `accumulate.Version` (`version.go`)
- `.gitlab/release.gitlab-ci.yml` fires the release jobs on `$CI_COMMIT_TAG != null`,
  building and cosign-signing the production image and the binaries.

So **"versioning accumulate" = creating and pushing a git tag.** The tag must be
on a commit that is a superset of what is deployed.

Note this is the protocol-independent part: no `ExecutorVersion` bump is
involved. cbd0ae4cc is explicitly version-independent, and the breaking,
activation-gated work (#4048/#4056 collection proofs, #4070) is excluded.

---

## Recommendation

**Two paths. They differ in risk, not in destination.**

### Path A — reconcile, then tag `v1.4.5` from `main` *(recommended)*

1. ~~Finish the merge of `v1.4.4.2` into `main`~~ — **done**, branch
   `merge/release-1.4.4.2-into-main` (`e0243d5fe`), build and tests green.
2. Fix the run-framework gap: expose `enable-healing` / `enable-anchor-healing`
   through `coreValidator` and decide their defaults there, so the config format
   mainnet actually uses can turn healing on.
3. Decide the one-healer-or-two question.
4. Build, run the docker deployment test, then the definitive soak.
5. Tag `v1.4.5` — a real minor bump, since the result is a strict superset of
   `v1.4.4.2` plus `main`'s security and concurrency fixes. `v1.4.4.3` would
   understate it.

Correct going forward; ends the lineage split permanently. Costs a soak cycle.

### Path B — urgent deploy from the release lineage

Cut `v1.4.4.3` from `release-1.4.4.2` with only the needed `main` fixes
cherry-picked (the strongest candidates are `1b663da68` concurrent map writes,
`6879d19ff` snapshot restore concurrency, `d6ddea514`/`28f87d9a1` math/rand and
ARM64 crypto). Lowest risk — it stays on the lineage that is deployed and soak-
tested — but it leaves `main` diverged and defers the reconciliation again.

**Recommended: Path A**, with Path B held in reserve if a mainnet incident forces
a same-day ship. Either way the reconciliation in Path A steps 1–3 is work that
has to happen; Path B only changes whether it gates the deploy.

---

## Open items

- [ ] Confirm the `accumulated` version on the real mainnet **validators**
      (`darkwingdog` was unreachable; only the local follower is confirmed at
      `v1.4.4.2`).
- [ ] Decide Path A vs Path B.
- [ ] Fix the run-framework healing config gap (blocks any `main`-based release
      from doing anything).
- [ ] Decide one healer or two in the reconciled tree.
- [ ] Run the definitive soak with the rewritten all-tx-type driver — no
      completed 24h run validates the healing fixes yet.
- [x] Merge `v1.4.4.2` → `main` resolved and verified
      (`merge/release-1.4.4.2-into-main` @ `e0243d5fe`).
- [ ] Nothing is tagged or pushed. All work so far is local and reversible.
      `main` is unchanged; the merge lives on its own branch.
