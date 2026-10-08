# Review: how a join pulls the state — 2026-10-07

**Goal (Paul, 2026-10-07):** a joining node pulls the state of every account.
It needs no history to process transactions; histories come later. During
the state pull it pulls account states and nothing else.

**Evidence:** soak run `test/docker/soak/runs/20261007T141334Z` (follower
walk, attached to the 37-hour network at ~1 block/s and 10 tps), followers
`acc-bvn1-fol1` (19:04–19:35Z) and `acc-bvn2-fol1` (19:36Z–), their
`accumulate_pull_*` and `accumulate_join_*` counters, their logs, and direct
timed queries of `acc-bvn2-val1`'s API. Code at `test-4530-4542` @ 889f68254
(`internal/node/join`, `internal/core/bootstrap/pull`, `internal/api/v3`,
`pkg/database/bpt`). Both followers ended NEVER ACTIVE at 30 minutes; the
first four of the run matched in 2–28 minutes, the next five did not.

## 1. What the pull takes

Per `PulledState.Pull` (`internal/node/join/state.go`), each round, in order:

| Phase | Calls | What the validator reads | What comes back |
|---|---|---|---|
| anchors | `anchorsrc.Collector.Read`, ≤32 `QueryAccount` | the anchor pool's newest entries | signed anchors (the root to match) |
| peers-block | `default` on each peer's ledger | ledger main state | the lowest block any peer is at |
| top | `bptBlock` (273 calls over a join) | BPT interior to depth 12 | 4,096 section hashes as of S |
| spine (once) | `pullOne` in `ModeFullSpine` / `ModeWindow` | the trust chain whole; the anchor pool, ledger and synthetic ledger by a window of chain entries, with messages | accounts **with chain entries** |
| records | `changedAccountStates{After: L}` (36 calls / 23 min) | block-ledger records after L, and each named account's state, current | records + states, ≤1 MiB pages |
| owed | `accountStates` for what a batch did not answer | as below | |
| sections | per page: `bptPage{ForHeight: L}` (256 leaves, 451 calls / 23 min) then `accountStates{ForHeight: L}` in chunks of 64 (765 calls) | the BPT as of L (`GetRangeAt`), then per account as of L: `BPTRootAt`, `GetReceiptAt`, `ResolveHistoricalAccountState`, `retainedAccountAt` | each stale leaf's state |

Besides the state, three things ride along:

1. **Chain entries on every record answer** — `takeStates` calls
   `pull.TakeChainsAt` for the windowed accounts (ledger, anchor pool,
   synthetic ledger) every time a record names them, which is every block.
   The spine step is where the spec puts the window; the rounds re-pull it.
   **#4548.** (A fix is in progress, uncommitted at the time of writing:
   chains only in the spine step; every account a round takes is left to
   the backfill.)
2. **Chain tails inside every state** — `AccountState.OpenEntries` is each
   chain's entries since its last mark point (`merkle.Chain.OpenSet`). For
   64 BVN2 accounts: 194 chains, 3,552 entries, 248 KB of a 315 KB answer —
   **79% of the bytes are chain entries.** The leaf hashes only the chain
   head (`merkle.State`: count and pending hashes), which is what an append
   needs; the open set writes the next mark point and serves receipts into
   the tail. **#4551 — decision needed.**
3. **Queued delivery messages** — `pull.KeepQueuedMessages` pulls the
   messages behind the synthetic ledger's local delivery queue. Small, and
   the executor needs them to deliver; not a problem.

The pull also reads the windowed accounts' chain *index* chains to find the
window, once per account per join; small.

## 2. What it costs, measured

**Per state answer (validator side, direct HTTP to `acc-bvn2-val1`, current
state, during the join):**

| Accounts | Time | Bytes |
|---|---|---|
| 1 | 0.33 s | 38 KB |
| 8 | 0.42 s | 54 KB |
| 32 | 0.66 s | 162 KB |
| 64 | 2.0–2.8 s | 315 KB |
| 4 × 64 at once | 1.44 s wall (0.58–1.44 each) | |

