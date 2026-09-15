# Capture: Directory store held view (accumulate#4279)

Taken 2026-09-15T14:00Z from run 20260915T042428Z, live, ~9.6 h in.
BlockchainDB main `fac421a`; accumulate `590884c1b` (issue-4263-stored-intermediates).

## Why this exists

`accumulate_bcdb_oldest_view_age_seconds{database="dnn"}` was 22,764 s on every
node, identical to within 10 ms, with `staged_commits{dnn}` = 51. The BVN store
on the same nodes read 0 for both. The view opened about 07:36Z. The Directory
had stopped producing blocks (frozen at 26,617) and anchors (stopped at 3,483)
while the BVNs ran on past 33,000.

Wedgewatch's own captures are from 06:15Z, BEFORE this view opened, so they do
not contain it. These are the only dumps taken while it was held.

## The finding these dumps support

**No goroutine is blocked for anything near the view's age.** The longest waits
in every dump are 569 minutes, which is process lifetime (startup goroutines).
So the view is a reference that was opened and never released, not a reader
stuck in a call. A goroutine dump therefore cannot name the holder, which is
what the open-time warning added by D5 is for. That warning does not appear in
this run's logs.

`acc-bvn1-val1.goroutines.txt` does show a live read resolving through
`bcdb.(*Database).getAt(..., 0x44d4, ...)` = version 17,620, while the
Directory was at 26,617 — a read served at a version ~9,000 blocks behind.

## Files

- `*.goroutines.txt` — full stack dumps, 3 nodes, debug=2
- `metrics-*.txt` — full Prometheus scrape, includes the bcdb gauges
- `soakmon-data.json` — the dashboard's full state, includes the flow matrix
- `dn-synthetic-ledger.json` — `acc://dn.acme/synthetic`: produced=1 against
  delivered=32,469 / 67,958
- `bvn1-val1-logs-0730-0745Z.txt` — node log around the moment the view opened
- `Shard0000/` — one shard of a DN store, streamed in a single pass while live,
  so it may be torn; bcdb recovery handles that, but it is a crash-consistent
  copy rather than a clean one

## What this run is not

The soak harness was killed at minute 8, so stallkill never guarded the run and
there is no verdict. Measurements are all taken live from the nodes and stand on
their own; the run cannot be cited as an acceptance result.

## Finding (worked 2026-09-15, after the capture)

The dumps were read together with the full node log and the live network,
which was still up. Every question in #4279 is answered by the log.

**The holder is named.** The D5 warning did fire: 24 lines of `A reader is
holding an old database version`, on all eight nodes, at 05:40:46Z (age 10s,
version 4064, current 4074), 05:41:17Z (age 41s, current 4099) and
12:00:00Z (age 6h19m24s, current 4114, overlays 50). The opener is
`block.(*Executor).Begin` -- the Directory's own block batch. It is not in
`node-logs-live.txt` after 12:00Z because the warning, the age gauge and the
staged-commits gauge are all computed inside `commit`, and the Directory
store has not committed since 12:00:00Z (its 12h major block). The gauge's
22,764 s is the age as of that last commit, not the age now: 05:40:36Z to
12:00:00Z. The view did not open at 07:36Z; that figure came from treating a
frozen gauge as live.

**The batch leaked because Block.Close failed.** At 05:40:36Z, on every
node, Directory block 4114 failed to close:

    build receipt for entry 2754 (to 2756) of BVN1 intermediate anchor chain:
    index 2754 is outside the segment [2755, 2756]

Block 4114 received three BVN1 anchors in one healed envelope (source
blocks 4039, 4040, 4041). Staging released them in order and they executed
in order, appending 2754, 2755, 2756. Then the bundle folded each message's
state into the block through a map sorted by message hash, which threw
that order away: the spans reached `ChainUpdates.Merge` as 4041, 4039,
4040. The merge joined two adjacent spans in either order but had no case
for a third adjacent to neither, so 4039's span was dropped, the block's
segment was [2755, 2756] with 2754 missing, and the receipt for it could
not be built from memory. The map was never looked up; it existed only to
sort, and the sort was the bug.
`ExecutorBridge.ProduceBlock` returned the Close error without discarding
the block, so the batch begun at store version 4064 was never released.

**The stall is the execution-lag bound, not the view.** Nine committed
groups failed the same way between 05:40:36Z and 05:41:24Z (the anchors of a
failed block are never delivered, so healing re-sent them, larger). A failed
group is never reported executed, so the Directory's lag reached 9 against a
bound of 8 at 05:41:33Z and has stayed there: the primary has proposed
headers without batches ever since, refusing every submission including the
BVNs' anchors. That is why the Directory's anchors stopped at 3483 and its
ledger froze at block 26,617. The view is a consequence of the same failed
block, not the cause of the stall.

**The reads are current.** An API query begins a batch at the store's
current version; only the leaked batch reads at 4064. `produced=1` on
`acc://dn.acme/synthetic` is the Directory's own production, which is
normally near zero; the dashboard cell that was climbing was BVN1 to
Directory (sent 222,840 against 32,478 received), which is the backlog the
stall created. Nothing here implicates the store: BlockchainDB served every
read it was asked, and the adapter's D5 machinery worked as designed.

