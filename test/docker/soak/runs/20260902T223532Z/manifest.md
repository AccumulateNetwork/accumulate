# Soak run 20260902T223532Z

**Purpose:** FIRST BCDB SOAK. Every previous run, including 20260902T132651Z which livelocked, ran goleveldb -- docker-compose.yml defaults --database to leveldb and nothing set ACC_STORAGE, so the backend this branch line is named for has never been soaked. Validates #4189 (durable unhashed staging; MaxPendingSequenced and its silent refusal deleted) + #4201 (heal cadence every 4 blocks, 2 senders for PULLS only) + d8d636b52 (Received answered from staging on read). Note two variables differ from the baseline: staging AND backend. Previous attempt 20260902T222155Z died at 11min -- stallkill, BVN1 stalled 193s -- with BVN2 nodes at 194-264% CPU sitting on the 1200MiB GOMEMLIMIT; a 30s profile showed scanobject at 29% and healing at ZERO, i.e. GC thrash from leveldb recordCache+BufferPool+memdb (~310MB/node), not healing. WATCH: does dropping leveldb clear the GC ceiling; recv-deliv must never pin at 4096; heals must not climb into six figures against a stalled stream.

| field | value |
|---|---|
| started (UTC) | 2026-09-02T22:35:33Z |
| commit | `d8d636b52895304d2a4a6dd802a0c74581012eda` |
| describe | `10k-tps-654-gd8d636b52-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 5 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:933b93a25e3d6b52a14403c15f5bc68034957e4dc340da55434470b462b90abc` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-02T22:51:45Z
- reason: stalled 254s: BVN1,BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-02T22:51:46Z |
| elapsed | 0.22h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 214 |
| heals | 2 -> 99 |
| chaos events | 4 |
| monitor samples | 3 |
| seizure | SEIZED at 2026-09-02T22:48:00 :: stuck=0 stuckStream= worst=BVN1->Directory gap=155 deliv=1062 undeliv=synthetic BVN2->BVN1 undeliv=6377 |
| reconcile pulls (#4073) | 38 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 1902 timed reads, p50 1.4 ms, p95 26.1 ms, p99 272.6 ms, max 822.1 ms (txn read, BVN1, entry 151 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260902T224931Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
