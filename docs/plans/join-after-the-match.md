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
| 1c | the fill's drain: `fillReady` 64 → 256, span size probed once per fill (each pass re-paid a probe page and stopped at the cap after a span or two; the cap × block rate bounded the fill at ~40 sections/s) | BUILT, next run | pass line: `paused-on-ready` rare, pages ≈ sections/10; fill of ~4,000 sections ≤ 2 min; overlap `checked` ≤ 60 |
| 1d | the records' states: a peer that cannot answer as of T is passed over for one that can; current states only when no peer can (the inline fallback made a round repeat for one busy answer — joins 5–8 took 2–3 rounds with nothing owed) | BUILT, next run | joins one round each through universe 6,000+; `chunks-taken-current` ≈ 0 in the records/sections lines |
| 2 | attach run: validators + followers rebuilt in place from step 1, 10 tps, follower walk | TODO | 12 h: every join ACTIVE ≤ 300 s through universe 6,000+, every hold PASS; the sampler on each follower's first 5 min |
| 3 | uptime, controlled: at +2 h recreate one validator in place with the load kept; 64-account call on it and on an untouched peer every 10 min for an hour | DONE (run 232237Z): val2 178→392 ms over the hour, val1 578–745 throughout ⇒ the node's own state | drops to ~0.25 s and climbs again ⇒ the node's own state, an issue naming the mechanism; stays with its peers ⇒ the traffic |
| 3b | name the mechanism: timed snapshots (every 10 min) of both nodes' `stats.json` and `/metrics` for per-hour deltas — commits, deep misses, dyna/perm walks, history reads by caller, heap, GC — plus a CPU profile of each under the same 64-state calls; not segments (24 vs 27) | TODO | one counter or profile frame that is 3x on the old node per hour, named; then its fix as a step |
| 4 | #4548 + the window: spine = trust chain only, pool/ledgers by head + mark set, `TakeChainsAt` only for whole accounts, history from the backfill | TODO, needs Paul's go | spine ≤ 5 s, records phase ≤ 2 s per round; a joined node's synthetic proofs on anchors below its handoff wait for the backfill (DIFFERENCES E20) |
| 5a | #4551 chain tails out of the state answer (mark-point rule) | TODO, needs Paul's rule | state answer ≤ 25 % of today's bytes; `TestAStateAnswerCarriesNoChainTail` |
| 5b | #4546 cold-cache read cost (options a/b; BlockchainDB#103) | TODO, needs Paul's choice | 64-account call under load ≤ 0.3 s at +4 h |
| 4a | memory A/B: val1+val2 of each BVN at `GOGC=200`, `GOMEMLIMIT=2000MiB`, `mem_limit 2348m` (arm A); val3+val4 unchanged at 1700MiB/2048m/default GOGC (arm B); all 12 recreated in place together | BUILT `a41b2b0fa` | per arm at +1/+4/+8 h: 64-state call ms, GC/min, CPU %, RSS; A flat while B climbs ⇒ GC pressure is (part of) #4555 |
| 4b | the walk's ACTIVE/hold lines carry the overlap's length and `checked` | DONE | lines show both in follower-report.md |
| 4c | readprobe: a 64-account AccountStates as-of-block call per validator per round | DONE | per-validator ms in every round line, so the arms compare without hand sampling |
| 4d | relaunch: followers from HEAD (1c, 1d), attach-10tps-followers.conf, walk on | STOPPED 17:54Z (~5 h) for 4g | step 2's exit measurement; monitor up and load real before stopping to watch |
| 4e | blocks matched: gauge `accumulate_join_matched_blocks` (contiguous blocks settled against their record or under a matching anchored root; 0 on mismatch/demotion); dashboard follower column = blocks matched / current height | gauge BUILT (unit test only; join suite not run); dashboard height column live, matched not yet shown | column rises 1:1 with the partition once caught up, drops only to 0 |
| 4f | Paul 2026-10-09: more than 100 blocks behind ⇒ stop executing txs and process block-ledger entries until caught up; then execute again, checked against the block ledger | TODO — spec first | a follower never holds > 100 behind while executing |
| 4g | Paul 2026-10-09: the overlap ends on its clean run whatever the fill; a section a block loads ahead of itself (not the marker's, taken as of peer block P) is settled by the records through P + 10 (`SettleMargin`), the overlap over or not; a difference there is a reconciliation, not a return to the ledger. Replaces 0d's revert | BUILT: unit `TestASectionLoadedAheadIsSettledByTheRecordsThatFollowIt`; e2e `TestAColdLoadAfterTheOverlapIsSettledByTheRecords` (0d's scenario: overlap ended 16, cold deposit 23 reconciled, no demotion; fails without: demoted at 23); join suite + 54 e2e join tests green; soak pending | live: overlap ≈ 20 checked, ends in < 1 min; 0 demotions; hold worst-behind falls |
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
- 23:22Z step 2 join 3 ACTIVE 174 s; overlap 15:17; hold 3 FAIL worst 856/last 795 — the Directory backfill (ACME main) ran through BVN3's fill (5,868 chain calls; states calls 5.6 s each). Run stopped; relaunch with 1b (ef269c4d6).
- 23:31Z step 2 relaunched with followers @ef269c4d6 (1b). Teardown of 221608Z lingered ~10 min on stallkill (soak.sh waits for it); gone on its own — harness note.
- 23:40Z run 232237Z join 1 ACTIVE 102 s; overlap 5:21 checked=206; 1b holds (DN backfill began only after BVN1's fill). Fill rate ~12.6 sections/s: its own states calls are the limit, not the backfill.
- 23:50Z hold 1 PASS worst 146. Live: a 256-leaf page covers 9–12 sections, a span's states fit one 1 MiB page — the follower made 1,908 pages + 2,265 states calls for 4,032 sections (should be ~370): spans throttled or bypassed; suspects fillReady=64 per block and the shared gate. Instrumenting the fill (per-pass line) for the next run.
- 00:05Z run 232237Z join 2 ACTIVE 124 s; overlap 7:02 checked≈? — unchanged by 1b: the fill is bounded by its drain (fillReady × block rate). Step 1c built.
- 00:18Z run 232237Z join 3 ACTIVE 154 s; overlap 5:57 (was 15:17 without 1b).
- 00:37Z run 232237Z join 4 ACTIVE 122 s; overlap 7:11 (universe 1,840).
- 01:06Z run 232237Z: hold 4 PASS 391; join 5 ACTIVE 295 s (spine 63 s, records 30 s, sections 27 s; universe 2,490); hold 5 PASS 383. Dashboard: followers' ok-blocks column was frozen (join-log source) — fixed, soakmon restarted via its supervisor at 01:04Z.
- 01:25Z run 232237Z: join 6 ACTIVE 183 s (universe 3,033); hold 6 PASS 381. Six for six at +2 h. Step 3 launched 01:26Z: bvn3-val2 recreated in place (image from ef269c4d6), load kept; 64-state call on it vs bvn3-val1 every 10 min for an hour (scratchpad uptime-step3.log).
- 01:36Z run 232237Z join 7 ACTIVE 244 s (universe 3,633). Step 3: bvn3-val2 recreated 01:26Z, ACTIVE 01:30:48, accepting submissions 01:31:11; +10 min first-call: val2 178 ms vs val1 578 ms (before: 1,088 / 990).
- 01:47Z hold 7 PASS 513. Step 3 +20 min: val2 191 ms vs val1 711 ms (first call); +10: 178 vs 578 — restarted validator 3–4x cheaper per call under the same load, two samples.
- 01:57Z join 8 ACTIVE 355 s (universe 4,343; joins 5–8: 295/183/244/355). Step 3 +30: val2 349 vs val1 644 — val2 climbing with its own uptime (178→191→349).
- 02:05Z hold 8 PASS 294. Join 8 was 3 rounds (172/84/50 s) with nothing owed: the as-of-T fallback to current on the first failing peer → step 1d built. Step 3 +40: val2 338 vs val1 586.
- 02:12Z join 9 ACTIVE 305 s (universe 4,969). 1d committed bacf1751b.
- 02:26Z hold 9 PASS 281. Step 3 series, first-call ms (restarted val2 / untouched val1): +10 178/578, +20 191/711, +30 349/644, +40 338/586, +50 423/err — val2 climbs from a restart toward its peers within the hour under the same traffic: the node's own state, not the traffic.
- 02:37Z step 3 DONE: +60 val2 392 vs val1 745 ms. Restarted node 3x cheaper at first, climbs toward its peers within the hour under identical traffic ⇒ the node's own state. Mechanism not yet named (3b).
- 02:55Z hold 10 PASS 413; join 11 ACTIVE 355 s at universe 6,228 (NEVER ACTIVE two runs ago). Step 3 done; per-hour deep misses 306k (old) vs 100k (restarted), commits 4,750 vs 15,100, segments 24 vs 27 — step 3b to name it.
- 03:10Z hold 11 PASS 414 — 11/11 joins and holds at +3h45, universe 6,228. #4555 filed.
- 03:16Z join 12 ACTIVE 265 s (universe 6,811).
- 03:30Z hold 12 PASS 477 — 12/12 at +4 h, universe 6,811. Continuing; reporting on change of state only.
- 03:53Z join 13 ACTIVE 376 s (universe 7,499); hold 13 PASS 308.
- 04:13Z join 14 ACTIVE 264 s (universe 8,043); hold 14 PASS but worst 732 — backlog climbing with the universe; 1c is the built fix, not in these followers.
- 04:34Z join 15 ACTIVE 234 s (universe 8,597); hold 15 PASS 493 — 15/15 at +5h10.
- 04:53Z join 16 ACTIVE 194 s (universe 9,125); hold 16 PASS 431 — 16/16 at +5h30.
- 2026-10-09 CT: Paul: GOGC 200 + 300 MiB more container and GOMEMLIMIT, as an A/B; walk overlap fields; readprobe states call; relaunch. Steps 4a–4d added.
- 2026-10-09 07:13 CT: 4a built a41b2b0fa (compose arm A). 4b done: walk ACTIVE/hold lines end in `overlap BVN1 7m23s checked 78, dn 0m13s checked 20` (checked against run 232237Z's log).
- 2026-10-09 07:14 CT: 4c done: readprobe's round adds one 64-state call as of a block per validator (readprobe-states.csv + a 'states' log line); live check on the idle network reads 'not read' (400 blocks changed only system accounts), as expected without load.
- 2026-10-09 07:45 CT: 4d: validators built from 9f78a3a7d, all 12 recreated together 12:24:52Z (arms verified by docker inspect; every validator ACTIVE, heights agree). Launch failed at the follower build: disk 100% full — Dockerfile `COPY . .` took test/docker/soak/runs (128 GB) into the build context; build cache 206→343 GB. Build killed before any load; validators unharmed (no ENOSPC, no restarts, heights agree). Fix: .dockerignore excludes soak runs.
- 12:47Z (07:47 CT) 4d launched: run 20261009T124645Z, validators 9f78a3a7d (A/B arms), followers d401137bb; monitor 200 on the tailnet, load running; first states round (fresh restart, both arms): 33–203 ms on all 12.
- 2026-10-09 09:10 CT: Paul: 'ok blocks' told nothing — replaced on the dashboard by current height (behind); blocks matched gauge built (4e, WIP); catch-up rule given (4f). Paul: stop. Run 20261009T124645Z still running.
- 2026-10-09 CT: 4g built. Found by Paul's question 'why wait for all sections?': in run 232237Z the overlap's extra 40–186 blocks per join caught nothing (34 overlaps, 0 differences); the wait only hid an unsettled load-ahead. Spec §3a amended. Note: DIFFERENCES E29 is cited (executor.md:630, sections.go) but was never written; executor.md 'K and the clean run' still says eight, code is 20.
- 12:57 CT: run 124645Z stopped by hand at ~5 h for 4g; relaunched as run 20261009T175728Z, followers from f984c68e5, validators untouched (A/B continues from 12:24:52Z).
- 13:05 CT: run 175728Z join 1 (bvn1-fol1): ACTIVE 162 s; BVN1 overlap ended at proven+20 exactly (18 checked, 2 frontier, 0 differences, 0 reconciled) in 1m58s with ~3,100 sections still unloaded — 4g works. But the follower executes ~0.2 blocks/s vs network 1/s, before and after the overlap (behind 138 → 178 in a minute): the settle was never the bottleneck; execution with sections unloaded is. Cause not yet measured (suspects: inline cold-section loads per block, the fill's states calls). Paul's 4f rule addresses it.
