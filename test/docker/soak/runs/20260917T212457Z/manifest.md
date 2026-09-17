# Soak run 20260917T212457Z

**Purpose:** 30m proof at 100 tps, chaos every ~3m: #4284 healing guard, #4283 heal accounting, #4282 byte budget, #4277 heartbeat+restart seed

| field | value |
|---|---|
| started (UTC) | 2026-09-17T21:24:57Z |
| commit | `cc48ba1a6d87f8d0de5e545e19202414cf6faf25` |
| describe | `10k-tps-749-gcc48ba1a6` |
| branch | `issue-4283-heal-accounting` |
| uncommitted files | 335 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:0d4e79a83b0602147a3b67c61a182531fa47d2177b6a0ed8b3f9571e8eb1d0b3` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | restart or pause one BVN container every 180s + 0-60s |
| topology | 3 BVNs, 12 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 30m |
| target TPS | 100 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-17T21:46:38Z |
| elapsed | 0.33h |
| driver exit | 0 (clean) |
| dn height | 36 -> 1179 |
| heals | 0 -> 0 |
| chaos events | 12 |
| monitor samples | 37 |
| seizure | SEIZED at 2026-09-17T21:31:08 :: stuck=n/a stuckStream= worst=BVN2->BVN3 gap=351 deliv=2673 undeliv=synthetic BVN2->BVN3 undeliv=148 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| read-back probe | Whole run: 4076 timed reads, p50 1.4 ms, p95 2.9 ms, p99 6.0 ms, max 21.0 ms (chain read, BVN2, entry 332 blocks old); 9 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## What this run showed (read against the numbers above, two of which are wrong)

**Healing ran in Docker for the first time on this line, and it worked.**
Chaos restarted acc-bvn3-val2 at 21:30:34Z. seizewatch flagged a hole on
BVN2->BVN3 at 21:31:08Z (gap 351, delivered 2673). At 21:31:07Z the BVN3
conductor logged "Requested missing synthetics start=2674 end=2687
entries=14" and the stream moved on. Over the run: 1,291 heal requests
answered, 2,231 entries applied, 3,691 not-required, `lagging` 0 (12,073 on
the run before), `lagging-miss` 5, `miss` 0, staged-proof refusals 0. All
nine synthetic flows delivered at every 45s sample through both restarts.

**The `heals | 0 -> 0` row above is wrong.** monitor.csv sums a metric no
node has exported since healing became receiver-pull; the number the record
should carry is `accumulate_conductor_heal_entries_total{outcome="applied"}`.
Same for `stalled channels at end | 9`: streams-final.txt is a snapshot taken
after load stopped, so every flow's in-flight tail reads as "STALLED". The
received-minus-delivered on the three BVN3-sourced streams (46, 95, 85) is
larger than on the others (0) and is the one thing in that table worth a
second look.

**Why it does not meet the bar.** Twenty minutes, not thirty: the load
generator stopped at 1171s of 1800 (cause under investigation, below). Two
real disturbances, not six: the fault model draws "skip" 20% of the time and
three of five slots drew it. Both are harness, not network.

**New defect, real, filed separately:** 35 heal requests failed with
"continue receipt list: receipts cannot be combined". The source-side error
(`sequencer_cache.go` building the answer) clustered on acc-bvn3-val2, the
node restarted at 21:30 -- 82 of the source-side occurrences -- and stopped
within a few minutes. A re-seeded cache whose continuation receipt does not
chain is the shape; the restart re-seed is what #4277 added this morning.

**Held entries** climbed monotonically at the four samples: 552, 3624, 6252,
10275 (21:31, 21:36, 21:40, 21:45). Unresolved by the run's end. Worth the
first look on the rerun.
