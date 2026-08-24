# Test reporting specification

What a test run MUST report, and what the words on a dashboard are allowed to
mean. This exists because every clause in it has been violated, at cost:
panels rendered `0` for instruments that do not exist, a load generator
reported `success` for transactions the node rejected, containers reported
`healthy` for a chain that had stopped, and a 10,500 TPS result was published
with no chain-side evidence whatsoever.

Normative language: MUST / MUST NOT are requirements. A run that violates one
is not evidence of anything.

## 1. Displayed means measured

A dashboard panel MUST show a value only if the instrument behind it exists
and was read. An absent instrument MUST render as **`— not measured`**, never
as `0`. Zero is a measurement; absence is not.

## 1a. No impossible states, and bounds over blanks

A panel MUST NOT display a value that another value on the same panel
disproves. `received: 23, sent: 0` is not a missing measurement — it is an
assertion of P and ¬P, and it teaches the reader to distrust the whole board.

Where chain truth proves a bound, the display MUST show the bound rather than
an absent instrument's zero: 23 received proves `sent ≥ 23`, so render
`≥ 23 (inferred)`. An inconsistency between an instrument and an inference
MUST itself be surfaced as an alarm — it means an instrument is broken, which
is a finding, not a rendering choice.

Self-streams (a partition's deliveries to itself) are real sequenced streams
that carry real traffic and can wedge like any other — measured: BVN2→BVN2
`produced=1 delivered=1` during the #4103 bisection. The flow matrix MUST
display the diagonal, not delete it as "bookkeeping".

"No stream entry yet", "zero traffic", and "instrument absent" are three
different facts and MUST render distinguishably.

## 2. The chain is the source of truth

Every claim about network behaviour MUST be derived from chain state or node
telemetry, never from the tool that generated the load:

| claim | authoritative source |
|---|---|
| blocks produced | system ledger `index` per partition |
| a transaction happened | its effect on chain state (account exists, balance moved) |
| delivery state per stream | synthetic/anchor ledger `produced`/`received`/`delivered`, per src→dst |
| recovery activity | heal / reconcile counters exported by the node |
| induced faults | container logs (`dropping … envelope`) |

## 3. Required node exports

Nodes MUST export the following Prometheus families (namespace `accumulate`).
This is the contract the soak monitor is written against:

| family | kind | labels | meaning |
|---|---|---|---|
| `crosschain_heals_total` | counter | type, partition, remote | messages healed, by mechanism |
| `crosschain_reconcile_pulls_total` | counter | partition, remote | "anything new?" pulls that recovered data |
| `crosschain_heal_deferred_total` | counter | partition, remote | heals deferred by pacing |
| `crosschain_heal_errors_total` | counter | partition, remote | heal attempts that failed |
| `crosschain_heal_focus_total` | counter | partition, remote | focused-stream heal attempts |
| `crosschain_heal_stuck_tries` | gauge | partition, remote | consecutive failed tries on the stuck head |
| `crosschain_sequence` | gauge | partition, remote, kind={synthetic,anchor}, dir={produced,received,delivered} | per-stream sequence state — the flow matrix |
| `debug_dropped_total` | counter | kind, partition | envelopes deliberately dropped by fault injection |

Exported: the first two, on both branches. **Missing: the remaining six — which
is why the flow matrix and wedge panels have never shown a true value** (#4095).

The consensus-status API MUST additionally report `syntheticHeals` and
`anchorHeals` (#4075) — the coarse monitor's CSV reads them.

## 4. Health means liveness

A container healthcheck MUST fail when the node's partition ledger stops
advancing beyond a threshold. `13/13 healthy` over a chain that wrote nothing
for 12 minutes (#4103) is a false report, and it is the one report everyone
checks first.

## 5. Load generators are witnesses, not referees

A load generator MUST report, distinctly:
- **requested vs achieved rate** — silently substituting another rate (#4102) is a defect
- **accepted vs rejected**, with rejection reasons — counting HTTP acceptance as success (#4104) is a defect
- **followed-to-delivery outcomes** — a bounded number of followed transactions
  that never landed MUST fail the run (`-max-stranded`)

A generator that cannot produce fee-valid work (#4107) MUST refuse to start
rather than submit doomed transactions.

## 6. Provenance

Every run MUST record before load starts: commit, `git describe`, branch,
uncommitted-file count and patch, image ID, executor version, topology,
settings (duration, rate, drops, healing), and the config files as run.
Results MUST be appended to the same manifest. (Implemented — `soak.sh`.)

## 7. Observation

The monitor MUST be running and verified before load starts, and the run MUST
abort if it is not (implemented — the soakmon gate in `soak.sh`). The
dashboard MUST be visible to the operator, opened for them, before load
starts (implemented — `run-remote.sh`).

## 8. Summaries in, raw data out

Test results are reported as summaries — in the issue the run served, and in
the run's `manifest.md` / `runs/INDEX.md`. Raw test data MUST NOT be
committed: no logs (`*.log`), no heap/CPU profiles (`*.pb.gz`), no goroutine
dumps, no node-log captures. They stay on the machine that ran the test
(enforced — `runs/.gitignore`); anything a reader needs from them goes into
the summary. On 2026-08-24 the full history of every branch and release tag
was rewritten to purge previously committed raw data — do not reintroduce it.

## Current compliance

| clause | state |
|---|---|
| 1 displayed=measured | **violated** — soakmon renders absent metrics as 0 (#4093) |
| 1a no impossible states | **violated** — anchor flow shows `sent 0` beside `18/18 received`; diagonal deleted (#4093, #4095) |
| 2 chain as truth | met by soak.sh/loadgen; violated by parallel-loadtest's recorded results |
| 3 node exports | **6 of 8 families missing** (#4095); consensus-status fields missing on DI (#4075) |
| 4 health=liveness | **violated** (#4108) |
| 5 generator honesty | met by tools/cmd/loadgen; parallel-loadtest fails all three (#4102, #4104, #4107) |
| 6 provenance | met |
| 7 observation | met, as of this branch |
