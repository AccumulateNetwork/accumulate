# Soak run 20260916T185711Z

**Purpose:** 8 execution shards at 500 tps on dagbft-integration 20625f0a0: executionShards=8 in docker-network.yml, written into every node's config by init (no environment path). #4149 gates green under -race at 1/8/64. Chaos off. Question: does sharding move the serial executor's ceiling -- 21% of blocks over 0.82s, accepted load 499->414, 48% refused at 500 tps? Watch: exec_phase parallel > 0 and flushes > 0 on every node (the proof it is on), block-time tail, accepted tps, refusals, lag; heals must stay zero; any shard commit failure poisons a block -- watch for process-failed hand-offs and stalls.

| field | value |
|---|---|
| started (UTC) | 2026-09-16T18:57:11Z |
| commit | `444119cc78026a9057c91b48d3aa373149147d8d` |
| describe | `10k-tps-717-g444119cc7-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 272 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:7ab5f97e8eae30b8540bff50283ddc0673001eea87ba85db3362b2c6b03ff292` |
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

Config as run is frozen in `config/` (soak.conf, the compose and network files). Results appended below on exit.

## Results (recorded by hand: the launcher was stopped with SIGTERM at 20:19:59Z to start the #92 run, so the harness did not append)

| field | value |
|---|---|
| ran | 1.3h of 12h (18:57Z to 20:20Z) |
| DN blocks | 10→4561 at 76 min |
| heal entries | **0** (nothing missing, nothing stranded) |
| accepted tps (run average) | 500 at 5 min → 348 at 76 min |
| refusals ("execution is lagging consensus") | 2.25M submissions by 76 min |
| blocks over 0.82 s (cumulative) | 5% at 5 min (serial: 21%) → 15% at 19 min |
| knee | 16-19 min; from 25 min BVN2 lag 15-29 and refusing, BVN1 lag ~6 |

**What sharding did:** the per-block execution cost fell (tail 5% vs 21% serial at 5 min). **What it did not do:** move the knee, because the knee is not execution. The 30 s CPU profile at 19 min (`cpu-shards8-knee-bvn2val4.pprof`) has `KVShard.PutDyna → KV2.Compress → CompactHistory` at 3.43 s of 30 s on the committing goroutine (0.31 s on the serial run at 15 min): the store compacted history from inside a put every 5,000 writes, on the protocol path. **BlockchainDB #92**, fixed in PR #93 (`8751863`).

At 60 min, twenty goroutine snapshots of bvn2-val4 half a second apart: seal shard goroutines in `fsync` 16 times and blocked in `write()` of the manifest 11 times (host dirty budget 256 MB full); block loop in `pread` on cache-missed Gets 7 times and **inside `CompactHistory` from `PutDyna` twice** (once fsyncing the merged segment, once waiting on the maintenance lock the adapter's pass held); executor shard workers waiting on the `syncStore` mutex in **18 of 24** samples while the holder read from disk. Disk: I/O pressure `full` 24-34%, each BVN2 node reading ~200 MB/s (compaction folding a 220 MB + 90 MB segment per shard), a 4 KB fsync 16 ms p50 / 48 ms max. Filed **BlockchainDB #94** (compaction has no I/O budget) and **accumulate #4281** (syncStore serializes shard reads across the disk read).

**Contamination:** the BlockchainDB test suite ran on this box 19:37-20:10Z (from minute 40) and the consim overload tests 20:11-20:19Z; both after the knee. Refusal episodes are trip/clear chatter around 8 (#4250).