As of a past block (`forHeight = H−5`, `H−300`): 2.2–3.3 s for 64 — **the
historical path costs about the same as current.** The "as of a block" rule
is not where the time goes. A repeat of the same `H−300` query took 0.12 s,
so the validator's store caches the answer's reads.

Per account the validator does ~9 store reads: BPT leaf, main state, leaf
state, and per chain (3 on average) the head and a 256-entry tail chunk.
With BlockchainDB's history lookups cold (#4546, BlockchainDB#103) each is a
pread binary search; the host disk was at **9,500 reads/s, 60 MB/s, 1.18 s of
read time per second**, load average 40 on 24 cores.

**Per page (follower side), from the counters:** `bptPage` 0.22–0.27 s,
then 1–3 `accountStates` calls at ~1.7 s each in parallel, then the write.
About **2.6 s per page, ~100 accounts per page, ~40 accounts/s.** 451
pages in 23 minutes.

**Concurrency:** 60 s at 20:00Z: 47 state calls = 78.5 validator-seconds,
23 page calls = 5.1 s — **1.4 queries in flight** of the 16 the four
validators admit (`statesInFlight` 4 × 4 peers). The BVN2 validators showed
0.3–0.7 inbound `query:bvn2` streams each. The follower's own gate waited 35
s in total over the join, so the gate is not the limit; the page loop is
(`loadOpened` → one `loadOpenedPage` at a time; a page's ~100 stale accounts
are 1–3 chunks). **#4549.**

**The round:** bvn2-fol1's last two rounds: records 58 s + sections 467 s +
top/spine 62 s = 587 s; then records 118 s + sections 927 s = 1,046 s. The
network moved 587 and 1,046 blocks meanwhile; every block's record names the
accounts it changed, so the next round's records are longer and open more
sections. At 10 tps with hash-distributed accounts, a 600-block round names
thousands of accounts across most of the 4,096 sections: **the sectioning
does not bound the work on a loaded partition**, it bounds it on a quiet one.
Once a round is longer than its records cover, L never reaches the head
("the records after L were more blocks than one answer covers").

