# Soak run 20260831T070855Z

**Purpose:** SOAK-12H-500-4179: 12h CHAOS soak at 500 tps on BlockchainDB 5c7b0e5 (windowed permanent reads, background maintenance, N=64) with the #4179 routing fix (f72b74f5a). The previous run 20260831T060018Z seized at 15 minutes: Service.SubmitTransaction routed on the hash of an empty key, so every transaction on every node landed in worker 1, holding all own uncommitted batches in 8 MB of the 32 MB partition budget - 35,999 over-limit warnings, all workerID=1, evictions on worker 1 at 11x the other workers. Keyless submission now round-robins. Acceptance: batch-store over-limit warnings spread across workers or absent; no seizure; blocks 3.0 s; CPU flat over hours (windowed read still under test); restarts rejoin without seal-height errors; probe reads of old entries answer via GetDeep; heals return to 0 after chaos; maintenanceErrors 0. Watch: replay-rejection rate against this run's 3.4% baseline, since round-robin spreads a signer across workers.

| field | value |
|---|---|
| started (UTC) | 2026-08-31T07:08:55Z |
| commit | `f72b74f5a5539bd719e63cdb1cf4abe6b755551f` |
| describe | `10k-tps-612-gf72b74f5a` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:c0314598994512cd16198ba55a5a82d0c2d4be796faf0853568f3d3560480380` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-31T07:29:24Z
- reason: stalled 240s: BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-31T07:29:25Z |
| elapsed | 0.28h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 315 |
| heals | 0 -> 1242 |
| chaos events | 4 |
| monitor samples | 4 |
| seizure | SEIZED at 2026-08-31T07:23:28 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=514 deliv=69546 undeliv=synthetic BVN2->BVN1 undeliv=2559 |
| reconcile pulls (#4073) | 202 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 2850 timed reads, p50 1.3 ms, p95 2.6 ms, p99 6.1 ms, max 158.6 ms (chain read, Directory, entry 335 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260831T072730Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Verdict: #4179 confirmed fixed; BVN2 wedged for an unrelated reason

The routing fix did what it was filed for. Against the previous run:

| signal | 20260831T060018Z | this run |
|---|---|---|
| batch-store over-limit warnings | 35,999, **all** workerID=1 | **0** |
| evictions by workerID | 966 / **10916** / 973 / 965 | 1277 / 1258 / 1405 / 1300 |
| stall lines | 43 by 06:23 | **0** |
| loadgen rate | 458.8 | 497.4 of 500 |
| loadgen rejected | 23,521 (3.73%) | 861 (0.18%) |
| loadgen skipped | 28,121 | 4 |

Evictions within 6% across four workers is the fix stated as a measurement.
(The skip collapse is a separate fix, 43bf3da54, which landed 13 minutes after
the previous run started and so was not in its binary.)

The run still died at 0.28h, on a different failure: **BVN2 wedged at height
277** and never moved again — 07:27 stalled 127s, 07:29 stalled 240s at 23.1
s/block, while BVN1 advanced 331 -> 368 at a flat 3.0 s/block. The Directory
began stalling behind it (85s at 07:29) and stallkill tore the run down at
07:31.

BVN2 is not blocked on storage. Its goroutine dumps contain no semacquire at
all — nothing is waiting on a lock — and no processCommittedCertificate frames.
It is busy rather than stuck: 37-57% CPU against BVN1's 14-19%, with the
adapter's background maintenance goroutine in mergeIndexes.

The load is heavily skewed onto BVN2, which is the standing condition behind
every "BVN2 is the slow one" observation in this series:

| measure | BVN1 | BVN2 |
|---|---|---|
| database | 3.6 GB | 6.3-7.0 GB |
| synthetics produced to the other BVN | 24,608 | 80,991 |
| synthetics produced to the Directory | 1,313 | 19,429 |

15x on the Directory flow is far more skew than the identity population
explains (9/6 by url routing bucket), so the cause is which ACTIONS land where,
not merely how many identities each partition holds.

Replay rejections rose per transaction as predicted for round-robin: 300.5 ->
534.0 per 1k generated. End-to-end loss fell 20x over the same interval
(3.73% -> 0.18%), which points at duplicate signature deliveries being deduped
rather than lost work — NOT VERIFIED; the run was seized before the end-of-run
per-action report that would settle it.

Read-back probe, whole run: 2850 reads, p50 1.3 ms, p95 2.6 ms, p99 6.1 ms, max
158.6 ms, 0 failed, 0 timed out — no counter-evidence against the windowed-read
fix, but 17 minutes proves nothing about CPU drift over hours.
