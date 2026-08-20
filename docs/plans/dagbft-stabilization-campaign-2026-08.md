# DagBFT stabilization campaign — what we are doing and why

**Status:** active. Written 2026-08-20 on `issue-4105-collection-proof-delivery`
(code tip `1827dc587`). Companion to
`docs/bugs/dagbft-soak-collapse-2026-08.md`, which is the post-mortem this
campaign answers. This document is the plan of record: what changed, what is
running right now, what each result decides, and what is deliberately deferred.

---

## 1. Objective

Make the DagBFT integration branch survive sustained load — first without
chaos, then with it — with every claim backed by run artifacts, and every
failure visible on instruments rather than reconstructed from log archaeology.

Two termination conditions:

- **Success:** a 12h chaos soak passes with converged delivery, flat resource
  trends, and no false-clean verdicts — the gate the week-long churn plan
  (`docs/plans/dagbft-week-churn-soak.md`) has always required.
- **Redirect:** the evidence shows the remaining defect is structural (e.g.
  primary delivery needs the #4105 collection-proof delivery path finished
  before soaking is meaningful) — then the campaign pauses and the structural
  work becomes the task.

## 2. Where we started (one paragraph)

The 2026-08-19 12h soak collapsed at 4.6h — not from chaos, but from its own
recovery machinery: primary cross-partition delivery under-delivered from
minute 12, the healers stormed, the unconfigured libp2p resource manager
exhausted, one missing error case poisoned peer tracking network-wide, and the
Directory froze twice while every healthcheck stayed green. Full mechanism
chain, with file:line, in the post-mortem. Issues: #4111 (root), #4115 (chain),
#4103, #4108, #4110–#4114, #4086.

## 3. What we changed — the fix stack

All on `issue-4105-collection-proof-delivery`, one commit per mechanism:

| commit | change | answers |
|---|---|---|
| `23fdd5388` | `classifyDialError`: resource-limit errors are **local** — never mark the remote peer bad. `tryDial` backoff resets on success; cap 24h → 15m. | the latch that made the collapse permanent (§2.5–2.6 of the post-mortem) |
| `fa5e7d645` | Explicit resource manager: transient outbound streams 256 → 2048, system 8192; per-scope usage exported as `libp2p_rcmgr_*` Prometheus metrics. | transient-scope exhaustion (§2.4) |
| `3a339b474` | Healing bounded: per-remote circuit breaker (3 consecutive failures → 15s…5m backoff), exclusive scans (no stacking past the 3s block interval), `healAnchors` capped at 16 re-drives per scan. | the heal storm (§2.3) |
| `356196096` | Dispatcher requeues on worker backpressure (up to 10 Send cycles) instead of dropping the envelope. | anchors dying with user traffic (§2.2) |
| `09d708017` | Every docker node serves `/metrics` on :26670 (#4110). | the blind node panels; makes every other metric real |
| `c04de643d` | Harness: `compose logs -f` streamed into the run dir from network-up (rotation-proof), per-container CPU/mem in `stats.csv`. | the lost first 3 hours of evidence (§3.1) |
| `1827dc587` | Reconcile pull spin fixed: pulls feed/respect the breaker; `NotFound` ("reached the end of the chain") is deterministic — no 3× retry, skip the rest of the range (#4086). | found by the shakedown: 157k failed pulls in 17 min |

**Deliberately deferred** (structural; revisit after the current runs):

- DAG-BFT consensus onto its own libp2p host — `pkg/consensus/p2p` exists with
  connmgr watermarks and is unused; today consensus shares the API host
  (`dagbft.go:373`) and shares its fate.
- Priority classes in the DAG-BFT worker queue so system messages (anchors,
  synthetics) never queue behind user transactions. The requeue is the
  band-aid; this is the cure.

## 4. The validation ladder

Each rung runs only if the one below held. Everything lands in
`test/docker/soak/runs/<ts>/` with a manifest; the dashboard (:8099) is open
and visible before load starts, always.

### Rung 1 — 17-minute chaos-free shakedown ✅ (run `20260820T050616Z`)

Purpose: does the fix stack behave at all. Result:

| signal | collapse run (4.6h) | shakedown (17 min) |
|---|---|---|
| `resource limit exceeded` | 1,358,047 | **0** |
| `worker backpressure` | 213,000+ | **0** |
| `no live peers` (node-side) | 240,023 | **180** (startup, no latch) |
| `/metrics` endpoints | 0 of 12 | **12 of 12** |
| breaker activations | n/a | 0 (pulls succeeding — correct) |

Two things it did **not** fix, exactly as predicted: fleet CPU still pinned
~2,200% (healing still carries all delivery), and **every synthetic stream
showed produced ≫ delivered** — `BVN1→dn: 11 produced, 0 delivered` — with
healthy transport and zero chaos. That is #4111 isolated: primary delivery
itself is the defect. Submissions succeed into the DAG-BFT service (66,955
accepted, 51 rejected in the run) and vanish between consensus intake and
destination execution. The shakedown also caught the reconcile spin
(`1827dc587`) — visible only because the rotation-proof capture existed.

### Rung 2 — 12h chaos-free, 10 TPS, 3s blocks ▶ RUNNING (run `20260820T053226Z`)

Started 05:32 UTC 2026-08-20, tip `1827dc587`, chaos disabled, dashboard live.
This is the #4111 long-run experiment with working transport. Watch list and
pass criteria:

1. **Delivery convergence:** does per-stream `delivered` approach `produced`,
   or does the gap grow without bound? (streams matrix + `monitor.csv`)
2. **Breaker behaviour:** "Healing circuit open" events should be rare and
   self-clearing; a breaker stuck open marks a source that cannot serve.
3. **Resource trends:** goroutines (baseline ~1,081/node) and RSS (~100 MiB)
   flat over 12h — the #4089-class leak check, measurable for the first time.
4. **rcmgr transient scope:** peak usage vs the new 2048 limit — headroom or
   near-miss.
5. **No false-clean:** if seizewatch seizes, the verdict must say so.

Outcomes → next step:

- Delivery converges, resources flat → **rung 3**.
- Delivery gap grows forever → #4111 needs the delivery-path fix *before* any
  more soaking: instrument the DAG-BFT worker → proposal → execution path to
  find where accepted submissions vanish, and/or finish #4105's
  collection-proof delivery. Soaking pauses.
- Resource creep → new leak issue, fix, re-run rung 2.

### Rung 3 — 12h WITH chaos (the original failed run, retried)

Same load, default chaos cadence (pause/restart every ~8–12 min). Pass = what
rung 2 passed plus: discovery recovers after every disruption (the 20260819
failure mode), no seizure, verdict clean *and honest*.

### Rung 4 — the week-long churn soak

Per `docs/plans/dagbft-week-churn-soak.md`, which explicitly forbids starting
until the above pass. Membership churn (join/promote/drop) is still blocked on
#4059/P3 — that remains true and is not this campaign's scope.

## 5. Open investigation: where do accepted submissions go? (#4111)

The sharpest open question. Facts so far: envelopes reach
`SubmitterService.Submit()` and `service.SubmitTransaction` successfully;
destination ledgers show near-zero `received`; per-source variance exists
(BVN3's streams mostly delivered, BVN1's delivered zero); the sources' own
sequencers answer "reached the end of the chain" for messages their ledger
says were produced — meaning the *source's* servable chain lags its produced
counter, which also starves pull-healing (#4086 interaction). Hypotheses to
kill in order: (a) submissions accepted by a node whose worker feeds the
wrong/no partition DAG; (b) proposals made but transactions dropped between
DAG commit and executor delivery; (c) executed but sequence-gated waiting on
signature thresholds that never assemble. The rung-2 logs (complete, for the
first time) are the dataset.

## 6. Operating notes

- **Worktree:** `/home/paul/repos/gitlab.com/accumulatenetwork/di`.
- **Dashboard:** `http://127.0.0.1:8099`, opened before load; soak.sh refuses
  to run unmonitored.
- **Run provenance:** every run under `test/docker/soak/runs/<ts>/` with
  manifest, frozen config, and now complete logs. Commit run evidence
  immediately; GitLab rejects blobs > 1 MiB, so `node-logs*`,
  `container-logs/`, and `reconcile-pulls.txt` stay local (gitignored) —
  commit summaries instead. **Check staged file sizes before pushing run
  dirs.**
- **Issue trail:** findings land on #4115 (transport chain), #4111 (delivery),
  #4103 (DN freezes) as they happen; the post-mortem doc is the index.