**The sent/delivered readings.** The matrix and `streams.py` read one node.
A validator answers from its own store, as current as its own executor, and
for partitions it does not validate the router picks a peer, a different one
each call. Run 20260906T134054Z ended with that node's BVN1 executor 348
blocks behind its siblings, so the final report showed BVN1 -> Directory as
produced 100,804 against received 102,177: the destination had received more
than the source had sent. Live, the same mixing reads as sent and delivered
flickering between a lower and a higher value. The Directory -> BVN `sent`
cell was 0 in every capture of this run and 1 at 14:00Z; whatever was seen
climbing there is not in any snapshot we have. Fixed in the readers: every
ledger is read from every node and each field is the max across answers, a
field that then goes DOWN is logged as `SEQUENCE REGRESSION` and turns its
cell red, and every history sample now carries the whole matrix
(REPORTING-SPEC 1b).

**The healing zeros.** The board read HEALS 0, ANCHOR 0, SYNTHETIC 0, HEAL
ERRORS 0 and "none yet" for the whole run while the node log held 4,028
span requests and 31,307 "still in flight" answers, and the scrape held
`accumulate_conductor_heal_entries_total 18171`. The monitor was reading
`accumulate_crosschain_*` families that no longer exist anywhere in the
node and rendering the absence as 0 -- the clause-1 violation the reporting
spec had recorded against #4093 and never closed. Fixed: the monitor reads
the exported families (`conductor_heal_requests_total` by source,
destination and outcome; `conductor_heal_entries_total`;
`exec_staged_proofs_total`; `staging_held_*`; `dispatcher_drops_total`), a
family no node reports renders `— not measured`, and the spec's family table
names what exists.

**Nothing logged the streams.** No node line said where a synthetic or
anchor stream stood block by block, so a stall could only be read off the
dashboard after the fact, with no record of when delivery stopped or what
it was waiting on. Added: every block logs `Stream position` per stream it
advanced or that is behind (delivered, advanced, sighted, reach, held,
waiting-on) and `Stream produced` per destination (from, to, count), at
Info under `module=stream`; `test/docker/soak/streamlog.py` reads a stall,
a value that went backwards, or a numbering gap back out of the node log.
Specified in executor.md, "What a stream logs".

**Fixes** (accumulate, uncommitted at the time of writing):

- The bundle's hash-sorted state map is gone. `bundleStates` is a slice
  in execution order, folded in that order, so chain spans reach the block
  as the chain was written. `ChainUpdates.Merge` joins only a span that
  continues the segment and records any other as an error, which
  `buildDirectoryAnchor` refuses before building receipts: an out-of-order
  fold now fails the block with a message naming the span, instead of
  leaving a hole. Tests for the container, the merge, and the refusal.
- `block.(*Block).Close` discards the batch, cache and staging view when it
  returns an error; `ProduceBlock` checks for a missing batch before it
  begins a block. Tests for both.
- The `oldest_view_age_seconds` help text says the value is as of the last
  commit or release, and a view release now refreshes the gauges.

Open, not fixed here: a committed group that fails to execute is logged and
skipped, and its transactions are lost while the lag it leaves behind is
permanent. That policy is what turned one bad block into a dead partition.

## Review of these fixes (2026-09-15, after the write-up above)

A full review of the change set found two defects in the monitor fixes that
every unit test had passed over, and three more in the executor work:

- **`collect_metrics` raised `NameError` on every tick.** Deleting the dead
  `crosschain_*` aggregation took the node/memory/lifecycle aggregation with
  it, so `/data` never got past `{"ok": false}` and every watchdog that reads
  it — stallkill, wedgewatch, seizewatch, ladder — silently stopped tripping.
  The dashboard's script also failed to parse (a `const` shadowing one already
  in `tick()`), so the page rendered nothing at all. Both are now covered by
  `test_soakmon_contract.py`, which runs `collect_metrics` against a stubbed
  scrape and parses the page with `node --check`; neither could be caught by a
  unit test over a helper function.
- **`BLOCKS produced` was the fleet sum**, so the board read 687,160 against
  partition heights totalling 111,869. Every validator produces the same
  blocks: that counter is a partition fact and now takes the max, while
  genuinely per-node counters still sum.
- **The seizure watchdog read an unmeasured signal as zero.** With `stuck` no
  longer exported it printed `stuck=0` and could never trip; it now prints
  `n/a`, says so once, and trips on gap and undelivered only.
- **`Block.Close` was only half the leak.** `BlockState.Commit` has two error
  paths of its own — the pre-commit publish, and a `Conflict` that returns
  before the change set commits — and neither released the batch. Both do now.
- **A segment fault failed the whole block.** It is recorded per chain and
  refused where that chain's receipt is built, so one unbuildable receipt
  cannot become a partition that never closes a block again.

One consequence is recorded in the spec rather than fixed: folding in
execution order changes the receipt order inside a `DirectoryAnchor`. Nothing
votes on a block hash under DagBFT; what differs is the anchor body each
Directory validator signs, so a BVN could not gather a quorum for it, and the
system ledger that stores it, so state trees would diverge. It is ungated because `V2Kourou` has never run a
network that outlives a soak, and every soak starts from genesis. If Kourou is
activated on a live network before this lands, it needs a gate.
