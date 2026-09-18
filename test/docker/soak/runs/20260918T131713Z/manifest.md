# Soak run 20260918T131713Z

**Purpose:** 30m 100tps chaos: E11 proof on DI 0132b886c, with #4296 -- the join can now find a validator to ask, and refuses to execute if it cannot. Every restarted validator must keep agreeing on the anchor body on both partitions; Directory 12 of 12.

| field | value |
|---|---|
| started (UTC) | 2026-09-18T13:17:13Z |
| commit | `0132b886c1665ca6008ad5825c43ca8235bde9f5` |
| describe | `10k-tps-845-g0132b886c-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 353 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:eaf4d46c918c78691cb8fa4c93bdea7fa2293889e023228e8deedfc420793785` |
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

- stopped (UTC): 2026-09-18T13:23:08Z
- reason: #4296 works -- the join now finds peers and takes staging (22 times, e.g. block=201 Directory / block=198 BVN1 on the restarted node). Two defects behind it stop the network instead: (1) every account pull is refused ("Pulled the accounts the blocks named asked=1 pulled=0 refused=1", 1663 occurrences), so a node that took staging never converges and stays in the join's pull loop forever; (2) at genesis every node loads genesis, so lastBlock=1 and Fresh(lastBlock==0) is false, and all 12 nodes join and mutually refuse ("X is joining and cannot answer for what it has not executed"). Result: 11 of 12 nodes collecting, 1 executing. Directory stuck at block 1.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-18T13:23:08Z |
| elapsed | 0.07h |
| driver exit | 143 (FAILED) |
| dn height | 1 -> 201 |
| heals | 0 -> 2 |
| chaos events | 5 |
| monitor samples | 8 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 176 timed reads, p50 1.2 ms, p95 2.0 ms, p99 3.4 ms, max 3.5 ms (txn read, BVN2, entry 213 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260918T132118Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
