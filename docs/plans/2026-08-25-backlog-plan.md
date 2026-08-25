# Development plan: clearing the deck

**Date:** 2026-08-25 · **Scope:** accumulate (277 open), core/Devnet (3),
core/staking (~40), core/accman (2 known, unfiled)

## The honest starting position

"Fix all 277 issues" is not a plan. Here is what the backlog actually is,
measured rather than assumed:

| | |
|---|---|
| open on accumulate | **277** |
| untouched > 120 days | **137** (49%) |
| filed in the last 30 days | 62 |
| carrying a `dagbft*` label | 98 |
| **unlabelled** | **84** |
| umbrella/epic issues | 8 |

Two facts should shape everything below.

**First: this has been attempted.** #3715, *"Repository Cleanup: Triage and
Reorganize 100 Open Issues"*, was opened on 2026-03-09 against a backlog of
100. It is still open. The backlog is now 277. A one-time triage sweep has
already been run and did not hold, so repeating it unchanged is not a plan
either — it is the same move at 2.8× the size.

**Second: the labels lie.** Of the 62 issues filed in the last 30 days, only
3 carry a `dagbft` label, yet most of #4125–#4164 are DAG-BFT and sharded
execution work. 84 issues carry no label at all. Any plan that routes work by
label will route it wrongly.

## Why #3715 did not hold, and what changes

Triage reduces a backlog once. It does not change the rate at which issues
arrive or the rate at which they are closed, so the backlog returns to its
equilibrium. Nothing in #3715 addressed either rate.

So this plan is ordered by **leverage**, not by issue count:

1. **Stop measuring wrong** — a fix you cannot verify is not a fix.
2. **Stop the backlog lying** — triage, but with a policy that holds.
3. **Then work the streams**, gated on the first two.

Phases 1 and 2 are small and mechanical. Phase 3 is the actual engineering,
and it is a program of months, not a sprint. Saying so up front is part of
the plan.

---

## Phase 1 — Fix measurement first (blocking everything else)

Nothing else should start until the harness can be believed. Right now it
cannot: the 8-hour v1.4.6.3 soak reported **"0 wedges"** while **1,515 drops
were induced**, and reported "0 errors / 0 stuck / 0 deferred" from metrics
that do not exist. A green panel currently means "not measured".

| Issue | Why it is first |
|---|---|
| **#4095** | node exports none of the cross-chain observability metrics — no synthetic counts of any kind exist |
| **#4114** | heals cards wired to a dead source |
| **#4110** | DI docker nodes serve no `/metrics` listener at all |
| **#4113** | harness declares a seized run clean (exit 0, "stalled channels: 0") |
| **#4126** | `soak.sh` freezes the wrong compose path; `runs/latest` makes every run alias the last |
| **#4158** | cold-start `docker compose up -d` returns nonzero and is swallowed |
| **#4108** | container healthcheck passes while the chain is stopped |
| **#4104**, **#4107**, **#4102**, **#4109** | `parallel-loadtest` reports success for rejected submissions; retire it as a measurement tool |
| **#4112**, **#4130** | loadgen accounting: timeouts vanish; accounts advertised spendable before funding lands |

**Exit gate:** a deliberately induced fault (drop N synthetics, pause a
partition) must show up as a non-zero number on the dashboard, and a clean run
must be distinguishable from an unmeasured one. Until that holds, no
behavioural fix below can be validated, and #4093/#4094 already showed how
easily "0" is mistaken for "healthy".

Roughly 12 issues. Mostly tooling, low risk, high leverage.

## Phase 2 — Make the backlog honest

Run against the 137 issues untouched for >120 days:

1. **Close with reason, don't silently drop.** Each gets one of: *fixed
   elsewhere* (link the commit), *superseded* (link the successor),
   *no longer applicable* (say why), or *still real* (label and keep).
2. **Label the 84 unlabelled.** Minimum: stream (`dagbft` / `mainline` /
   `tooling` / `protocol`) and kind (`bug` / `enhancement` / `cleanup`).
3. **Resolve the 8 umbrellas.** #3715, #3807, #3838, #3856, #3859, #4051,
   #4062, #4134 — each either becomes the tracking issue for a Phase 3 stream
   or is closed. An umbrella that tracks nothing is noise.
4. **Cross-link what is already fixed.** Example found while surveying:
   **#4030 "M1 — no bootstrap key rotation path"** is precisely the problem
   #4092 solved (merged), and nobody linked them. There will be more.

