# Soak run 20260825T140034Z

**Purpose:** 1000 tx/s ladder on the NEW 8-node topology (2 BVNs x 4 validators, cut from 12 nodes to free CPU per node; DN+BVN1+BVN2 so cross-partition traffic is still real). Chaos OFF - throughput+footprint run, not a resilience run. Rate ladder driven live via the loadgen control API: 100 -> 250 -> 500 -> 750 -> 1000, ~45min per step, then hold at the highest clean rate for the remainder. TWO GATES: (1) the sub-1GB footprint from #4164 items 1-5 (configured budget ~680MB/container, GOMEMLIMIT 1200MiB against a 1536m cap) must hold under sustained load - every prior number is from a 33-min simulation, not a node; (2) name the rate knee on the way to 1000. Prior history on the 12-node topology: 800 target achieved 47/s, 250 achieved 220/s, 60 achieved 57/s. Starts at 100.

| field | value |
|---|---|
| started (UTC) | 2026-08-25T14:00:34Z |
| commit | `5d079de6a4e59a44ffe981821c7ea1b83cb71992` |
| describe | `10k-tps-528-g5d079de6a` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:74ef69860b5b402d2eef20bf713d4647e4f282564551f70f6ae68e6dea93acff` |
| image id (corrected) | recorded at start as `docker-bvn1-val1` (sha256:65b858c9…), a leftover built 2026-08-20 from before the compose project was pinned to `disoak` (#4124). The network ran the image above, built by this run. Fixed in soak.sh at 2891a003f; this run started minutes earlier. |
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

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-25T14:59:28Z |
| elapsed | 0.89h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 1019 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 11 |
| seizure | SEIZED at 2026-08-25T14:20:01 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=212 undeliv=synthetic BVN2->BVN1 undeliv=902 |
| reconcile pulls (#4073) | 13 |
| stalled channels at end | 3 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
