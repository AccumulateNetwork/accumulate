# Soak run 20260829T055201Z

**Purpose:** BlockchainDB f5ab54d at 250 tps (#4165): the rate every 8-node leveldb run seized at within 30 min (08-25). Durable dynamic layer at seal (BlockchainDB#29), ErrImmutable, MergeBelow lag 512, borrowed fds; adapter 1daa40fb1; loadgen a0d734cc6+ef414fdc6 (ADIs created, page-key race fixed). 8 nodes, chaos OFF, 8 shards. The 100 tps run 20260829T054138Z was clean (87 tps achieved, 3.0 s blocks, RSS 335-541 MiB, 0 conflicts) until the loadgen panicked on retune to 250. Watch: achieved tps, block interval, RSS vs the 1536m cap, backpressure, perm file count.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T05:52:01Z |
| commit | `ef414fdc6633591143092a9e2b11df9dbe226b10` |
| describe | `10k-tps-584-gef414fdc6-dirty` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 4 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:9b48644bdecdfa9751f0b18af98fc112ae53492806c2542c199c8ce67f9da6d2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 250 |
| storage | blockchainDB |

Config as run is frozen in `config/`. Results appended below on exit.

## Result (stopped by hand at 06:04Z, ~12 min of generation)

Not a harness verdict — Paul stopped it. Rate ladder driven live: 250 → 400
tps at 06:00:06 (old pacer, so achieved ≈ 87 % of target throughout).

| phase | achieved | blocks | CPU / node | RSS / node | heals · waits · backpressure |
|---|---|---|---|---|---|
| 250 tps (05:53–06:00) | 216–219 tx/s | 2.8–3.0 s | 15–27 % | 421–432 MiB (BVN1), 650–682 (BVN2) | 0 · 0 · 0 |
| 400 tps (06:00–06:04) | 292 → 348 tx/s | 3.0 s | BVN1 53–65 %, **BVN2 90–104 %** | 800–860 MiB, bvn2-val2 **1024** | 0 · 0 · 0 |

Total 146,914 sent, 0 rejected, 10 ADIs. No stall, no heal, no backpressure
at any point. At 400 the BVN2 containers (the treasury side, which also
produces most synthetics) were CPU-saturated at one core each and RSS was
~180 MiB under the 1200 MiB GOMEMLIMIT and rising — the two limits that
ended the leveldb runs, now reached at 3–4× the rate. Storage never became
the bottleneck in this run.
