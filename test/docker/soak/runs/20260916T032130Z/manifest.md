# Soak run 20260916T032130Z

**Purpose:** Verify #4248: the in-flight window widens by the answering node's execution lag, so a less-lagging node stops answering for entries the leader has not sent. 1h, 500 tps, no chaos. Watch heal entries/min against accumulate_dagbft_execution_lag_blocks -- under the old rule they rose together.

| field | value |
|---|---|
| started (UTC) | 2026-09-16T03:21:30Z |
| commit | `32f46ece5f700f3dc9046f9af39ca9a814c848fb` |
| describe | `10k-tps-704-g32f46ece5-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 270 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:e2c023ee0a82a63eb2b4c26cdc1a0344d794367a4baa7f6ae8ab9cf97b4a597e` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 1h |
| target TPS | 500 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf, the compose and network files). Results appended below on exit.

## Result — stopped at 24 min (of 1 h), deliberately

Stopped early: the run had already answered the question it was launched for,
and the throughput slowdown after ~23 min is a known, separate problem (#4258).

**What it was testing.** #4248: the healing in-flight window widened by the
answering node's execution lag, so a node less behind than the leader stops
answering for entries the leader has not sent (commit b2a94f625, merged
32f46ece5).

| t | lag (max) | heal entries | answered | not-yet | miss | failed | tps |
|---|---|---|---|---|---|---|---|
| 4 min | 0 | 0 | 0 | 99 | 0 | 0 | 500 |
| 10 min | 0 | 0 | 0 | 216 | 0 | 0 | 497 |
| 13 min | 6 | 0 | 0 | 275 | 0 | 0 | 497 |
| 16 min | 12 | 141 | 3 | 335 | 0 | 0 | 494 |
| 19 min | 4 | 141 | 3 | 397 | 0 | 0 | 482 |
| 22 min | 10 | 332 | 5 | 463 | 0 | 0 | 468 |
| 24 min (stop) | 3–4 | 1 811 | 29 | 519 | 0 | 0 | 463 |

Reference, soak 20260906T134054Z at 500 tps with no faults: 123 heal entries at
17 min, 7 493 at 31 min, 75 936 at 51 min.

**Verdict: the fix reduces the amplification substantially but does not remove
it.** Heals stayed at zero through the first twelve minutes and through the
first appearance of lag, which is new — the reference was already climbing by
then. But they track lag once it arrives, reaching 1 811 by 24 min against the
reference's ~7 493 at 31 min. Zero misses and zero failures throughout, so
nothing was lost and no stream stranded.

**Why it does not remove it, and it is the predicted reason.** The window
widens by the *answering* node's lag. When the answering node is caught up and
the leader is behind, the estimator gives zero margin and the node answers
anyway — stated as the known limitation in healing.md, "The in-flight window
belongs to the sender". 29 answers got through on that path.

This is the argument for #4253 (P6): send at begin(N+1) with the source-root
proof and let the destination complete it, which takes the leader's lag off
the path entirely and makes the in-flight window moot.

Also measured, and unrelated to healing: the load generator was refused
231 698 of 655 160 submissions (35%), and accepted load decayed 500 -> 463 tps
as execution fell behind. That is #4250 (back-pressure without hysteresis) and
#4258 (the throughput ceiling), untouched by this change.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-16T03:51:13Z |
| elapsed | 0.4h |
| driver exit | 1 (FAILED) |
| dn height | 10 -> ? |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 46 |
| seizure | SEIZED at 2026-09-16T03:28:10 :: stuck=n/a stuckStream= worst=BVN2->BVN1 gap=248 deliv=7217 undeliv=synthetic BVN2->BVN1 undeliv=855 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| read-back probe | Whole run: 4950 timed reads, p50 1.5 ms, p95 9.6 ms, p99 41.0 ms, max 1156.2 ms (txn read, BVN2, entry 846 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
