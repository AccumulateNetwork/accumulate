# Soak run 20260905T153920Z

**Purpose:** ACCEPTANCE RUN #7 (eighth start), 12 H, CHAOS OFF, on issue-4193-producer-cache @455a82ee1 = seventh start (heals held at 0 for 49 min) plus the header cap: a header carries at most 1 MiB of batches, the backlog after a refusal window comes back a header at a time. Seventh start oscillated on the C6 bound (10-17 s blocks after each refusal window, 392 tps accepted). THIS RUN ANSWERS: heals == 0 for 12 h; no 10 s blocks — lag stays under the bound without oscillation, load accepted near 500 tps; memory plateau after the cache horizon (1h); E9 shallow misses; Directory store history reads ~0.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T15:39:20Z |
| commit | `9543f023dd60dfd1c3dd4265490c7fa2fcb500a0` |
| describe | `10k-tps-755-g9543f023d-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 274 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:16903ab6aecefa19f1ba446335f4338a46caff55bddccea9eeaefc8b6ed65104` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T16:21:25Z |
| elapsed | 0.62h |
| driver exit | 1 (FAILED) |
| dn height | 11 -> 2275 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 70 |
| seizure | SEIZED at 2026-09-05T15:54:46 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=103 deliv=22865 undeliv=synthetic BVN2->BVN1 undeliv=673 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 8814 timed reads, p50 2.4 ms, p95 27.6 ms, p99 80.1 ms, max 1889.4 ms (txn read, Directory, entry 2016 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Stopped at 45 minutes — heals held at 0; stopped to free the box for simulation

Heals 0 → 0 for the whole run, no heal request answered, every stream
delivering. With the 1 MiB header cap the network still oscillated on the lag
bound: 22 ten-second blocks in 40 minutes, load accepted sliding from 497 to
416 tps. Stopped by hand at 16:21Z so the box could run the in-process
reproduction of the oscillation (consim at the soak's proportions) instead of
another half-hour run. The committed default cap is now 256 KiB (d6986b5a0).
