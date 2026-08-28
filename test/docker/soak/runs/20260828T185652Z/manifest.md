# Soak run 20260828T185652Z

**Purpose:** #4165 SMOKE, 5 min: first run of the 8-node docker network with every node on BlockchainDB (ACC_STORAGE=blockchainDB, ba1d3bd8d). Questions: do nodes boot and produce blocks on the bcdb backend; does stats.json land in storage-stats/ at teardown; misroutedShapes must be empty and PermKV PutConflict zero. Not a measurement — 5 minutes proves nothing about duplicate rates.

| field | value |
|---|---|
| started (UTC) | 2026-08-28T18:56:52Z |
| commit | `ba1d3bd8d80c085eb6889415e1a3987e72b5fbe4` |
| describe | `10k-tps-573-gba1d3bd8d` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:435502b3a159e2e153c753e281a130584d145683e284b0350827ccd3227c65c5` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 5m |
| target TPS | 10 |
| storage | blockchainDB |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-28T19:06:28Z |
| elapsed | 0.12h |
| driver exit | 0 (clean) |
| dn height | 8 -> 166 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 22 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Stopped early by stallkill

- stopped (UTC): 2026-08-28T19:08:23Z
- reason: monitor unreachable for 120s

Evidence was captured before stopping; see the probe-* directory
written at that moment.
