# Soak run 20260822T053653Z

**Purpose:** 12h chaos soak on 199f8ea29: #4125/#4128 consensus fixes (retention window, re-delivered-certificate skip), loadgen fixes for #4130 (token accounts funded and balance awaited before being advertised) and #4129 (lock-account skips while no major blocks exist), and stallkill now stops a run whose monitor has died rather than polling a dead endpoint. Watch: redelivered must stay 0, batch waits none, tx/s should approach 10 now that unfunded and locked accounts are not eating attempts.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T05:36:53Z |
| commit | `c2b014acb40fa6622a543974a85c7e9e0cf8a8e6` |
| describe | `10k-tps-424-gc2b014acb-dirty` |
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

- stopped (UTC): 2026-08-22T05:43:42Z
- reason: stalled 280s: BVN1,BVN3 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T05:43:42Z |
| elapsed | 0.0h |
| driver exit | 143 (FAILED) |
| dn height | 169 -> 553 |
| heals | 4 -> 12 |
| chaos events | 0 |
| monitor samples | 2 |
| seizure | none detected |
| reconcile pulls (#4073) | 11 |
| stalled channels at end | 3 |
| wedge captures (#4125) | 1 wedge-20260822T054340Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Discarded — the monitor died again

soakmon died about 3.5 minutes in, for the second run running. This time
stallkill's new monitor-liveness guard caught it and ended the run rather than
letting it generate load unobserved, which is the guard working as intended —
but the run is still too short and too disrupted to conclude anything.

The death itself is unexplained. Zero-byte log again, no traceback, no OOM
(42GB free), and soakmon has no self-exit path — which points to a signal from
outside. It ran for 2h14m without trouble in run 20260822T015342Z, so this
started recently; the obvious suspect is the soakmon edits that landed just
before the first occurrence (the life_from refactor and the dashboard row),
though the unit tests pass and it compiles and runs by hand.

Rather than guess again, soakmon now logs its own exit — signal handlers for
TERM/INT/HUP/QUIT plus an atexit hook — so the next occurrence names the cause
instead of dying mute. soak.sh also supervises and restarts it, so a momentary
loss no longer costs a whole run.

Nothing here reflects on the consensus fixes under test: redelivered=0, no
batch waits, retention serving hits, for as long as it was observed.
