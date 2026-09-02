# Soak run 20260902T222155Z

**Purpose:** validate #4189 durable unhashed staging + #4201 heal cadence, at the same 12h/500tps as 20260902T132651Z which livelocked with recv-deliv pinned at exactly 4096. Includes d8d636b52: Received is answered from staging's sighted mark on read rather than stored, which is what the first attempt at this run got wrong -- every channel read 0 received and the dashboard called a healthy network STALLED. THE TEST: recv-deliv must never pin at 4096, heals must not climb into six figures against a stalled stream, and heights must keep moving.

| field | value |
|---|---|
| started (UTC) | 2026-09-02T22:21:56Z |
| commit | `d8d636b52895304d2a4a6dd802a0c74581012eda` |
| describe | `10k-tps-654-gd8d636b52-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:bedf7bd9b592c4e3a99dd37e0c758c888af2d384993aa223b8cb39e7b74328ab` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | leveldb |
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-02T22:33:04Z
- reason: stalled 241s: BVN1,BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-02T22:33:05Z |
| elapsed | 0.14h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 114 |
| heals | 0 -> 402 |
| chaos events | 2 |
| monitor samples | 2 |
| seizure | none detected |
| reconcile pulls (#4073) | 16 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 870 timed reads, p50 1.6 ms, p95 6.0 ms, p99 21.5 ms, max 95.4 ms (txn read, Directory, entry 94 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260902T223111Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
