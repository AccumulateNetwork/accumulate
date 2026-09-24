# Review: run 20260924T111811Z (run-analyst)

Run: `issue-4205-lead` @ 3a51d6d3f, the fourth Docker run of the second-pass join (E11, #4205). Relative to the third run (`20260924T093936Z` @ be4ddd0ca, whose `review.md` is the baseline), it adds #4423, #4421, #4416, #4418/#4419, #4424, #4426 and the #4425 harness changes.

- **Setup:** 30 minutes at 100 tps. 3 BVNs x 4 validators, plus the follower `acc-bvn3-fol1`.
- **How it ended:** stallkill decided to stop at **11:43:03Z** (`STOPPING: stalled 246s: BVN1,BVN2,BVN3,Directory`). The capture ended at 11:43:36, and teardown ran at 11:44:11. Load had run for about 24.9 minutes: 103,610 transactions generated at 73.6 tps.
- **Disturbances that happened:** six, all as scheduled.
  - Three restarts: acc-bvn1-val1 at 11:21:43, acc-bvn3-val1 at 11:29:50, acc-bvn2-val2 at 11:36:08.
  - Three pauses: acc-bvn2-val1 at 11:24:20 for 172 s, acc-bvn1-val2 at 11:32:39 for 79 s, acc-bvn3-val2 at 11:38:48 for 174 s.
- **Sources:**
  - `node-logs-live.txt` (ANSI stripped): 30,105 `Block execution accounting` lines and 63,120 `Sending an anchor` lines.
  - The run files: `chaos.log`, `stallkill.log`, `wedgewatch.log`, `nodestate.csv`, `monitor.csv`, `manifest.md` and `loadgen-stats.json`.
  - The two metrics scrapes: `delivery-stall-20260924T114143Z/` and `probe-20260924T114303Z/`.
  - The code at 3a51d6d3f.

## The regression: why acc-bvn1-val1's BVN1 did not rejoin

**No merge from today changed the code path that failed.** Two separate things happened.

**1. The fast path was closed by a gap at q+1.**
- The node restarted level with its peers: all four validators executed BVN1 block 157 at 11:21:42.
- The node matched its own state at 157 before any pull had written anything, exactly as in the third run. But `HasGap(158)` answered true, so the node went to the pull instead of handing off.
- This happened on **every BVN restart in this run: 3 of 3.**

  | restart | partition | first staging | outcome |
  |---|---|---|---|
  | acc-bvn1-val1 | BVN1 | `staged=1 notStaged=5 state=157` | gap |
  | acc-bvn3-val1 | BVN3 | 11:30:03 | gap, `block=638 synced=637` |
  | acc-bvn2-val2 | BVN2 | 11:36:23 | gap |

- In runs 2 and 3, the own-state matches on a BVN found **0 gaps in 3**: acc-bvn1-val1 twice and acc-bvn3-val1 once. The third run's line was `staged=1 notStaged=7 … streams=2`, and it joined 1 s later. This run's staging held `streams=3`.
- **The code that decides this did not change today.** `internal/node/join/stage.go` (`HasGap`) was last changed in #4398, and `internal/node/dagbft/collect.go` (`stageThroughNow`) has no diff between be4ddd0ca and 3a51d6d3f.
- **The only merged change on the staging path is #4423.** It changed `msg_synthetic.go` (`heldOnly` returns `errCollected` for a held copy) and `exec_stage_run.go`. Whether that changes what a collected block leaves in staging is **not verified**.
- **The log cannot settle it.** The gap line names neither the stream nor the missing number.

**2. The pull path now reaches the handoff and fails in the seed. The defect is latent, and this is the first run that reached it.** The BVN1 timeline, verbatim (the error chain is cut to its outer and inner messages):

```
11:21:43Z INFO Stopping DAG-BFT service module=dagbft partition=BVN1
11:21:47Z INFO Restored consensus position block=157 lastCommit=312 partition=BVN1 round=313
11:21:50Z INFO DAG-BFT service started module=dagbft partition=BVN1 validators=4
11:21:51Z INFO Part of a pass was served at another root, or at none, and is fetched again again=27 kept=118 module=join partition=acc://bvn-BVN1.acme
11:21:53Z INFO Staged the buffered groups through the block after the state block=158 blockRound=316 module=dagbft notStaged=5 partition=BVN1 round=312 staged=1 state=157
11:21:53Z INFO The next block has a gap; advancing the sync block=158 module=join partition=BVN1 synced=157
11:21:55Z INFO The next block has a gap; advancing the sync block=158 module=join partition=BVN1 synced=157
11:21:55Z INFO Pulled the spine, verified against a proven root module=join partition=acc://bvn-BVN1.acme
11:21:55Z INFO Pulled the accounts the block ledger named asked=118 module=join partition=acc://bvn-BVN1.acme pulled=118 refused=0 root=8c6cdf2e
11:21:56Z WARN Execution lags consensus: proposing headers without batches until it catches up lag=9 max=8 partition=BVN1
   … 4 minutes of passes, every one with again=27..274 of ~512 …
11:25:53Z INFO Staged the buffered groups through the block after the state block=394 blockRound=794 module=dagbft notStaged=6 partition=BVN1 round=792 staged=236 state=393
11:25:53Z INFO Staging settled at the block the state is block=393 directoryAnchorBlock=158 module=sync partition=BVN1 released=0 streams=3
11:25:53Z INFO Joined: executing from the block after the state alreadyInTheState=236 block=393 module=dagbft partition=BVN1 round=792 toProduce=7
11:25:53Z ERROR Handoff failed; collecting again with the groups not produced block=393 buffered=7 module=dagbft partition=BVN1 round=792
11:25:53Z ERROR The handoff failed; syncing again and handing off again error=… "produce buffered group 1 of 7 (round 794): produce block: begin block: seed synthetic cache: rebuild cache for block 383: load synthetic chain state for acc://bvn-BVN2.acme before 1208: mark point 1023 of Account.acc://bvn-BVN1.acme/synthetic.SyntheticChain.bvn2 is missing; cannot compute the state at 1207" attempt=1 block=393 module=join partition=BVN1
11:26:13Z  … attempt=5 block=405: mark point 1023 of …SyntheticChain.bvn3 is missing; cannot compute the state at 1141
11:26:21Z  … attempt=8 block=419: mark point 511 of …SyntheticChain.directory is missing; cannot compute the state at 727
11:26:32Z  … attempt=11 block=427: mark point 1023 of …SyntheticChain.bvn3 … at 1145
11:26:41Z  … attempt=14 block=438: mark point 511 of …SyntheticChain.directory … at 729
11:26:55Z  … attempt=16 block=452: mark point 1023 of …SyntheticChain.bvn2 … at 1210
11:27:13Z  … attempt=20 block=469: mark point 511 of …SyntheticChain.directory … at 742
11:27:24Z ERROR The handoff failed; syncing again and handing off again … "produce buffered group 1 of 13 (round 960): … rebuild cache for block 468: … mark point 511 of Account.acc://bvn-BVN1.acme/synthetic.SyntheticChain.directory is missing; cannot compute the state at 742" attempt=25 block=476 module=join partition=BVN1
11:27:25Z–11:43:05Z  123 x `Joining: collecting committed blocks, executing none … lastBlock=476`. The buffer grew from 14 to 919. Passes continued (again=60..308 each). The node never matched again, and made no handoff after attempt 25.
```

- **Where the bad store comes from.** `<partition>/synthetic` is not a spine account, so every pass pulls it in `pull.ModeStateOnly` (`internal/node/join/state.go:916`).
  - `pullChainHeads` (`internal/core/bootstrap/pull/pull.go:897-930`) calls `RestoreHead(want, open)` with the peer's head and **only the open mark set**.
  - The node's own chains stopped at block 157, and the peer's heads are hundreds of entries past that. Mark points 511, 767 and 1023 lie between the two and are never written.
- **Where it fails.** The first block a handoff produces opens the seed:
  - `seedCacheOnce` → the loop at `synth_cache_seed.go:86-95` → `rebuildCacheBlock` → `chain.State(from-1)` (`synth_cache_seed.go:263`) → `merkle.Chain.StateAt` → `NotFound "mark point … is missing"` (`pkg/database/merkle/chain.go:451`).
- **Why the skip rule does not save it.** The #4400 rule treats "entries not held" as the store's evidence that the node did not execute a block (`notExecutedError`), and skips the block. That check is `chain.Entries` at line 270, **after** the `State` read. So the mark-point absence surfaces as a plain NotFound, and the seed fails "as loudly as before" (d0c92e5c8).
- **This code was all in be4ddd0ca:** d0c92e5c8/#4400, a8e5953ec/#4293, and `StateAt`'s refusal to invent a missing mark point.
- **Why it had not surfaced before.** No BVN pull matched in runs 1–3. In the third run, acc-bvn3-val1 logged 80 `The spine did not verify`. Today's merges moved the pull forward: here the BVN1 spine verified on the first pass (11:21:55) and the state matched 25 times.
- **The pull's reach was the trigger, not a new bug.** The pull now reaches a handoff, and the handoff cannot seed from what the pull wrote.

**What #4421 and #4419 logged:** nothing by name.
- This build has no log line for a retake, a rewind or a held-entry repair.
- `accumulate_join_spine_stalled_entry` reads **-1** for every joining pair in both scrapes (acc-bvn1-val1, acc-bvn2-val2, acc-bvn3-val1, on both partitions), so no spine was held at an entry.
- The only trace of a chain being rewritten is the bcdb first-refusal line on acc-bvn1-val1:
  - 11:22:18, `Permanent layer refused a write: this shape is not write-once`, for `RootChain`, `BptChain` and `AnchorSequenceChain` `Element`/`Intermediate`;
  - 11:27:36, the same for `BlockLedgerChain.Element`, `MainChain.Element`, `MainChain.ElementIndex` and `MainChain.Index.Element`.
- Both come 2–5 s after a Directory demotion (below), which is consistent with a diverged chain being retaken. The line names no account (bcdb logs only the first refusal per shape).

## Verdict: per restart and partition

A pause restarts no process and gets no verdict. All three paused nodes were executing at the peers' height at the end, with 0 accounting and 0 anchor disagreements.

| restart | partition | joined | first block vs peers | agreed to the end | handoff failures | gauge | mechanism |
|---|---|---|---|---|---|---|---|
| acc-bvn1-val1 11:21:43 | **BVN1** | **No.** It restarted 0 behind (157 = peers' 157). Gap at 158, then a 4-minute pull that matched at 393, then 25 handoffs from 393 to 476. Executed 157 to the end. | — | — | **25** (metric `…handoff_failures_total{partition="BVN1"} 25`) | BOOTING throughout: gauge 0 in both scrapes, nodestate final BOOTING. | Gap at q+1, then the seed on a state-only synthetic chain (above). |
| | **Directory** | **Yes, 5 times, and it diverged each time.** Joined at 178 (11:22:04), 219 (11:22:47), 423 (11:26:29), 429 (11:26:37) and 476 (11:27:29). | 179: round 356, arrived 23, batches 11, the same as its peers. **All 44 of its Directory blocks match the peers on (round, arrived, batches), yet 16 have a different root or BPT.** | **No.** `An executed block's root is not its anchored root` at 192, 223, 426, 433 and 479: 3–14 blocks after each handoff, then BOOTING. Executed 482 to the end (peers 1365). | 0 | **ACTIVE while diverged**, 5 windows (below). The harness row saw one: `gauge ACTIVE at 27.3s`, `reached` at 188. | Divergence on the joined Directory state (NEW-1). |
| acc-bvn3-val1 11:29:50 | **BVN3** | **No.** Gap at 638 (11:30:03), then 54 x `The spine did not verify` for `acc://bvn-BVN3.acme`. Executed 637. | — | — | 0 | BOOTING. Correct. | Gap; spine never verifies (#4411 class) |
| | **Directory** | **No.** 2 x `not been collected yet` (11:29:58–11:30:00), then 76 x `The spine did not verify` for `acc://dn.acme`. Executed 618. | — | — | 0 | BOOTING. Correct. | Spine never verifies |
| acc-bvn2-val2 11:36:08 | **BVN2** | **No.** 1 gap, then 38 spine failures. Executed 956. | — | — | 0 | BOOTING. Correct. | Gap; spine |
| | **Directory** | **No.** 2 gaps, then 47 spine failures. Executed 956. | — | — | 0 | BOOTING. Correct. | Gap; spine |

**The harness agrees:** rejoined 0 of 6. The third run had 1 of 8 and the second 2 of 10.

## Agreement tables

**Accounting:** 5,363 (partition, block) groups. **0** have more than one (round, arrived, batches). The one restarted node that executed after its restart (acc-bvn1-val1/Directory, 44 blocks, 179–482) matches its peers on every one.

**Anchors,** grouped by (`source`, block), follower excluded: 4,776 groups.
- **16 have two bodies. All 16 are Directory blocks, and the odd sender is acc-bvn1-val1** each time. Its other 11 peers agree:
  - 192, 193 (at 192 only the BPT differs: root `dc09c513`, peers' bpt `463fd311` against its `64dab40f`);
  - 223–226 (223: the root is the same, the BPT differs);
  - 426–428 (426: the root is the same, the BPT differs);
  - 433–436;
  - 479–481.
- Each was sent once to each of the 4 destinations, 64 sends in all. There were 0 re-send loops (at most 1 per node, source, destination and block) and 0 self-contradictions.
- **BVN1, BVN2 and BVN3: 0 disagreeing.** acc-bvn1-val1 sent no BVN1 anchor after its restart.

Validators that sent the majority body, by the minute a block was first sent:

| minute (Z) | Directory of 12 | BVN1 of 4 | BVN2 of 4 | BVN3 of 4 | blocks with >1 body |
|---|---|---|---|---|---|
| 11:19–11:20 | 12 | 4 | 4 | 4 | 0 |
| 11:21 | 11–12 | 3–4 | 4 | 4 | 0 |
| 11:22 | 11–12 | 3 | 4 | 4 | 6 (Directory 192, 193, 223–226; acc-bvn1-val1) |
| 11:23–11:25 | 11 | 3 | 4 | 4 | 0 |
| 11:26 | 11–12 | 3 | 4 | 4 | 7 (426–428, 433–436) |
| 11:27 | 11–12 | 3 | 4 | 4 | 3 (479–481) |
| 11:28 | 11 | 3 | 4 | 4 | 0 |
| 11:29–11:35 | 10 (acc-bvn3-val1 absent too) | 3 | 4 | 3 | 0 |
| 11:36–11:43 | 9 (acc-bvn2-val2 absent too) | 3 | 3 | 3 | 0 |

The pauses do not appear in the table, because a paused node's anchors go out on resume. **Taking first-send time into account:**
- From 11:38:48 to 11:41:42, only acc-bvn3-val3 and acc-bvn3-val4 sent BVN3 anchors. That is **2 of 4, below the 3 needed**.
- The Directory ran on exactly **8 of 12**: acc-bvn1-val2/3/4, acc-bvn2-val1/3/4 and acc-bvn3-val3/4. That is the threshold, with no margin.
- acc-bvn3-val2 sent a backlog of 75 BVN3 anchors at 11:41 and 117 Directory anchors to BVN1 at 11:42 after it resumed.

## Why the run stopped

- **It was a delivery stall, not a block stall.** At the decision, soakmon had every partition `stalledBy=delivery` with `blocksStalledFor=0`: Directory and BVN1 at 246 s, BVN3 at 54 s, BVN2 at 13 s. Every partition closed blocks every minute; BVN1 closed 46–59 a minute through 11:42.
- **It started ~11:38:57, the second acc-bvn3-val2 paused.**
  - acc-bvn3-val1 had never rejoined, so the pause left BVN3 with 2 of 4 executing validators and no BVN3 anchor quorum.
  - At the 11:41:43 capture every flow out of BVN3 was red: BVN3→BVN1 91 undelivered, BVN3→BVN2 84 (lag ∞) and BVN3→Directory 46. All three had `recv == deliv < sent`.
- **What was still red at the decision.** After the resume at 11:41:42 the BVN3 flows were draining (BVN3→BVN2 78 s behind at 11:43:03). What remained red was BVN2→BVN1 (822 undelivered, 169 s), BVN1→BVN3 (255, 372 s), BVN2→Directory (221) and BVN1→Directory (70). wedgewatch had the worst stall down to 36 s (BVN2) by 11:43:26.
- **So stallkill fired on a stall that had begun to clear.** Its cause is the one the third run predicted: joins that never complete use up the fault margin (BVN3 3 of 4, Directory 9 of 12), and a single pause then takes a partition below its anchor threshold.
- **#4423 held.**
  - 0 `Stream stopped` lines.
  - `accumulate_exec_run_stopped_total` has no series in either scrape. It is a CounterVec, so it was never incremented.
  - 0 `Stream position` lines with `waiting=0` and reach > delivered. The third run's stop cause (a stream frozen with every number held) did not recur.

## ACTIVE while not executing in agreement: yes, for the first time in these four runs

acc-bvn1-val1/Directory was ACTIVE during five windows, each ended by its own root check:

| window | executed after the handoff | divergent anchors it sent |
|---|---|---|
| 11:22:04–11:22:16 | 179–193 | 192, 193 |
| 11:22:47–11:22:49 | 220–226 | 223–226 |
| 11:26:29–11:26:31 | 424–428 | 426–428 |
| 11:26:37–11:26:40 | 430–436 | 433–436 |
| 11:27:29–11:27:31 | 477–481 | 479–481 |

- **Verbatim, the first window's end:** `11:22:16Z WARN An executed block's root is not its anchored root block=192 matched=191 module=join partition=acc://dn.acme`, then `WARN This node is not executing in agreement; it is BOOTING until it hands off again block=192`.
- **Demotion (ACTIVE → BOOTING) works**, which the third run could not show. But the node signs and sends its anchor for block N before the anchored root for N arrives. **Every window cost 2–4 divergent signed anchors, 16 in all.**
- With 11 agreeing validators they did not move the quorum, and the follower report counted them: `root/BPT mismatches 16, first … block 192`.
- No other node read ACTIVE while diverged. Every other restarted pair read BOOTING throughout.

## Counts against the third run

| line | third run | this run | where |
|---|---|---|---|
| `Main not found` | 126 | **8** | All are `An account could not be pulled`, none are handoff failures. 6 are `acc://bvn-BVN1.acme/synthetic` on acc-bvn1-val1: every peer answers `queued local delivery acc://dd3e8024…@lg-ebc68f70….acme/tokens: … Message.dd3e8024….Main not found`. 1 is `acc://bvn-BVN1.acme/anchors` (`entry 8965 … the peer does not serve the transaction aa2c2a3e a message refers to`). 1 is `bvn-BVN2.acme/synthetic` on acc-bvn2-val2 at 11:41:50. |
| `does not hash into` | 4 | 2 | acc-bvn1-val1, refetched: `lg-07e4dc6de414959b.acme/book/2` (11:23:56) and `lg-fae5752c88cbd5ca.acme/tokens` (11:38:09). |
| `Handoff failed` / `The handoff failed` | 126 / 63 (Directory) | **50 / 25 (BVN1)** | All acc-bvn1-val1 BVN1, 11:25:53–11:27:24, all the seed's mark point. Metric: 25. |
| `not its anchored root` | 0 | **5** | acc-bvn1-val1 Directory, blocks 192, 223, 426, 433, 479. |
| `Stream stopped` (new) | — | **0** | `exec_run_stopped_total`: no series |
| `spine_stalled_entry` (new) | — | **-1** on all 6 joining pairs, both scrapes | 0 `spine is stalled` lines |
| `dispatcher_refused_total` (new) | — | **no series** (never incremented) | |
| heal refused | — | **0** | No `outcome="refused"` child of `conductor_heal_requests_total` on any node, and 0 log lines. |
| heals (monitor) | 8,405 | **9,168** | Over 24.9 min of load; the third run had 26.7 min. |
| loadgen reads lost | 0 | **0** | 36,691 NotReady and 382 transport errors, all retried elsewhere. |
| read probe | 1,541 failed (NotReady included) | 36 failed (all 8 s timeouts), 1,179 NotReady, of 3,275 timed | |
| stranded (headline) | 66 (re-read) | **116**, worst 18 on acc-bvn1-val2/BVN1 | Rising 49 → 116 over the last 5 samples. The final row's own readings sum to 102. Not a drained sample (stallkill). |
| `The spine did not verify` | 80 + 47 + 21 | 1 + 1 (acc-bvn1-val1) / 76 + 54 (acc-bvn3-val1) / 47 + 38 (acc-bvn2-val2) | |
| `Dispatch queue past its block bound` | 162 | 111 | 10 at 11:2x, 101 at 11:3x–11:4x |
| `Permanent layer refused a write` | 26 | 36 | 25 are the usual `Message.(hash).Main` at 11:21:0x / 11:22:36. The 11 on acc-bvn1-val1 are chain rewrites (above). |
| `Vote channel full` | 722 | 877 | |
| `Consensus stalled` | 48 | 102 | All acc-bvn3-val1/Directory |
| `A reader is holding an old database version` | 52 | 67 | |

## What the run proves

- **#4421/#4416 fixed the third run's Directory handoff failure** (`load anchor pool main chain entry N: Message….Main not found`): 0 of those this run, and acc-bvn1-val1's Directory handed off five times.
- **The BVN pull can now verify its spine and match.** acc-bvn1-val1/BVN1: `Pulled the spine, verified` on the first pass, and matched 25 times from 393 to 476.
- **Demotion ACTIVE → BOOTING fires on a root mismatch.** 5 of 5, each in the same second as the check.
- **#4423 held:** no stream stopped without a hole.
- **#4419's gauge reads -1 (no stall) on every joiner.**
- **0 accounting disagreements across 5,363 blocks.**

## What it does not prove

- **That any restarted node can rejoin:** 0 of 6.
- **That the gap at q+1 is, or is not, caused by a merge.** 3 of 3 here, 0 of 3 before, and the log names no stream.
- **Anything about #4424 or #4426 in action.** No heal was refused and nothing was refused to the dispatcher, so neither path was exercised.
- **Anything past 24.9 minutes of load.** No node was restarted twice.

## New

- **NEW-1: a joined Directory diverges within 3–14 blocks on the same inputs.**
  - acc-bvn1-val1/Directory, 5 times. (round, arrived, batches) are identical to the peers', while the BPT differs first (blocks 192, 223, 426: root equal, bpt different).
  - So the state the join handed off from is not the peers' state, even though the root matched at the handoff block. Not root-caused.
  - A debugger should start from the v3 API on acc-bvn1-val1 against a peer at Directory 192.
- **NEW-2: divergent anchors are signed before the node can learn it diverged.** 16 blocks, 64 sends. #4385 gates ACTIVE on the handoff, but nothing gates a Directory anchor on the anchored root for the same block.
- **NEW-3: the synthetic-cache seed fails on a state-only `<partition>/synthetic` chain**, because the mark points between the node's old head and the pulled head are never written. The mechanism and code path are above. This one blocks every BVN join that has to pull.
- **NEW-4: every BVN restart found a gap at q+1** (3 of 3), and the gap line names neither the stream nor the number, so no instrument says which one.
- **NEW-5: peers cannot serve `acc://bvn-BVN1.acme/synthetic` whole** when it holds a queued local delivery whose message they lack: `Message.dd3e8024….Main not found` from every peer asked (acc-bvn1-val1 11:22:35 and 5 more).
- **Harness:** the rejoin row's `gauge ACTIVE at 27.3s` for acc-bvn1-val1/Directory is a sample taken inside the first divergent window. The row's verdict (NOT rejoined, disagrees at block 192) is correct.
