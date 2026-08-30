# Soak run 20260829T141021Z

**Purpose:** STORAGE A/B at 100 tps, arm=leveldb. Paired with the blockchainDB arm run immediately after: same binary (BlockchainDB dep bumped to v0.1.1 so both arms build identically), same genesis recipe, same rate, ONE variable (ACC_STORAGE). 8-node 2-BVN compose, chaos OFF, 8 shards, 20m, default memory budget (1536m / GOMEMLIMIT 1200MiB). Compare block interval, CPU, RSS over time, read-probe latency by age. Context: the prior 100 tps bcdb run (20260829T054138Z, f5ab54d) failed at 0.11h on stalled channels and the 50 tps leveldb A-prime seized on a stalled stream, so watch stalls in BOTH arms before attributing anything to storage.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T14:10:22Z |
| commit | `511d2ddacf4402b8705a92b2c3b0a984d963b4c7` |
| describe | `10k-tps-595-g511d2ddac-dirty` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:e2ac3bb96f666e6637181b15bce0c4ed27049142b095084948efa49b43d4c82b` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 20m |
| target TPS | 100 |
| storage | leveldb |
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.
