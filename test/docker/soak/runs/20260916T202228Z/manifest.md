# Soak run 20260916T202228Z

**Purpose:** BlockchainDB #92 fixed (PR #93, 8751863): puts never compact; 8 execution shards, 500 tps, no chaos; accumulate afc7c7513

| field | value |
|---|---|
| started (UTC) | 2026-09-16T20:22:28Z |
| commit | `afc7c7513933b2139e33d12e11b8fa8300d99c84` |
| describe | `10k-tps-719-gafc7c7513-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 273 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:3c25a5b0357b6bde4b46e50f8cc1c850c6942111e9c7fea842de89d55b148748` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | none (CHAOS=off) |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Results (recorded by hand: stopped at 20:47Z on request, the harness was SIGTERMed and the network taken down)

| field | value |
|---|---|
| ran | 0.4h of 12h (20:22Z to 20:47Z) |
| DN blocks | 10→1171 |
| heal entries | **0** (6,349 heal requests declined locally as "lagging", none sent) |
| accepted tps | 500 to 12 min, 494 at 18 min |
| refusals | 2 at 6 min, 259 at 12 min, 19,446 at 15 min, 79,295 at 24 min |
| blocks over 0.82 s | 3-6% at 6 min; **29% of the blocks between minute 6 and 17** |
| commit phase (block_phase_seconds, new) | 180-200 ms at 6 min → 390 ms average over minutes 6-17; process 160-210 → 280 ms |
| put-path compaction in 20 goroutine snapshots | **0** (old run: 2); `PutDyna` 1% of CPU (old run: 12%) |

**What #92 changed:** the store no longer compacts from inside a put, verified in the running binary (BlockchainDB `8751863`). **What it did not change:** the knee, at the same 15-17 minutes. Twenty goroutine snapshots of bvn2-val4 at 17 min: all 56 seal shard goroutine samples in `fsync`; 35 of 44 executor shard workers queued on the `syncStore` mutex (accumulate #4281); BVN2 nodes reading 60-120 MB/s and writing 100-140 MB/s (the adapter's compaction cadence, BlockchainDB #94); I/O pressure `full` 19% (60 s average), host load 52. Stopped to work the store in isolation: `BlockchainDB/cmd/bdbench` reproduces the adapter's drive without Accumulate.
