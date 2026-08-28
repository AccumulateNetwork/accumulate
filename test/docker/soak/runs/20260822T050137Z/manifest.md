# Soak run 20260822T050137Z

**Purpose:** 12h chaos soak on the #4125/#4128 fixes plus batch-lifecycle instrumentation (eedd47685). Retention window (10m/4096) keeps committed batches fetchable; re-delivered certificates are skipped instead of waited on. New metrics: certificates_redelivered_total (must stay 0 — nonzero means commit dedup is still wrong upstream and the fix is masking it), retention hits/expired/held for window sizing, batch_waits_total by reason, and blocks_produced vs blocks_empty to tell an idle network from a wedged one. stallkill now refuses to count an idle stretch against its threshold, after it killed the previous run four minutes in during the loadgen bootstrap wait.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T05:01:37Z |
| commit | `eedd47685e1c65c53e7db755dff7b08520668f6a` |
| describe | `10k-tps-418-geedd47685-dirty` |
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

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T05:23:02Z |
| elapsed | 0.24h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 7423 |
| heals | 0 -> 1869 |
| chaos events | 1 |
| monitor samples | 4 |
| seizure | SEIZED at 2026-08-22T05:22:56 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=78 undeliv=synthetic BVN2->Directory undeliv=5 |
| reconcile pulls (#4073) | 6987 |
| stalled channels at end | 9 |
| wedge captures (#4125) | 1 wedge-20260822T050634Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Note on the single wedge capture

`wedge-20260822T050634Z` is a FALSE POSITIVE, not a wedge. It was taken during
the load generator's bootstrap wait, when the network is idle and committing
empty rounds: every height freezes, the monitor reports "stalled", and
wedgewatch had no idle guard yet. The guard was added mid-run (`f7e91c0ea`) and
wedgewatch restarted against this run, so no further idle captures were taken.

The run itself showed no wedge at any point: `redelivered=0`, `batch waits:
none`, and retention serving 216 hits. It was stopped deliberately to fix two
load-generator defects it exposed (#4129, #4130).
