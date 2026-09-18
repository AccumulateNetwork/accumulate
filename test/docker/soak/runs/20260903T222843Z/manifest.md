# Soak run 20260903T222843Z

**Purpose:** STEADY-STATE ACCEPTANCE RUN #4 (PLAN.md), CHAOS OFF, on issue-4207-batch-plane-budgets (64cbb38b4) = run #3's code plus S3 (C1 #4206, C2 #4207, C3 #4208): seal timeout 1 s (was 100 ms) and one worker per node (was four), so batches carry tens of transactions instead of one or two; stores bounded in bytes only (no count limits); user submissions refused with NotReady while a worker's own uncommitted batches fill its share, system traffic never refused; over-limit logged on transition; loadgen backs off on NotReady. Run #3 (20260903T213153Z) reached the batch-store storm at minute 16 with no chaos and no other fault. THIS RUN ANSWERS: does the batch plane stay within budget at 500 tps for 12 h, and are memory and CPU flat (PLAN steady-state table)? WATCH: tx per batch in execution accounting (was 2); accumulate_dagbft_batch_store_bytes and batch_store_refusing; zero 'Batch store over limit' storms (a transition line is fine); loadgen skipped counts (NotReady) vs rate; mem.csv heap per node hour over hour; captureProvableView view age.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T22:28:43Z |
| commit | `64cbb38b4d31bbd666709f81cbfb7fec8592dc4d` |
| describe | `10k-tps-680-g64cbb38b4-dirty` |
| branch | `issue-4207-batch-plane-budgets` |
| uncommitted files | 70 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:00f9779d648e2ae771daa39391dfe90b795c1d9519b0858c4546f887031783f8` |
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

## Stopped early by stallkill

- stopped (UTC): 2026-09-03T23:06:33Z
- reason: stalled 249s: BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-03T23:06:34Z |
| elapsed | 0.58h |
| driver exit | 143 (FAILED) |
| dn height | 15 -> 2105 |
| heals | 8 -> 216565 |
| chaos events | 1 |
| monitor samples | 65 |
| seizure | SEIZED at 2026-09-03T22:42:25 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=492 deliv=58671 undeliv=synthetic BVN2->BVN1 undeliv=810 |
| reconcile pulls (#4073) | 1213 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 8250 timed reads, p50 2.2 ms, p95 11.1 ms, p99 26.7 ms, max 214.5 ms (txn read, BVN1, entry 1446 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260903T230356Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
