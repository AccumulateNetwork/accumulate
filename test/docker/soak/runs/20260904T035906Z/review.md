# Acceptance run #6 — review at 55 minutes (run still going)

Run #5's code plus H7 (single-flight reconcile, deadline, per-source back-off)
and C5b (`c21d98d1a`), chaos off. 12 h / 500 tps, bcdb, 1 s blocks. At 55
minutes nothing had stalled and nothing was storming: no re-proposal spiral,
no failed-pull flood, watchdogs quiet, heap 440–700 MiB, RSS 860–950 MiB.
**User throughput was 7 tps.** The loadgen was generating 7 transactions a
second because both BVNs refused user submissions continuously from 04:25
(`batch_store_refusing=1`, 46,000 skips a minute, 832,648 skipped by 04:48).

## Why refusal never lifts: consensus is twenty minutes ahead of execution

| 04:48–04:50, from `bvn1-val1` / `bvn2-val1` logs | BVN1 | BVN2 | Directory |
|---|---|---|---|
| DAG round being voted on | 5,794–5,911 | 5,787–5,904 | 5,689–5,807 |
| leader round of the block being executed | 3,472–3,554 | 2,532–2,568 | 5,686–5,804 |
| rounds consensus commits per minute | 118 | 118 | 118 |
| rounds execution consumes per minute | 84 | 36 | 118 |
| blocks executed per minute, messages per block | 42, ~200 | 19, ~370 | 60, ~60 |
| own-store bytes (budget 32 MB) | 50 MB, +1.1 MB/min | 38 MB | 11 MB |

The Directory executes the round it certifies. Each BVN certifies two rounds a
second and executes blocks of 200–400 messages at 1.4–3 s each, so its
executor consumes a third to two thirds of what consensus commits. The
certified-but-unexecuted certificates queue in the commit channel (capacity
5,000 blocks, `DefaultCommitBufferSize`), by design: "if the executor lags,
backpressure here is the correct response; consensus certificates keep
accumulating in the channel's buffer and the DAG regardless"
(`pkg/consensus/consensus.go`). Nothing tells the proposer to stop.

An own batch stays in the own store until its block is **executed**
(`PruneCommitted`), not until it is certified. With execution 2,300–3,300
rounds behind, every own batch of the last twenty to thirty minutes is still
"uncommitted", the own store is over its share, `SubmitUser` refuses, and it
can never recover while the gap grows. The refusal is doing exactly what
invariant 4 says; it is reporting a backlog that is not the batch plane's.
The transactions being executed at 04:45 were submitted before 04:25: 2,265
"Additional transaction failed: insufficient balance: have 0" a minute on
`bvn1-val1` are loadgen sends whose funding deposit from the other BVN is in
the other BVN's own backlog.

## Why execution is slow: the healer is most of the block

Heals network-wide: 336,862 at 04:41, 406,643 at 04:46 — **230 a second**,
against user traffic of 7 a second. Every one of the eight BVN validators runs
its own gap scan every block (`requestMissingSynthetics`, `runExclusive`
per node), and the per-sequence suppression the comments still describe
(`claimSyntheticRequest`) was removed in `632c1d2a8`: there is no record of
what has been asked for. In minute 04:45 on `bvn1-val1`: 1,417 pulls for 330
distinct numbers (4.3 each); `bvn1-val2` pulled 264 distinct numbers, all 264
also pulled by val1; `bvn1-val3` pulled 1,200. BVN1's four validators made
5,631 pulls that minute (94/s) for a stream whose delivered-to-received gap was
504 numbers, of which a third were missing (#4214). Each pulled message is
re-submitted through `Submit` (system traffic, never refused), sealed into an
own batch, certified, and executed — four times over, once per validator — in
the blocks above. The source pays for each pull too: `getSynth` was 35% of a
source's CPU in run #5's hour-one profile.

So: the healer manufactures ~100 messages a second per BVN of system traffic,
the executor spends its capacity on them, consensus runs ahead, own batches
pin until execution, user traffic is refused. The 7 tps is the residue.

## Against the criteria

| criterion | 0–25 min | 25–55 min |
|---|---|---|
| memory flat | yes | yes, ~700 MiB heap, 950 RSS — but the commit channel and own store are growing behind it |
| GC not the workload | yes (0.7–1.3 cycles/s) | yes |
| CPU flat | yes, 1.0–1.8 cores per node | yes |
| consensus memory bounded | yes | own store over budget by 18 MB via `Submit`; certified certificate backlog unbounded (C6) |
| logging bounded | **yes** — H7 held: no failure flood, one line per refusal transition | yes |
| throughput | 500 tps until refusal | **7 tps** |

## What this run settles

H7 and C5b work. The next wall is not a resource; it is the healer's volume
and the fact that consensus is allowed to outrun execution without bound:

- **H6 #4212 / H1 #4193**: one pull per missing number per partition until
  it lands, with a source-side cache. This removes ~90% of the system
  traffic the executors are spending themselves on.
- **#4214**: why a third of the synthetics never arrive by dispatch.
- **C6 (new)**: consensus does not outrun execution. A header carries no
  batches while the executor is more than a bound behind; the own store's
  "uncommitted" is then a batch-plane fact again.

Files: `mem.csv`, `storage-stats.csv`, `probe-20260904T043413Z`, the hourly
capture at 04:59, `node-logs-live.txt`.
