# Soak run 20260916T001538Z

**Purpose:** Verification of the COMPLETE #4280 fix on dagbft-integration de7e6323d: healing now asks for nothing until a stream's Delivered has sat still for probeAfter (4) activations — holes included, because a stream executes in order so a moving Delivered proves nothing below it is missing. 1h, 500 tps, CHAOS OFF from soak.conf. Baselines on the same conditions: 20260915T211229Z ~5,600 healed entries/min (no gate), 20260915T233806Z ~1,500/min (probe gated only). THIS RUN ANSWERS: does a fault-free network now heal essentially nothing? WATCH: conductor_heal_entries_total near zero and flat; answered near zero; not-yet collapsed; miss 0; every stream's delivered advancing; block production near 1s; no SEQUENCE REGRESSION. Also reads the exported hand-off accounting (#4279): arrived minus executed, and what the failure outcomes leave unaccounted.

| field | value |
|---|---|
| started (UTC) | 2026-09-16T00:15:38Z |
| commit | `de7e6323db397b612f587d75f41fdac059dc8839` |
| describe | `10k-tps-700-gde7e6323d-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 271 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:c97512836719583fe1b4fe490ddb0270257a7d08d139df8333fefea741b25b46` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 1h |
| target TPS | 500 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-16T00:21:36Z |
| elapsed | ?h |
| driver exit | 1 (FAILED) |
| dn height | ? -> dnHeight |
| heals | ? -> heals |
| chaos events | 1 |
| monitor samples | 0 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 6 timed reads, p50 1.5 ms, p95 2.5 ms, p99 2.5 ms, max 2.5 ms (txn read, BVN1, entry 33 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
