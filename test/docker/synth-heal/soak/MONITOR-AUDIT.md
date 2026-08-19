# Soak monitor audit — what the dashboard actually measures

**Date:** 2026-08-18 · **Against:** `v1.4.6.3` (`714281b06`), executor v2-kourou

Audit of every number `soakmon.py` reports, prompted by the observation that
credit burns and credit deposits — both DN→BVN synthetics — never appear in
any count.

**The observation is correct, and the cause is worse than a missing type.**
There are no synthetic transaction counts at all. Five of the dashboard's
panels read Prometheus metrics that **no node defines**, so they display `0`
permanently, whatever the network does.

## Method

Three independent checks, because a grep alone can miss dynamic registration:

1. Every `prometheus.*Opts{}` in the tree, by namespace and subsystem.
2. `/metrics` scraped from a live node in the soak rig (911 lines).
3. `/metrics` scraped from a node on thelio after 8h under load (1015 lines).

## What the `accumulate` namespace actually contains

```
accumulate_badger_*          commit_duration, db_open, gc_duration, gc_run, txn_open
accumulate_snapshot_*        collect_account, collect_count, collect_duration,
                             collect_failed, collect_message, collect_other, collect_skipped
accumulate_tendermint*       consensus/mempool passthrough
accumulate_crosschain_heals_total              {type, partition}
accumulate_crosschain_reconcile_pulls_total
```

That is the whole set. The last two are defined in
`internal/core/crosschain/synthetic.go` (lines 48 and 59) and are the **only**
crosschain metrics that exist.

A caveat that made this confusing to diagnose: a Prometheus `CounterVec`
series does not appear on `/metrics` until it is first incremented. On a
freshly started node `crosschain_heals_total` is absent too, and looks
identical to a metric that does not exist. Only after healing occurs does it
appear. The distinction is in the source, not in a scrape.

## What soakmon reads, and whether it exists

| soakmon reads | exists? | panel it drives |
|---|---|---|
| `crosschain_heals_total` | **yes** | synthetic heals, anchor heals, per-partition |
| `crosschain_reconcile_pulls_total` | **yes** | reconcile pulls (#4073) |
| `debug_dropped_total` | **NO** | **Wedges: synthetic drops, anchor drops, by-destination** |
| `crosschain_sequence` | **NO** | **the entire flow matrix — produced / received / delivered per src→dst** |
| `crosschain_heal_deferred_total` | **NO** | **deferred (unprovable)** |
| `crosschain_heal_errors_total` | **NO** | **pull errors** |
| `crosschain_heal_focus_total` | **NO** | **focus** |
| `crosschain_heal_stuck_tries` | **NO** | **stuck (churn), stuckStream** |

Two of eight work. Six read nothing and render `0`.

## Consequences, in order of how badly they mislead

**1. There is no synthetic transaction count, of any type.**
The flow matrix is built entirely from `crosschain_sequence`, which does not
exist. So the answer to "why are credit burns and credit deposits missing" is
that nothing is counted — not `SyntheticDepositCredits` (52), not
`SyntheticBurnTokens` (53), not `SyntheticDepositTokens` (51), not any of the
seven synthetic types (49–55, now including `SyntheticLockedDeposit` from
HTLC). A per-type breakdown cannot be missing a type when there is no
breakdown.

**2. "0 wedges" is not a result.**
The wedge panel is the rig's headline safety signal, and it is structurally
zero. It cannot distinguish "nothing wedged" from "nothing measured", and it
presents the second as the first. During the 8h v1.4.6.3 run it read 0 while
**1,515 drops were induced** — countable only by grepping node logs for
`dropping synthetic envelope`.

**3. "0 errors, 0 stuck, 0 deferred" is not a result either.**
Same defect, three more panels. These are the numbers a reader uses to decide
a soak passed.

**4. Heal counts ARE real.**
`crosschain_heals_total` works, which is why heal totals moved sensibly
(3,361 over 8h) and per-partition splits looked plausible. Any conclusion
resting on heal counts stands. Any conclusion resting on wedges, errors,
stuck, deferred, or flows does not.

## Why this survived so long

The failure mode is silent by construction. A missing metric is not an error
in Prometheus — it is simply absent, and summing an empty set yields 0, which
is also the value that means "healthy". Every panel reads green.

`soakmon.py` already carries a comment about exactly this class of bug: #4087
added heal types the script did not know, and indexing a fixed dict killed the
collector thread, freezing the dashboard on its last good sample. The lesson
was applied to heal *types* but not to metric *names*.

## What to fix, in order

1. **Make absence visible.** A panel whose source metric is not present on any
   scraped node must render `n/a`, not `0`. This is the single change that
   would have prevented every wrong conclusion above, and it needs no node-side
   work.
2. **Export the drop counter.** The drop hooks already fire and log; they just
   do not count. `debug_dropped_total{kind,destination}` is what the wedge
   panel was written against.
3. **Export cross-chain sequence state.** `crosschain_sequence{type,src,dst,field}`
   with produced/received/delivered is what the flow matrix needs, and is the
   thing that would answer "are credit deposits flowing DN→BVN2" directly.
4. **Export the heal detail counters** — deferred, errors, focus, stuck_tries.
5. **Consider per-type synthetic counts.** Sequence numbers give volume per
   direction but not per transaction type. Counting by type is what makes
   "credit deposits are missing" answerable at a glance.

Until at least (1) lands, a soak verdict should quote heal counts, block
heights, loadgen stats and log-derived drop counts — and should not quote
wedges, errors, stuck or flows.
