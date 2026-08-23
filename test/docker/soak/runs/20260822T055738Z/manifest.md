# Soak run 20260822T055738Z

**Purpose:** 12h chaos soak on 2663e8ac1. Consensus: #4125 retention + re-delivery skip, #4128 committed batches stay fetchable. Loadgen: #4130 fund-before-advertise, #4129 lock guard. Harness: soakmon supervised, line-buffered, heartbeat every minute with threads/fds/rss/subprocess-failures, and it names the signal that kills it; watchdogs log their decisions once a minute; bootstrap timeouts report what the network says about the deposit txid; first failure and first skip of every action type is logged.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T05:57:38Z |
| commit | `2663e8ac15b19503683f638258e2fad144fa5c15` |
| describe | `10k-tps-426-g2663e8ac1` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 0  |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T06:10:01Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 553 |
| heals | 0 -> 0 |
| chaos events | 0 |
| monitor samples | 3 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