**The partition:** BVN2 holds ≈96k leaves (9 full pages of 4,096 covered
1,566 sections at 23.5 leaves each; the 10th page was refused by the
validator's gate), 9–42 per section, none over a page. At 40 accounts/s the
whole state is 40 minutes of pulling, against a 30-minute deadline, before
the chase.

**What else is on the validators during the join:** the same node's
Directory joins in ~2 minutes and starts its history backfill (ACME's main
chain, 377k entries; dn.acme/ledger and dn.acme/anchors chains) while the
BVN is still pulling: **853 `chain` queries in 60 s, ~160 validator-seconds**
— three times the BVN pull's validator time — against the same twelve
validators, through the same per-peer gate on the follower. Over 23 minutes:
14,651 chain queries (3,955 validator-s) vs 765 state queries (1,289
validator-s). **#4550.** Also after the Directory is ACTIVE, its root watch
asks the validators for signed anchors ~1,300 times a minute (`asks=16472`
in 12 minutes); not measured in validator time.

## 3. Findings, ranked

1. **The pull is throughput-bound by its own serial page loop, not by the
   validators.** 1.4 of 16 slots used; validators answer four 64-account
   calls in the time of one. Pipelining pages across validators is the
   largest single gain available and needs no spec change (the spec already
   says pulls run in parallel). #4549.
2. **The state answer is mostly chain entries.** 79% of the bytes and one
   extra read per chain per account are the open mark sets. Under Paul's
   rule they belong to the history backfill; the leaf proves the head
   without them. Needs a decision on how a mark point is written before the
   tail is backfilled. #4551.
3. **History is pulled during the state pull, twice over:** the windowed
   accounts' new chain entries on every record answer (#4548), and the
   Directory's whole history backfill on the same node and validators while
   the BVN is still syncing (#4550). Both are the rule broken, and #4550 is
   the largest query load on the validators during a join.
4. **Per-account validator cost (~10 ms marginal, 2–3 s per 64 under load)
   is store reads**, cold-cache bound (#4546) — but see §5: the validators'
   read latency is nearly flat through a run; what grows is the join's work. Fewer reads per account (finding
   2) and the resident index BlockchainDB#103 asks for are the levers; the
   "as of a block" rule is not (measured equal).
5. **Sections do not bound the work on a loaded partition.** Every round
   opens most sections, and the round grows with the network's movement
   during it. The pull must pull the whole state faster than the network
   changes it; with 1–3 above it can (96k accounts at 16× today's
   concurrency is a few minutes), without them it cannot.

## 4. Recommendations

1. Pipeline the sections phase: up to 16 state calls in flight across the
   opened sections, answers written as they land (#4549). Expected: the
   sections phase from 467–927 s to under 100 s per round at today's
   per-call cost.
2. Chains only in the spine step; every account a round takes goes to the
   backfill (#4548; fix in progress).
3. Gate the node's history backfill on every one of its partitions being
   ACTIVE, and rate-limit it (#4550).
4. Decide #4551: open mark sets out of the state answer, into the backfill
   as a `SyncedEntries` gap, with the mark-point rule settled first.
5. Then re-measure the join on the 37-hour network: accounts/s during the
   sections phase, round length vs. blocks moved, and time to ACTIVE, one
   follower at a time (the walk).

## 5. Why a join slows down through a run and is fast again at the next (Paul, 2026-10-07)

Paul: "Times should be the same every time, only maybe degrading a bit as the
databases get bigger." They are not, and the cold cache (#4546) cannot be why:
a cache does not warm on a restart, and the validators were **not** restarted
between run 20261007T050444Z (joins failing from 03:10Z) and run
20261007T141334Z (first follower ACTIVE in 112 s at 14:25Z) — they had been
up since 22:36Z the day before. What restarts with a run is the load
generator, and it **bootstraps a fresh account universe every run**:

| minutes into the run | accounts it has created (three runs) |
|---|---|
| 8–10 | 122–165 |
| 26–28 | 625–665 |
| 45–49 | 1,234–1,268 |
| 70–76 | 1,911–2,180 |
| 91–110 | 2,518–3,215 |
| 141 | 4,188 |

At 10 tps the block-ledger records name ~18 accounts a block whatever the
universe (BVN2 validator, blocks 215757+, 216485+, 220261+: mean 17–18), but
the *distinct* accounts a span of blocks names grow with it: 163 over 40
blocks at +10 min (the whole universe), 239 at +27, 348 at +91. A join's
work per round is the distinct accounts its records name, each opening a
section the node loads whole (~23 leaves): run 211137Z's first rounds went
53 sections / 1,196 accounts at +10 min → 261 / 6,104 at +27 → 394 / 9,500
at +91. Early in a run the universe is a few hundred accounts and every
round names the same ones; two hours in it is thousands, and a round opens
most of the tree. The serial page loop (#4549) fell over at ~40 accounts/s
once a round's work passed what it could load before the network moved on;
at ~160 accounts/s the fifth follower (+91 min, 9,500-account first round)
still matched in 638 s.

The validators themselves barely move: the read probe's p50 was 3.1–5.9 ms
through run 050444Z and 4.5–5.9 ms through 141334Z, p95 31–61 ms; across
the day p50 rose ~50% (4.2 → 6.6 ms) as the store grew — the "degrading a
bit" Paul expects. The join time is a property of the test's traffic shape,
not of the network's age: the follower walk should record the load
generator's universe (`accounts=`) beside each join so joins are compared
like for like, and a "same every time" expectation needs a load generator
whose working set is the same every run.

## 7. After #4549: the records phase is the next wall (run 20261007T211137Z)

With the sections loaded at once, ten followers in a row were ACTIVE in
92–972 s (the same network that had failed from the fifth follower on) and
every hold passed. The eleventh, at +4 h 8 min with the load generator's
universe at ~6,000 accounts, was NEVER ACTIVE: its rounds were 14–16 s, of
which the records phase was 10–12 s for ~17 blocks of records and the
sections 3–4 s, and the network moved 14–16 blocks in each — the join sat
2–3 blocks behind the head for twenty minutes. `recordsOnce` fetches the
`ChangedAccountStates` answer a page at a time and the validator reads every
named account's state inside the answer at ~30 ms; at 10 tps a block names
~18 accounts, and once they are distinct that is ~0.5 s of serial validator
time per block of records — parity with a 1 block/s head. **#4552**: the
states of a records answer fetched in parallel as the sections' are (names
in the answer, `askStates` 16-wide), or the answer's pages fetched by index
concurrently; either amends "One answer is the node's whole catch-up".

## 8. Not verified

- That the Directory backfill is what makes a 64-account answer take ~2 s:
  it takes 2–2.8 s from an idle HTTP client too, with the backfill running.
  Which validators the backfill's queries land on was not attributed (the
  stream gauge is too coarse for short streams).
- The follower's own write per page (`pull.KeepStates`, one commit per page
  into BlockchainDB) — about 0.7 s per page is unaccounted for by queries
  and is probably this; not measured separately.
- The records phase per account: 36 calls, 62 s; accounts per page unknown.
- The exact leaf count (the 10th page was refused); ≈96k is an
  extrapolation from nine full pages.
- Whether the 0.33 s for one account is a fixed per-call cost or that
  account's 38 KB of open entries.
- §9: that join 2's last-round union fit one page is inferred from the
  absence of the `fallen behind` line for that round (one page is one view,
  so T = Block); the page count of a past answer cannot be reproduced, the
  validators retain BPT history for 1,024 blocks only.

## 9. The one-page threshold: why a join converges or never does (Paul, 2026-10-08)

Paul: "Resetting the network makes bootstrapping faster — same history,
faster bootstrapping, until the network has been running for a while, then
it is slow again." Worked against the code and the validators' records.

**What a reset resets.** The validators were started 2026-10-06 22:36Z and
never restarted through runs 050444Z, 141334Z and 211137Z (`docker inspect
StartedAt`). The history is never read by a join: it takes the records after
L — the last tens of blocks — and the sections they open. What a run resets
is the load generator (a fresh universe, from zero accounts) and the
followers' stores. So what makes a join fast or slow is in the last few
blocks' records at the moment of the join, and in the states they name.

**The convergence rule** (`state.go` `Pull`, `recordsOnce`;
`account_states.go`). A round is complete when L ≥ the highest block any
reply was read at. A `ChangedAccountStates` answer is the union of the
distinct accounts the gap's records name, served in pages of 1 MiB of
marshalled states; T is the peer's block at the first page and each page is
its own view. A one-page answer is one view, so Block = T and the round is
ready. A two-page answer's second page is served ~10 s later at a later
block, Block > T (`the join has fallen behind`), and the round repeats from
L with the blocks that passed meanwhile. **A join converges when, and only
when, a round's gap names a union that fits one page.** Every round of the
eleventh follower (+4 h 08) logged `fallen behind`; join 2 (+20 min) logged
it for rounds 1–7 and not for round 8, which converged. The two joins'
steady-state rounds were otherwise alike — records 0.55 vs 0.65 s/block,
17–23 blocks a round, 13–19 s — the difference was the threshold, not the
speed.

**Both sides of the threshold grow with the load generator's run, and reset
with it.** The picker is uniform over the universe (`randIdentity`,
`randLite`), so the distinct accounts a window names rise toward names ×
blocks as the universe grows; and a state grows with the account's age
(entries since its last mark point, key pages, credits). BVN2 validator,
blocks of run 211137Z and of the run before it:

| point | names/block | distinct in 17 blocks | in 40 | in 100 |
|---|---|---|---|---|
| +10 min (universe ~150) | 15.7 | 69 | 112 | 173 |
| +20 min (join 2, ACTIVE in 324 s) | 17.7 | 156 | 239 | 354 |
| +1.3 h | 16.7 | 160 | 301 | 557 |
| +2.3 h | 16.0 | 156 | 324 | 638 |
| +3.3 h | 16.5 | 161 | 319 | 748 |
| +4.3 h (join 11, NEVER ACTIVE) | 17.0 | 189 | 370 | 801 |
| previous run +3.4 h / +5.0 h / +5.9 h | 15–16 | 153 / 190 / 154 | 350 / 382 / 323 | 678 / 737 / 707 |

Names per block are constant; distinct names over a round's gap grow
through the run (and the previous run's last hours sit where this run's
fourth hour does, which is where both failed). The states grow too: the 167
accounts of join 2's converging round need two pages *today* (93 on the
first) and the 189 of join 11's rounds need two (131 on the first) — the
same accounts, four hours older. Early in a run a gap of 17 blocks names
~70 young accounts, a page; four hours in it names ~190 older ones, over a
page, and the round that reads them takes ≥10 s (the serial ~30 ms per
account on the validator, #4552), in which ~17 more blocks pass naming
another ~190: a stable orbit two pages wide. Restart the load generator and
the next join's gap names a few dozen small accounts again.

**Not the cause:** validator age (never restarted; read-probe p50 3–6 ms all
day), the size of the history (never read), the partition (BVN1, BVN2 and
BVN3 join alike), the Directory (joins first every time).

**What would falsify it.** (a) Restart the validators mid-run with the load
generator kept: the theory says joins stay slow. (b) Cap the universe (no
`add-account` past a few hundred): joins stay fast all run. (c) Raise
`accountStatesBudget` from 1 MiB: later joins converge, with no change to
the validators. **What removes it:** #4552 option 1 — names in the records
answer, states fetched `askStates` 16-wide — makes the answer a few KB and
always one page, so the threshold is gone and a round's length is the
states' parallel fetch; #4551 (chain tails out of the state) shrinks the
states that set the page.

## 10. #4552 built; the restart that exposed #4553; what is left is the chains (run 20261008T180957Z)

**#4552 option 1 built** (`28c0adc56`): the records answer is records only,
the states asked 16-wide as of T, one commit a step. To measure it the 12
validators had to serve the new answer, so they were recreated **in place**,
one at a time, from the rebuilt image — all node state lives in the one
shared volume `disoak_network-config`, so the 41-hour history survived; each
rejoined in 62–163 s and the network made blocks at 1/s throughout.

**That exposed #4553.** Every restarted validator refused user submissions
for good: `execution_lag_blocks` stuck at exactly its rejoin's
`alreadyInTheState` (125 on BVN1, 50 on the Directory) because
`performHandoffAt` dropped the collected groups the pulled state already
held without reporting them executed. The load generator sent 0
transactions in 30 minutes (run 20261008T170913Z, stopped at +30 min: joins
on an idle network prove nothing). Fixed `c31f1dacf` (the in-state groups
count as executed at the handoff; spec invariant 9); the validators were
rolled a second time, every lag gauge 0 after.

**Run 20261008T180957Z** (validators and followers @`c31f1dacf`, 10 tps,
follower walk, load real: 10.0/s, 0 refusals): joins 1–3 ACTIVE in 72 s
(universe 149), 102 s (647), 142 s (1,146) — the same slots were 92, 324,
334 s the run before — all holds PASS so far. The validators' per-call cost
measured directly (BVN3, idle): a 64-account `AccountStates` as of T 220–320
ms, a BPT page 120 ms, the records answer 0.0 s.

**Where the first round's time goes now** (third follower, pull-query
counters sampled every 2 s across both partitions):

| phase | wall | chain calls (query s) | accountStates calls (query s) |
|---|---|---|---|
| top+spine | 35 s | 1,548 (131) | 4 (2) |
| records, 246 accounts / 34 blocks | 25 s | 477 (60) | 1 |
| sections, 6,461 accounts | 26 s | 168 (29) | 208 (288, 16-wide) |

The records phase is no longer the states: it is `TakeChainsAt` on the
windowed system accounts every record names (anchor pool, ledger, synthetic
ledger) — 477 chain calls. The spine is the same, 1,548 chain calls. What is
left of spine+records is #4548 and the window ("The window", executor.md):
the change Paul described on 2026-10-08 — the spine is the trust chain,
validated once; the pool and the ledgers taken by head and mark set like
every other account; their history from the backfill. The sections phase is
bounded by the follower's own writes now (6,461 accounts in 26 s with the
validators answering 64 in 0.3 s), a store-side item.

## 11. Stopped by Paul at +3 h 18: the barriers after the match, and what slows with uptime (run 20261008T180957Z)

**Result:** 10 of 10 joins ACTIVE in 72, 102, 142, 142, 144, 152, 132, 152,
142, 226 s (universe 149 → 4,971; the same slots the run before: 92, 324,
334, 273, 638, 578, 224, 972, 851, 598 s), every join one round. Holds: 8
PASS, 1 FAIL (hold 8: 708 behind at worst, 216 at the end), 1 cut short by
the stop.

**The hold's crawl (#4554).** After ACTIVE a follower executed at 0.07–0.17
blocks/s for 10–13 minutes, then at 4–5 (monitor.csv, both of bvn2-fol1's
holds). The overlap settled every block for as long as the fill ran (its
rule from run 20261002T090546Z, "a clean run before every section is loaded
is not settled"): 95 blocks checked, ~8 s each; and in that window three
things shared the follower's per-peer query gate — the history backfill
(1,633 chain calls in 80 s), the fill (605 states calls, 593 pages) and the
settle — every call queued ~4.5 s (`gate_slot_wait` +358 s in 80 s). Paul:
synced is the match and executing in agreement; neither the fill nor the
backfill is part of it, they are background work, and the only question is
whether they make keeping up harder. Built: the backfill yields to the fill
(`7db7d2c5d`, an order between background tasks, not a sync condition) and
the overlap ends on its clean blocks whatever the fill is doing (a block's
in-line section load is as of the node's own block, checked against the
marker; the old divergence predates the sectioned sync).

**What slows with uptime (Paul's hypothesis: caches or "optimizations"
whose overhead exceeds their benefit).** The per-call cost of a 64-account
`AccountStates` on the same validators: 0.22–0.32 s forty minutes after
their in-place restart (under 10 tps and join 2), 0.44–2.2 s four hours
later (under 10 tps and join 10), 2.0–2.8 s on the 37-hour-old validators
the day before — and **22–44 ms idle**, 25 minutes after the load stopped.
So the answer itself is cheap; what grows is contention on the validator
under load. Not pinned to a cache yet: the hourly CPU profiles at +1/+2/+3
h have the same shape (syscalls 23–27 %, ed25519 10 %, GC 15–20 %), the
sampled live heap grew only ~24 MB between +1 h and +3 h (consensus
headers/certificates ~100 MB, BlockchainDB segment blooms 70 MB, the bcdb
immutable cache 57 MB, pubsub buffers 58 MB), `heap_inuse` 700 MB → 1.03 GB
with a GC every ~5 s, plain reads flat (read probe p50 3–4 ms all run), and
the store's shallow misses 84k → 2.06M and deep misses 107k → 467k in
three hours (#4505: reads that leave the shallow window and walk). The
controlled test is a validator restarted mid-run with the load generator
kept: if its calls drop back to 0.25 s while its peers' stay at 1–2 s, it
is the node's own state (caches, segments, retention); if not, it is the
traffic. Not run.
