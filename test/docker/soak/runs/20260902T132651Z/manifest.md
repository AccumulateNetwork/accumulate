# Soak run 20260902T132651Z

**Purpose:** SOAK-12H-500-CACHE: 12h CHAOS soak at 500 tps on the sharded store with the #4186 read cache (200,000 entries a generation, 400,000 resident) on top of the Account(U).Url routing fix. Caches the write-once records the executor reads every block: Account(U).Url, Message(H).Main and Transaction(H).Main (what healing reads), and the anchor/anchor-sequence chain elements and mark points. Profiled cost it targets, from run 20260902T041031Z: Syscall6 18.45% of CPU, pread 16.5%, and 71.8% of the preads were segment.bloomTest. Measure: cacheHitPct and cacheGenerations in storage-stats -- many turnovers against a low hit rate means 200k is still too small; RSS against the ~800-930 MiB the last run held, since the cache is estimated at 100-150 MB. Chaos is ON specifically so healing runs: the simulator never drops a synthetic, so the Message/Transaction reads healing does are unmeasured until now. Known: BVN2 wedged at 20 min in the last three runs and this change does not address that.

| field | value |
|---|---|
| started (UTC) | 2026-09-02T13:26:51Z |
| commit | `dc92ddda8bb666eeaf66f89a49b9126b53799ef0` |
| describe | `10k-tps-625-gdc92ddda8` |
| branch | `issue-4186-bcdb-read-cache` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:2774a759039ce7907421f823655907c4bb8d2bf03e8ba5956ba119697c2090c2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-02T13:51:11Z
- reason: stalled 240s: BVN1,BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-02T13:51:11Z |
| elapsed | 0.35h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 208 |
| heals | 0 -> 100793 |
| chaos events | 4 |
| monitor samples | 5 |
| seizure | SEIZED at 2026-09-02T13:41:03 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=2228 deliv=48576 undeliv=synthetic BVN2->BVN1 undeliv=5657 |
| reconcile pulls (#4073) | 611 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 3844 timed reads, p50 1.7 ms, p95 105.8 ms, p99 466.7 ms, max 8040.6 ms (chain read, BVN1, entry 192 blocks old); 11 failed, 11 timed out (8s), 2 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260902T134916Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Verdict: the synthetic stream livelocks against the pending-receipt bound

Seized at 21 minutes, `BVN2->BVN1 gap=2228`. All three partitions were LIVE at
the time (DN 3.0 s/block, BVN1 3.1, BVN2 3.9) -- this is not the wedge the
previous runs died of. It is synthetic delivery failing to advance.

The number that names it: on two samples eight minutes apart,

    recv 64,478 - deliv 60,382 = 4,096
    recv 64,885 - deliv 60,789 = 4,096

`MaxPendingSequenced = 4 * maxRunPerBlock` = 4,096 (`msg_sequenced.go:289`).
The pending window is pinned at its cap, exactly, while delivery crawls -- 407
messages in eight minutes against a source that had produced 75,583.

`stream_position.go:174` refuses to RECORD a receipt beyond
`delivered+MaxPendingSequenced`, silently (`return nil`, logged at Debug).
Meanwhile the healer is working correctly: the "Requested missing synthetic
transaction" line is emitted AFTER `c.submit()` succeeds
(`crosschain/synthetic.go:447`), so each one is a message fetched from the
source and re-submitted to the destination. There were 53,011 of them for
**8,556 distinct sequence numbers**, spanning 47,509-65,479, some re-healed
**41 times**.

So the loop is: heal a message beyond the window -> the receipt is dropped
without error -> the reconciler still sees it missing -> heal it again. 44,206
heals with `errors 0`, because nothing errors. The loadgen meanwhile takes
`worker backpressure` because the executor is saturated with healing traffic
that cannot advance anything.

The bound is not the defect. Bounding receipt state has to survive. What is
missing is that healing is not ordered by what would ADVANCE the delivery
point: healing 65,479 while 47,509 is the hole costs a fetch, a submit and a
block slot, and accomplishes nothing.

**NOT PROVEN**: that the refusal branch is what drops them. It logs at Debug
and the run was at INFO, so this is arithmetic and behaviour consistent with
that branch, not the log line. Promoting that one line to Info settles it.

## The read cache (#4186), which this run was launched to measure

| | |
|---|---|
| raw hit rate, 8 BVN engines | 11.2% (1,420,352 hits / 11,275,760 misses) |
| hit rate on records that EXIST | ~57%, from two live samples 12 min apart |
| generations | **0** -- never turned over |
| entries | 617,288 across 8 engines, ~77k each against a 200,000 limit |
| RSS | 1.0-1.1 GiB, against 800-930 MiB the previous run |

Size was never the constraint: nothing was ever evicted, at 38% of the limit.
The raise from 20,000 to 200,000 could not have helped and neither would more.

The raw rate is low because most reads of a cacheable shape are for records
that are NOT THERE -- ~94% of misses at the samples taken. A cache cannot serve
those, and caching the absence would answer "not here" forever for a record
that is simply written later. `cacheAbsent` (7ab259589) measures this directly
from the next run; here it is inference from `entries` against `misses`.

Two shapes still reached history: `Message.(hash).Main` and
`AnchorChain.directory.Root.ElementIndex`. The first is the healer reading
messages old enough to have left the window -- which is what a stream 8,556
messages behind would do.

## What the read path is NOT

Checked, because the previous run's write-up pointed at it: the store's filters
rule out **97.8%** of permanent-layer lookups without walking any segment
(`filterAbsent` 25,215,333 of 25,771,377; `filterWalked` 2.2%; `permWalkPct`
2.16). Absent reads do not walk. The only walk worth attacking is
`filterMisled` -- 1.2% of lookups that walked on a filter's say-so and found
nothing -- which is a false-positive-rate question (k=3, 12 bits/key), not a
caching one.