**Policy, so it holds this time** — the part #3715 lacked:

- An issue untouched for 180 days is closed automatically with a reason. It
  can be reopened; reopening is cheap, and a backlog nobody prunes is a
  backlog nobody reads.
- New issues require a stream label at filing.
- A WIP cap per stream. Work in progress beyond the cap means finishing
  before starting.

**Exit gate:** every open issue has a stream label and has been touched in
the last 180 days.

## Phase 3 — The work streams

Four streams, roughly independent, each with its own gate. Sizes are issue
counts, not estimates of effort.

### 3a. DAG-BFT / consensus (~98 issues)

The largest by far and effectively its own product line: #4041–#4164 plus the
`dagbft-integration` series. Contains genuine blockers — #4125 (certificates
commit but no partition produces a block), #4132 (95 of 100 transactions
vanish between accept and execute), #4128 (a node that falls behind can never
catch up), #4103 (halts under load while reporting healthy).

**Sequence within the stream:** correctness before performance. #4132, #4125,
#4128, #4131 before #4164, #4145, #4136.

**Gate:** #4100 — a 1 TPS containerised soak with provenance — cannot be
trusted until Phase 1 lands. This stream is therefore gated on Phase 1, not
merely sequenced after it.

### 3b. Mainline correctness (21 `bug`-labelled, non-DAG-BFT)

What runs on mainnet today: #4088 (panic on shutdown under load), #4086
(synthetic reconcile grace fails when anchoring lags), #4082 (unbounded slow
API queries), #4065 (v1.4.4.x never advertises acc-svc), #4067 (CometBFT p2p
map crash under churn).

The count is 21 by label, but labels undercount here — 84 issues carry no
label at all, so the true size of this stream is only known after Phase 2.

**Highest value in the whole plan**, because these affect the running network
rather than a line that has not shipped. Should run in parallel with 3a, not
behind it.

### 3c. Protocol / security review (#4027–#4038, 10 issues)

A coherent, already-scoped set from a security review: peerdb poisoning
(#4032), unsigned seed list (#4031), bootstrap key rotation (#4030 — **check
against #4092 before doing any work**), gateway CORS (#4036), role-combination
validation (#4035).

Small, well-defined, and several may already be fixed. Cheapest stream to
close out.

### 3d. Test health (~21 `testing`-labelled)

Flaky tests: #4083 (depends on the live Kermit testnet), #4080, #4076. Plus
the #4116 coverage tiers (#4120, #4121).

Flakes are corrosive: they train people to re-run rather than read. Worth
doing early and cheaply, before they mask a real failure in 3a or 3b.

---

## Other repositories

**core/accman** — two known, both unfiled, both from the v1.4.45 rollout:

1. **Deploys report failure when they succeed.** Every one of the five
   v1.4.45 deploys returned exit 255 or 52 while landing correctly:
   `accman-superv` restarts the remote superv, killing the connection serving
   the response. The code handles that at the response layer but not at the
   SSH/curl layer. A real failure is currently indistinguishable from this.
2. **Fleet version drift.** server2 is the only node on `accumulated
   v1.4.6.2`; the rest are v1.4.6 or v1.4.4-beta.4. Decide one version and
   converge.

**core/Devnet** — 3 open (#18 docs, #16 copyrights, #10 deploy real network).
Small enough to clear in one pass.

**core/staking** — ~40 open, none from recent work, not surveyed here. Needs
its own triage pass before it can be sequenced.

---

## Sequencing summary

```
Phase 1  measurement          ~12 issues   blocks everything
Phase 2  triage + policy      137 stale    parallel with Phase 1
Phase 3a DAG-BFT              ~98          gated on Phase 1
Phase 3b mainline bugs         21          parallel with 3a; highest value
Phase 3c security review       10          cheapest to close
Phase 3d test health          ~21          early, protects 3a/3b
```

Phases 1 and 2 are days. Phase 3 is months and should be resourced as such.

## What "clearing the deck" honestly means

The deck does not get cleared by fixing 277 issues. It gets cleared by:

- **~137 closed** in Phase 2 as stale, superseded, or already fixed
- **~12 fixed** in Phase 1 so that everything after can be verified
- **~128 remaining**, labelled, streamed, and prioritised — a real backlog
  rather than an archaeological record

That is the achievable outcome. Anything that promises 277 fixes is promising
something nobody can deliver, and #3715 is the evidence: it promised a
reorganisation of 100 issues and the number went up.
