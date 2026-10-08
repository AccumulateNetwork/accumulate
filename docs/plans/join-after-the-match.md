# Join: after the match — plan

Branch `test-4530-4542`. Owner: this session. Paul, 2026-10-08: "fix the
barriers to loading"; synced is the match and executing in agreement, the
fill and the history backfill are background work.

The rule of this file: every step has a state and an exit measurement.
Nothing is built that is not a step here; a step is added before it is
started, and its state changes the moment it does. A step whose exit
measurement fails is marked FAILED with the number, not reworded.

Review: `docs/reviews/sync-state-pull-2026-10-07.md` §§7–11.

| # | step | state | exit measurement |
|---|---|---|---|
| 0a | #4552 records answer names only, states 16-wide as of T | DONE `28c0adc56` | joins one round each; run 180957Z 10/10 ACTIVE 72–226 s |
| 0b | #4553 in-state groups count as executed at the handoff | DONE `c31f1dacf` | every restarted validator lag 0, refusing 0 |
| 0c | #4554 fix 1: the backfill yields to the fill | DONE `7db7d2c5d` | no `chain` calls on a follower while `sections_not_loaded > 0` (sampler) |
| 0d | remove the overlap's wait for the fill | FAILED, reverted `c920657e0` | cold load after the overlap diverged (demoted at 23): no retention ⇒ the wait stays |
| 1 | #4554 fix 2: the fill loads sections in 256-leaf spans | BUILT (unit: 8 pages + 8 states calls for 1,565 sections vs 1,601 + 1,565); live numbers pending step 2 | `TestTheFillLoadsSectionsInSpans`; live: fill of ~3,500 sections ≤ 2 min, overlap `checked` ≤ 40, hold worst-behind ≤ 200, every hold back to 0 |
| 1b | node-wide fill gate: the history backfill of any partition waits until no partition of the node is still filling (the Directory's backfill ran 14 chain calls/s through BVN1's fill: per-partition gate) | BUILT, in the next run's followers | sampler: 0 `chain` calls on a follower while any partition has `sections_not_loaded > 0` |
| 2 | attach run: validators + followers rebuilt in place from step 1, 10 tps, follower walk | TODO | 12 h: every join ACTIVE ≤ 300 s through universe 6,000+, every hold PASS; the sampler on each follower's first 5 min |
| 3 | uptime, controlled: at +2 h recreate one validator in place with the load kept; 64-account call on it and on an untouched peer every 10 min for an hour | TODO | drops to ~0.25 s and climbs again ⇒ the node's own state, an issue naming the mechanism; stays with its peers ⇒ the traffic |
| 4 | #4548 + the window: spine = trust chain only, pool/ledgers by head + mark set, `TakeChainsAt` only for whole accounts, history from the backfill | TODO, needs Paul's go | spine ≤ 5 s, records phase ≤ 2 s per round; a joined node's synthetic proofs on anchors below its handoff wait for the backfill (DIFFERENCES E20) |
| 5a | #4551 chain tails out of the state answer (mark-point rule) | TODO, needs Paul's rule | state answer ≤ 25 % of today's bytes; `TestAStateAnswerCarriesNoChainTail` |
| 5b | #4546 cold-cache read cost (options a/b; BlockchainDB#103) | TODO, needs Paul's choice | 64-account call under load ≤ 0.3 s at +4 h |
| 5c | the follower's own writes in the sections phase (`keepSpan` commits; 6,461 accounts in 26 s with the validators answering 64 in 0.3 s) | TODO | sections phase ≥ 1,000 accounts/s; store-side issue filed if BlockchainDB |

Order: 1 → 2 and 3 in one run → 4 → 5a–5c. Step 1 is an afternoon.

## Log

- 2026-10-08 17:10 CT: file created after run 180957Z (stopped by Paul at
  +3 h 18). Steps 0a–0d were done before the file existed; 0d is the
  excursion this file exists to prevent.
- 2026-10-08 17:40 CT: step 1 built; the fill sorted ascending first and fell back to the per-section path every time (the test caught it: 1,601 pages); highest-first fixed it.
- 2026-10-08 17:55 CT: step 1: a span whose page no peer serves (a peer holding a section by its marker refuses it, naming the holder) is loaded a section at a time through the redirect — `TestASectionOnlyOnePeerHoldsIsLoadedThroughARedirect` caught it (83 sections left). Step 2 needs no validator roll: the fill and the gate are follower-side; validators stay at c31f1dacf.
- 22:24Z step 2 launched: attach run, followers @dbf0b21c1, validators @c31f1dacf, 10 tps, follower walk; suites green before launch (join 666 s, e2e 313 s).
- 22:32Z step 2 join 1: ACTIVE 61 s; overlap 4 min 57 s, checked=195 (was ~13 min / 95 at 8 s each); fill 4,032 sections in 4:50 (~14/s vs 3.5/s). Short of the step-1 bar (≤2 min, ≤40 blocks).
- 22:43Z step 2 hold 1 PASS, worst 108 (was 240–708). Sampler: fill bounded by 256-account states calls at ~2 s each (validators under load) and by the Directory backfill leaking through the per-partition gate → step 1b.
- 23:00Z step 2 join 2 ACTIVE 103 s; overlap 7:19, checked=156; hold 2 PASS worst 304 (was 437–566). Step 1b built (daemon gate `everyPartitionPastItsFill`).
