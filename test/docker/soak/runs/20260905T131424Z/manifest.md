# Soak run 20260905T131424Z

**Purpose:** BISECT 1/3, 12 MIN: tip f8e4bbc50 with E9 (7df36dc5b, single-site append) Go files REVERTED. Run 20260905T051008Z froze every synthetic stream at minute 5 of load: every Directory anchor was rejected at the BVNs (receipt 0 is invalid: result does not match the anchor), so no receipts reached the BVNs and nothing was dispatched. If BVN1->Directory delivered keeps growing past minute 5 here, E9 is the cause.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T13:14:24Z |
| commit | `f8e4bbc50a7cc78fdf801052296146eaaeb42f45` |
| describe | `10k-tps-747-gf8e4bbc50-dirty` |
| branch | `bisect-e9-revert` |
| uncommitted files | 184 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:fe7777e1cd211adb0cd37e84d8e665db56d38817fe3699a327fae489653c39ed` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12m |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T13:22:01Z |
| elapsed | 0.06h |
| driver exit | 1 (FAILED) |
| dn height | 15 -> 261 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 11 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 210 timed reads, p50 1.4 ms, p95 4.3 ms, p99 16.5 ms, max 28.5 ms (txn read, BVN2, entry 212 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Bisect result

Stopped by hand at 8 minutes. BVN1→Directory delivered froze at 240 by
13:21Z, four minutes after load started, with E9 reverted: E9 is not the
cause. The cause is the mark-point placement, found by reading
(`20260905T051008Z/review.md`).
