# Soak run 20260828T153810Z

**Purpose:** #4169 step 0 baseline + step 7 runtime proof on 289c5f7e1 (staging owns the ledger write, cascade deleted, step-0 metrics). 8-node 2-BVN topology, chaos OFF, 100 tps, ACC_EXECUTION_SHARDS=8 so the serial/parallel split is real. Questions: (0a) serial share of ProcessAll wall time — <25% means group 4 has nothing to win; (0b) flushes per block; (0c) synthetic proving-anchor co-arrival — <5% means the anchors-first round is not paying. Step 7 proof: no stream stalls over 12h with the per-block ledger write. Watch the new step 0 dashboard group and the flow matrix for undelivered growth.

| field | value |
|---|---|
| started (UTC) | 2026-08-28T15:38:10Z |
| commit | `289c5f7e1ca345da549bf830b3b09a71ec1b5d2a` |
| describe | `10k-tps-565-g289c5f7e1` |
| branch | `4169-step6-staging-drives-execution` |
| uncommitted files | 0  |
| image | `disoak-bvn1-val1` |
| image id | `sha256:189379683f83566623d2202a7ca6c2420f6b295c25c7cacaf8cf458f807bc88a` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 100 |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-28T17:14:21Z
- reason: stalled 245s: BVN1,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-28T17:14:21Z |
| elapsed | 1.46h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 1634 |
| heals | 0 -> 56894 |
| chaos events | 1 |
| monitor samples | 18 |
| seizure | SEIZED at 2026-08-28T16:00:34 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=266 undeliv=synthetic BVN2->BVN1 undeliv=849 |
| reconcile pulls (#4073) | 429 |
| stalled channels at end | 4 |
| wedge captures (#4125) | 1 wedge-20260828T170908Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Reading (written at teardown, 2026-08-28T17:20Z)

**Cause of death: consensus, not execution.** stallkill fired at 17:14:17 on
`stalled 245s: BVN1,Directory`. From ~17:05 every node sat at 1.11–1.24 GiB RSS
against GOMEMLIMIT=1200MiB (the GC-ceiling collapse recorded on 08-24), and
bvn2-val1's log showed the batch-dissemination spiral: `Evicted batches due to
storage limit (LRU) … skippedOwnUncommitted=683` → `Missing batch for header —
deferring vote` → `Re-proposed uncommitted batches` → worker `ErrBackpressure`
at the API; the Directory DAG stuck at round 3271 re-sharing certificates.
Execution was ~0.35 s per block against 3–5 s rounds (bvn2-val1: 1087 s of
ProcessAll over 3148 blocks) — consensus was not waiting on it.

**Execution side held (#4169 steps 6–8, first live evidence, 1.46h — not a
claim).** `streams-final.txt`: on all four streams received == delivered
(139,897 / 139,897 on the heavy BVN2→BVN1 stream). The "UNDELIVERED" column is
produced − received, i.e. messages the source had not yet dispatched/the
destination had not yet received — transport lag under backpressure — not a
staging backlog. The pending windows were empty at the end. The 08-25 8-node
runs at 250 tps seized on "stalled stream BVN2→BVN1 undeliv=1204" in 30 min
with the cascade; not a controlled comparison (different tps).

**Step 0 (1.46h, fleet sums off the nodes at teardown — indicative only):**

| gate | reading | threshold |
|---|---|---|
| 0a serial share | 26.7% (2581 s serial / 7096 s parallel) | 25% |
| 0b flushes/block | 3.54 (92,576 / 26,150) | — |
| 0c co-arrival | 0.002% (8 this_block / 307,598 earlier / 64,976 missing) | 5% |

0c: effectively no synthetic is ever admitted by an anchor applied in the same
block; 17.4% were judged while their anchor was still missing (waited a block).
0a sits right at the gate and is a floor (the parallel bucket includes each
flush's serial commit). Needs the 12h run to be called.

**Next run:** same everything, GOMEMLIMIT=2GiB — one factor. If it dies the
same way with RSS well under 2 GiB, memory is exonerated and the 32 MB
per-partition batch store is the next suspect.

**Correction (17:58Z):** "GOMEMLIMIT=2GiB" alone is wrong — docker-compose.yml
caps every node at `mem_limit: 1536m` and documents that GOMEMLIMIT MUST stay
below it, or the soft limit never engages and the kernel OOM-killer replaces
the GC (that was the 2750MiB mistake). The one-factor rerun is the memory
BUDGET: `mem_limit` 2560m and `GOMEMLIMIT=2GiB` together, everything else
identical.
