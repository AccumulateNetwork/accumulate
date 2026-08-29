# Soak run 20260829T010116Z

**Purpose:** STORAGE A/B, arm=leveldb RERUN (A-prime) (#4165, #4176). Same as 20260829T001519Z but with the loadgen fix a0d734cc6, so ADIs are created and the mix is the full one — arm B (20260829T003712Z) already ran with the fixed loadgen, so this makes the pair one-variable again: ACC_STORAGE only. 8-node, chaos OFF, 50 tps, 8 shards, 30m.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T01:01:16Z |
| commit | `d739cbb8e87bad633757ab0a6e1ce64e9a4fab71` |
| describe | `10k-tps-581-gd739cbb8e` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:68259101ec38ecff99c3300495d9911ed7e9ab1ddc1f90e4d4301745df08e915` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 50 |
| storage | leveldb |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-29T01:22:44Z |
| elapsed | 0.32h |
| driver exit | 0 (clean) |
| dn height | 8 -> 411 |
| heals | 0 -> 8 |
| chaos events | 1 |
| monitor samples | 54 |
| seizure | SEIZED at 2026-08-29T01:17:45 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=614 undeliv=synthetic BVN2->BVN1 undeliv=261 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Storage A/B — the one-variable pair (A′ vs B)

Same binary for the nodes, same compose, same genesis recipe, same loadgen
(a0d734cc6, full mix), 50 tps, chaos off, 8 shards, 20 min of generation,
DN 8→411 in both. ACC_STORAGE is the only difference.

| | A′ leveldb `20260829T010116Z` | B blockchainDB `20260829T003712Z` |
|---|---|---|
| block interval (DN) | 2.97 s | 2.97 s |
| loadgen | 46.6 tps, 0 rejected, 45 ADIs / 74 books / 119 pages / 37 tokens | 46.8 tps, 0 rejected, 46 / 68 / 114 / 24 |
| CPU per node, avg | **19.2 %** (16–22 by node), p95 57 % | **11.3 %** (9–13 by node), p95 27 % |
| RSS per node at end | **941 MiB** avg (861–1071) | **761 MiB** avg (726–821) |
| heals | 0 | 0 |
| streams at end | received == delivered | received == delivered |

Reading: at this load both hold the 3 s cadence with nothing rejected and
nothing healed. BlockchainDB uses ~40 % less CPU and ~180 MiB less RSS per
node. The RSS gap is at least partly configuration — leveldb runs a 128 MB
block cache + 64 MB write buffer per engine that bcdb has no equivalent of
— and leveldb's RSS was still climbing toward the 1200 MiB GOMEMLIMIT at
the end, so a longer run would widen it. The CPU gap is not explained by
caches and is the number worth a profile.

What 20 minutes cannot show: bcdb's two known degradations with block
count — one permanent segment per block with lookups walking them
(BlockchainDB#30; 268 segment files after ~150 blocks on one node) and the
whole-layer Compress every 128 commits (#31). Those curves need the 12h
run, chaos on, so a node restart also exercises #4173's fix.

Arm A `20260829T001519Z` (leveldb, OLD loadgen, lite-only: adis=0) is the
baseline that exposed #4176; it is not part of the comparison.
