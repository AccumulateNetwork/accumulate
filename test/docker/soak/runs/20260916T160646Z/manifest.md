# Soak run 20260916T160646Z

**Purpose:** Throughput run on dagbft-integration ab369548c after the healing fixes: #4248 in-flight window, stillness gate limited to synthetic streams (6a906d5fc), anchor quorum gathered by node and the #4260 lag gate (56d7c4241), and one healing answer path (f15eb4b18). 12h, 500 tps, chaos off. Watch: heal entries against execution lag; refusals; accepted tps over time; zero misses and nothing stranded.

| field | value |
|---|---|
| started (UTC) | 2026-09-16T16:06:46Z |
| commit | `ab369548c3ce44d4e4235f74a12be0a05eaafa4c` |
| describe | `10k-tps-711-gab369548c-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 270 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:e4e159939e5a97bc711388ef90ac995ede9da4261dbbef6a3c47e2b1cdcc2202` |
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

## Result — ended at 49 min (of 12 h) by an external SIGTERM

At 17:01:37Z every process under the launching shell received SIGTERM
(`soakmon exiting: signal 15`); the load generator, monitor, stallkill and
wedgewatch died together while the containers ran on unloaded. Not a stop the
harness or the operator chose: the launcher was a background task of the
operator's session and the session terminated its background tasks. The
containers were left up and their metrics captured at 17:05Z
(`*.metrics-at-stop.txt`). Evidence is valid to 17:01:21Z.

**What it was.** dagbft-integration ab369548c: #4248 in-flight window, the
stillness gate limited to synthetic streams (6a906d5fc), anchor quorum
gathered by node and the #4260 lag gate (56d7c4241), one healing answer path
(f15eb4b18). 500 tps, chaos off.

| t | lag (max) | heal entries | not-yet | miss | failed | tps | refused |
|---|---|---|---|---|---|---|---|
| 6 min | 1 | 0 | 11 | 0 | 0 | 499 | 5 |
| 15 min | 11 | 0 | 11 | 0 | 0 | 491 | 22,459 |
| 24 min | 11 | 0 | 11 | 0 | 0 | 461 | 228,791 |
| 30 min | 33 | 0 | 14 | 0 | 0 | 448 | 407,946 |
| 39 min | 18 | 0 | 15 | 0 | 0 | 432 | 722,523 |
| 48 min | 21 | 0 | 15 | 0 | 0 | 415 | 1,095,587 |
| 49 min (end) | — | **0** | — | 0 | 0 | 414 | 1,108,019 of 1,215,450 |

**Healing on a fault-free network is now zero.** Not low: zero entries over
49 minutes and 2,943 Directory blocks, with lag reaching 33. The reference
(20260906T134054Z) had 75,936 by 51 minutes; last night's run with #4248
alone (20260916T032130Z) had 1,811 by 24 minutes. The difference from last
night is the #4260 gate: a node behind consensus does not ask, so there is
nothing for a caught-up source to answer wrongly. Zero misses, zero
failures, nothing stranded; 14-15 "not-yet" are the initial quiet-stream
probes and never grew.

**Throughput is unchanged, as expected.** Accepted load decayed 499 -> 414
tps as execution fell behind; 48% of offered submissions were refused. That
is #4250 (bang-bang back-pressure) over #4258 (the executor ceiling), and
nothing in this build touches either. Lag oscillated 9-33 rather than
climbing: the executor keeps up in bursts and loses ground in bursts.

**BlockchainDB read path, measured in this run** (30 s CPU profile of
bvn1-val1 at 15 min, against the 2026-09-06 review): store reads 18.5% ->
10.5% of CPU, `lookupHistory` 11% -> 4.9%, `pread` ~17% -> 8.1%,
`segment.bloomTest` absent from the top 600 nodes. History filters are held
under budget (aggregate 9.2 MB perm / 9.8 MB dyna across 8 shards, ~15% of
the per-store bound). The read path is no longer the ceiling; GC (21%) and
allocation (12.6%) are, with `ed25519.Verify` at 8.5%.
