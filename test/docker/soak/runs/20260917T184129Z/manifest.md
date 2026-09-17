# Soak run 20260917T184129Z

**Purpose:** Validate the #4277 heartbeat and the restart anchor-serving fix over 12h at 100 tps with chaos on

| field | value |
|---|---|
| started (UTC) | 2026-09-17T18:41:29Z |
| commit | `5b782b51099d9fef0b536bdebab969006dbea98c` |
| describe | `10k-tps-744-g5b782b510` |
| branch | `dagbft-integration` |
| uncommitted files | 315 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:c2ff96553b706f75e0a9c11dbd61c24226cce0e0c84eaae93e946d3fea85c4a3` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | restart or pause one BVN container every 480s + 0-240s |
| topology | 3 BVNs, 12 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 12h |
| target TPS | 100 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Rate changed mid-run

At 2026-09-17T19:12:20Z the target rate was raised from 100 to 200 tps via the
loadgen control API, at Paul's request. The config frozen in `config/` says
`TPS=100`; that is what the run STARTED at, and anything measured after 19:12
is a 200 tps measurement. Instantaneous rate confirmed at 201.2 tps over the
minute to 19:18.

Consequence for what this run can claim: 100 tps for 12h is no longer what was
tested. Three BVNs are known to hold 100 tps for an hour (20260917T161555Z);
200 tps for twelve hours is unproven, so an end-of-run stall may be the rate
rather than either change under test.

Effect on CPU, from monitor.csv:

| window | samples | median cpu% | max cpu% |
|---|---|---|---|
| 100 tps (18:43-19:12) | 53 | 394 | 673 |
| 200 tps (19:12-19:18) | 12 | 542 | 719 |

## STALLED — delivery is dead on nine of eleven synthetic flows

Paul called it at ~19:55Z. I had checked block production and called the
network healthy, which was wrong: every partition keeps closing blocks at 1/s
with `stalledFor=0`, and cross-partition delivery has stopped underneath that.
Block cadence is not liveness.

At 20:02Z, flows sampled 45s apart (sent / received / delivered):

| flow | sent | recv | delivered | undelivered | delivered moving? |
|---|---|---|---|---|---|
| BVN1->Directory | 16267 | 16219 | 16219 | 48 | yes, the only one |
| BVN1->BVN2 | 38958 | 38692 | 19492 | 266 | **no** |
| BVN1->BVN3 | 39318 | 39184 | 31866 | 134 | **no** |
| BVN2->Directory | 38947 | 23190 | 23190 | 15757 | **no** |
| BVN2->BVN1 | 116212 | 55361 | 55361 | 60851 | **no** |
| BVN2->BVN3 | 231976 | 116063 | 35477 | 115913 | **no** |
| BVN3->Directory | 18635 | 15078 | 15078 | 3557 | **no** |
| BVN3->BVN1 | 37296 | 29682 | 29682 | 7614 | **no** |
| BVN3->BVN2 | 39687 | 31416 | 19146 | 8271 | **no** |

Two shapes. Out of BVN2 and BVN3 the destination is not even RECEIVING (recv
frozen with sent climbing). Out of BVN1 the destination receives but does not
EXECUTE (recv climbs, delivered frozen), so staging holds them: held entries
119,350 and growing, 50 MB.

**Healing cannot fix it, and that is the finding.** 37,095 requests, **0
answered, 0 healed entries**, every one refused as "source is lagging". The
refusal is on the source's own executor lag, so it is self-reinforcing: a node
that is behind refuses to serve, the holes its peers are waiting on never
fill, so they stay behind and refuse in turn. This is #4250 at 100% rather
than the 52-58% seen before.

**It does not drain when the load comes off.** Dropped to 10 tps at 20:02:11Z;
over the next 120s delivered advanced on BVN1->Directory alone (+122) and
nowhere else, while undelivered kept GROWING (BVN2->BVN3 +2579, BVN2->BVN1
+1786) — more than 10 tps can account for. So this is a wedge, not a
throughput backlog.

Not a deadlock in the Go sense: goroutine dumps from acc-bvn2-val1 and
acc-bvn3-val1 (`wedge-manual/`) show only the normal service loops parked on
channels, nothing piled on a lock.

**What this run cannot say.** The rate was moved 100 -> 200 -> 300 inside the
first 41 minutes, so the onset is not dated and neither the heartbeat (#4277)
nor the restart anchor-seed fix is cleared or convicted. Read latency was
still healthy at 200 (p50 1.6ms, p95 3.1) and degraded at 300 (p50 6.5, p95
44.2, max 246). The next run must hold one rate.

## Why execution gets stuck: the staged-proof budget saturates and overflow is discarded

Verified on acc-bvn3-val1 from its own metrics, sampled 90s apart at ~20:20Z.

A held entry executes only once proved. A proof that names a Directory block
this node has not executed yet is held in anchor staging until that anchor
runs. `maxStagedProofBlocks = 256` bounds how many distinct Directory blocks
one source may have proofs waiting on — its comment says "honest traffic waits
on a handful, so the bound only ever binds on a flood".

| metric (BVN3) | 20:19 | 20:21 |
|---|---|---|
| staged_proofs staged | 278 | 278 (frozen) |
| staged_proofs refused | 393 | 395 (growing) |
| staged_proofs validated | 7085 | 7087 (+2) |
| synthetic_anchor proven | 263318 | 263326 (+8) |
| synthetic_anchor collected | 8224 | 8249 (growing) |
| held entries, from BVN2 | 80552 | 80552 (frozen) |
| held anchors, from the DN | 1405 | 1494 (growing) |

`staged` is frozen while `refused` climbs: the budget is full and every new
proof is turned away. `refused` returns BadRequest, and the caller in
`exec_parallel.go` deliberately swallows BadRequest, so the proof is
discarded. Nothing re-sends it.

A slot frees only when the Directory anchor for that block executes — and
BVN3's own Directory anchor stream is backing up at the same time (1405 ->
1494 held). So the slots never free, every subsequent proof is refused, and
the entries those proofs would have proved are stranded: 80,552 from BVN2,
not moving, with no gap for healing to fill.

**The defect class is a bounded buffer whose overflow is dropped with no
recovery path** — the same shape as the batch-bytes defect fixed in #4159,
where one message type had no retry while headers, votes and certs all did.
The cap is correct as a bound; discarding on overflow is what makes it
terminal. Falling behind by more than 256 Directory blocks becomes
unrecoverable rather than slow.

What triggered the fall-behind here was the rate (300 tps), and that part is
not new. Whether the heartbeat changes how fast the budget fills is untested
and is the next thing to measure — it adds anchors, and anchors are what the
proofs name.

## Ended

Stopped by hand at 2026-09-17T20:5xZ, about 2h20 into a 12h run, once the
cause was understood. The run was already unusable as a validation of #4277:
the rate was moved 100 -> 200 -> 300 inside the first 41 minutes, so onset is
undated. Its value is the wedge it caught and the metrics that explain it,
which are written up above and filed as #4282.

Neither watchdog fired for the whole 2h20, because both classify a partition
as stalled from block height and every partition kept closing blocks at 1/s.
A delivery stall is invisible to them. Worth fixing: soakmon already computes
the red flow state and the "delivery STALLED" note, so only the wiring into
the wedge classification and stallkill is missing.
