# Soak run 20260822T052535Z

**Purpose:** 12h chaos soak on c99d3f4ef: #4125/#4128 consensus fixes (retention window + re-delivered-certificate skip) plus the loadgen fixes for #4130 (token accounts are funded and the balance awaited before they are advertised) and #4129 (lock-account skips while the network produces no major blocks, instead of bricking lite accounts forever). Watch: certificates_redelivered_total must stay 0, batch waits none, and tx/s should now approach the 10/s target rather than the 7.5-8.7 the unfunded and locked accounts were costing. stallkill 240s with the idle guard; wedgewatch idle-guarded too.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T05:25:36Z |
| commit | `6af714f2e86ebcd57b0b7fbd3cad9a5cb1203d9b` |
| describe | `10k-tps-422-g6af714f2e-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 1 (see config/uncommitted.patch) |
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

## Stopped early by stallkill

- stopped (UTC): 2026-08-22T05:32:27Z
- reason: stalled 279s: BVN1,BVN3,Directory (threshold 240s)

The run was ended once a partition had been stalled past the
threshold. Evidence was captured before stopping; see the probe-*
directory written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T05:32:27Z |
| elapsed | 0.0h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 553 |
| heals | 0 -> 0 |
| chaos events | 0 |
| monitor samples | 2 |
| seizure | none detected |
| reconcile pulls (#4073) | 12 |
| stalled channels at end | 3 |
| wedge captures (#4125) | 1 wedge-20260822T053224Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Discarded — the run was unmonitored

soakmon passed the startup gate and then died, and nothing noticed: soak.sh's
gate is a startup check, not a liveness guarantee, and wedgewatch and stallkill
both treat an unreachable monitor as "try again later". The run therefore
generated load for roughly eight minutes against a network no one was watching.

Its numbers are not trustworthy and no conclusion should be drawn from them.
The `wedge-20260822T053224Z` and `probe-20260822T053225Z` captures were taken
during teardown, not in response to any observed condition.

What killed soakmon was not established. It left a zero-byte log and no Python
traceback, which points to being killed rather than crashing; the likeliest
culprit is one of this session's own `pgrep -f` / `pkill -f` patterns matching
its command line, a mistake made more than once tonight.

Fixed in `199f8ea29`: stallkill now ends a run whose monitor has been
unreachable for MON_DEAD_SECS (default 120s), rather than polling a dead
endpoint forever.
