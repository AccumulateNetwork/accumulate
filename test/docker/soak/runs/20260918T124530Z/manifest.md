# Soak run 20260918T124530Z

**Purpose:** 30m 100tps chaos: E11 proof on DI 0259684c5 -- a restart is a join (#4291-#4295). Every restarted validator must keep agreeing on the anchor body on both partitions; Directory 12 of 12.

| field | value |
|---|---|
| started (UTC) | 2026-09-18T12:45:30Z |
| commit | `0259684c5bb813b84c3eb0db121fbb9a9f86f018` |
| describe | `10k-tps-840-g0259684c5-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 352 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:b9a05320fd5ee33e41986828c90146c403255dbc972d32ac90fe768a62fb0f31` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | restart or pause one BVN container every 120s + 0-60s |
| topology | 3 BVNs, 12 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 30m |
| target TPS | 100 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Stopped early by hand

- stopped (UTC): 2026-09-18T12:54:45Z
- reason: the join never engaged. join.APIPeers.Validators calls FindService without the network name, so it searches a DHT key nobody advertises: every lookup returns an empty list and the node falls back to executing from its own empty staging -- the #4290 behaviour. Proven by a standalone libp2p probe (0 results without Network, 2 with) and by the timing (18s of pure sleeping, 0ms per round). This run therefore tested the fallback, not the join.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-18T12:54:46Z |
| elapsed | 0.11h |
| driver exit | 143 (FAILED) |
| dn height | 37 -> 450 |
| heals | 0 -> 69 |
| chaos events | 7 |
| monitor samples | 14 |
| seizure | SEIZED at 2026-09-18T12:50:26 :: stuck=n/a stuckStream= worst=BVN2->BVN1 gap=116 deliv=1944 undeliv=synthetic BVN2->BVN1 undeliv=129 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| read-back probe | Whole run: 704 timed reads, p50 1.6 ms, p95 3.6 ms, p99 2017.7 ms, max 8040.1 ms (txn read, BVN2, entry 335 blocks old); 7 failed, 7 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
