# Review: run 20260924T074702Z (run-analyst)

Run: `issue-4205-lead` @ 3a02ec41d. This was the second Docker run of the second-pass join (E11, #4205), and it carries the fixes from the first run (`20260924T052134Z`, whose review is the baseline here): #4397/#4399, #4398/#4401/#4402, #4400, #4407, #4362e, #4363, #4395, #4404.

- **Setup:** 30 minutes at 100 tps. 3 BVNs x 4 validators plus the follower `acc-bvn3-fol1`.
- **How it ended:** it ran to completion, with driver exit 0 at 08:21:02Z. Stallkill never fired. Wedgewatch took one capture at 08:14:19Z (`partitions stalled 121s: BVN2,BVN3,Directory`), which was a delivery stall, not a halt: every partition closed blocks every minute of the run.
- **Criterion (manifest):** every restarted validator rejoins on both partitions, and the Directory is 12 of 12. **It failed.** 2 of the 10 partition rejoins succeeded, and the Directory ended with 7 of 12 validators executing.

Sources:
- `node-logs-live.txt` (ANSI stripped). From it I extracted every `Block execution accounting` line (41,341) and every `Sending an anchor` line (83,652).
- `chaos.log`, `nodestate.csv`, `submissions.csv`, `monitor.csv`, `loadgen-stats.json`, `readprobe-report.md`, `streams-final.txt`.
- `wedge-20260924T081419Z/*.metrics.txt`, the only metrics scrape in the run directory, taken at 08:14:19Z.

## Verdict: per restart and partition

Nine disturbances happened, as scheduled: five restarts and four pauses. A pause gets no join verdict. "Start" is the container start in `nodestate.csv`.

| restart | partition | joined (block, time from start) | first block after handoff vs peers | agreed to the end | handoff failures | re-sync after mismatch |
|---|---|---|---|---|---|---|
| acc-bvn1-val1 07:51:01 | **BVN1** | **Yes.** `Joined; executing from the block after the state block=180 executes=181` at 07:51:11 (9 s). It restarted 1 block behind: `alreadyInTheState=0 toProduce=8`. | 181: round 362, arrived 62, batches 7, the same as val2, val3 and val4. The anchor for 181 matches. | **Yes.** 1,667 blocks (181–1925) all match the peers' (round, arrived, batches). 1,441 anchors (180–1850), 0 disagreeing. | 0 | not needed |
| | **Directory** | **No.** 170 `Joining: collecting committed blocks, executing none` to 08:20:57. The buffer reached 1,680 groups. Executed block 184 for the whole run. | — | — | 0 | — |
| acc-bvn3-val1 07:59:03 | **Directory** | **Yes, then diverged.** `Joined … block=633 executes=634` at 07:59:10 (7 s). | 634 matches the peers. So do 635–657. | **No.** Block 658 has the same round, arrived and batches as acc-bvn3-val2 (1358/30/11), but a different root. See NEW-A. | 0 | **Did not converge.** `An executed block's root differs from its proven root; syncing again block=658` at 07:59:32, then 342 `An account could not be pulled`, all its own. Still on 661 at 08:21. |
| | **BVN3** | **Yes.** `Joined … block=655 executes=656` at 07:59:16 (13 s). | 656: round 1312, arrived 72, batches 7, the same as its peers. | **Yes.** 1,210 blocks (656–1927) and 1,043 anchors (655–1858), 0 disagreeing. | 0 | not needed |
| acc-bvn2-val2 08:06:11 | **BVN2** | **No.** 83 `Joining …` to 08:20:56. 66 passes, and every one logged `The spine did not verify; it is asked for again`. Executed block 1030. | — | — | 0 | — |
| | **Directory** | **No.** 83 `Joining …`. 149 `The next block has a gap; advancing the sync` (08:07:06–08:13:01), then passes that never prove. Executed block 1043. | — | — | 0 | — |
| acc-bvn1-val3 08:11:39 | **BVN1** | **No.** 52 `Joining …`. Its passes settle about half their accounts each time (average kept ~230 of ~510) and never match. Executed block 1375. | — | — | 0 | — |
| | **Directory** | **No.** 53 `Joining …`. 52 gap lines (08:14:00–08:16:34), then passes that never prove. Executed block 1350. | — | — | 0 | — |
| acc-bvn3-val3 08:18:56 | **BVN3** | **No, within the 2 min 4 s it had.** 11 `Joining …`. 22 `An anchor was refused … the anchor carries no signatures` for BVN3 blocks 1388–1409 at 08:19:17 (NEW-D). Executed block 1805. | — | — | 0 | — |
| | **Directory** | **No, within the 2 min 4 s it had.** 11 `Joining …`. 54 passes that did not prove (42 served at 1549, 12 at 661). Executed block 1745. | — | — | 0 | — |

**Handoff failures.** `accumulate_join_handoff_failures_total` has no series on any of the 13 nodes at the 08:14:19 scrape. It is a CounterVec (join.go:35), so it was never incremented. The log has 0 `The handoff failed` lines through 08:21:01, so there were none after the scrape either. The only handoffs attempted were the three that succeeded.

**The last restart (08:18:56Z).** It had 124 s before the end. The load generator's 30 minutes ended at about 08:19:00, so it also had essentially no load.
- **What can be said:** in 124 s neither partition joined, and it signed nothing divergent.
- **What cannot be said:** whether it would have joined, or how long it would take. The one thing it showed that no other restart did is NEW-D (anchors served with no signatures).
- **Its effect on the network:** its Directory departure took the Directory from 8 to 7 executing validators, below the 2/3 anchor threshold (see below). The Directory sent no anchor that any BVN received after 08:18:59.

## Agreement tables (anchors, grouped by source partition and block)

Each cell is the number of validators that sent the majority body, taken over the blocks first sent in that minute (min–max). The follower is excluded. BVN anchors go only to the Directory; Directory anchors go to all four partitions.

| minute (Z) | Directory of 12 | BVN1 of 4 | BVN2 of 4 | BVN3 of 4 | blocks with >1 body |
|---|---|---|---|---|---|
| 07:48–07:50 | 12 | 4 | 4 | 4 | 0 |
| 07:51–07:58 | 11 (acc-bvn1-val1 absent) | 4 | 4 | 4 | 0 |
| 07:59 | 10–11 | 4 | 4 | 4 | **3: Directory 658, 659, 660 (acc-bvn3-val1)** |
| 08:00–08:05 | 10 | 4 | 4 | 4 | 0 |
| 08:06–08:10 | 9 (acc-bvn2-val2 absent too) | 4 | 3 | 4 | 0 |
| 08:11–08:17 | 8 (acc-bvn1-val3 absent too) | 3 | 3 | 4 | 0 |
| 08:18–08:21 | 7 (acc-bvn3-val3 absent too) | 3 | 3 | 3 | 0 |

- **Which sends conflicted:** acc-bvn3-val1 sent Directory seq 598/599/600 (blocks 658/659/660) as roots `9c5794e5`/`f496ef04`/`4424a32d`, each **once** to each of the four destinations. The ten agreeing validators sent `99db2d11`/`e1a802cb`/`3b452065`. Grouped by (source, destination, seq), that is 12 conflicting sends. No validator contradicted itself. The first run's repeated sends (2,700 and 304 times) did not recur.
- **7 of 12 is below the Directory's anchor threshold.** `validatorAcceptThreshold` is 2/3, which is 8 of 12. The BVNs received Directory anchors as follows:
  - the last before acc-bvn2-val3's pause was block 1487, at 08:14:21;
  - the next was block 1506, at 08:16:22, after the pause ended at 08:16:19;
  - the last was block 1744, at 08:18:59, three seconds after acc-bvn3-val3 restarted, and none arrived after it.
- **BVN2 had the same margin problem.** Only val1, val3 and val4 were executing, so pausing val3 left 2 of 4 below 3. The Directory received no BVN2 anchor from block 1524 (08:14:38) to block 1525 (08:16:28). **The restarts that never rejoined used up the fault margin, and a later pause stopped anchoring.**
- **The end of the table:** BVN1 anchors stop at block 1850 (08:19:46) and BVN3 anchors at 1858 (08:19:51), while their blocks continue with 1–11 transactions each. This coincides with the load ending. I did not verify why those blocks carried no anchor.

## Count comparison against the first run

| line | first run | this run | where |
|---|---|---|---|
| `could not be pulled … Main not found` | 76,752 | **16** | All on acc-bvn3-val1's Directory re-sync (08:07:07–08:08:00), served by acc-bvn2-val2, which was itself joining: `entry 999 (9aff0591): the peer does not hold the message … Main not found` on `dn.acme/anchors/anchor-sequence`. None on any joiner's own pull. |
| `could not be pulled` (any) | — | 342 | All acc-bvn3-val1 after its divergence: 132 `cannot locate element N`, 119 `the local chain is at N and the peer served N; it cannot be re-pulled`, 77 `the local chain of N entries is not a prefix of the peer's`, 14 Main not found. |
| `does not hash into the anchored root` | 59+10+1 | **0** | |
| `Handoff failed` / `The handoff failed` | 2 | **0** | |
| `not its anchored root` | 1 | **1** | acc-bvn3-val1 Directory 658, 07:59:32. It is **24 blocks after** a successful handoff; the first run's was the first block. |
| stranded (accepted, neither certified, taken nor refused), final row | 3,979 on acc-bvn1-val1/BVN1 | **2,973 total**: 1,712 on acc-bvn1-val1/Directory and 1,232 on acc-bvn3-val1/Directory | `submissions.csv` rows `sample=final` at 08:21:00. Both nodes are Directory nodes whose gauge reads ACTIVE while they execute nothing. |
| reads lost to NotReady (load generator) | 52 | **0** | 44,231 NotReady answers and 872 transport errors were each retried at another endpoint. None failed at every endpoint. |
| healer NotReady answers filed as "still in flight" (#4387 class) | 204 | **286** | 257 `BVN1 is joining`, 22 BVN2, 7 BVN3; 266 of them 08:10–08:19. |
| `Dispatch queue past its block bound, dropping oldest` | 143 | 53 | In bursts at each pause: 07:53, 08:02, 08:08 and 08:14. |

## Delivery and heals (the #4409 leg)

Heals (validators' sum in `monitor.csv`) were 0 until 07:51:52 and ~580 by 08:06:29. After acc-bvn2-val2's restart they climbed to 10,858 by 08:18:58.

`Requested missing synthetics`, entries per minute:

- **BVN→BVN fell to healing exactly when a source validator did not rejoin its BVN.**
  - BVN2→BVN1 and BVN2→BVN3 went from 0 to 652 and 624 in the minute after acc-bvn2-val2's restart (08:07). They stayed at 100–650 a minute to 08:18.
  - Applied heals at 08:14:19: BVN2→BVN1 3,010 (val2 1,574 + val4 1,436) and BVN2→BVN3 3,020 (val3 1,447 + val4 1,573).
  - BVN1→BVN2 and BVN1→BVN3 started at 08:16–08:19, after acc-bvn1-val3's restart; it never rejoined BVN1.
  - After the two restarts whose BVN did rejoin (acc-bvn1-val1 BVN1 and acc-bvn3-val1 BVN3), BVN→BVN did **not** fall to healing: BVN1→BVN2/BVN3 stayed 0 until 08:16, and BVN3→BVN1/BVN2 totalled 24.
  - This is the dispatch share of a non-executing source validator (#4214/#4409), and it is unchanged from the first run.
- **BVN→Directory fell to healing when the Directory node in the sending container stopped executing, even where that container's BVN node was executing:**
  - BVN1→Directory: 22–76 a minute from 07:51, the minute acc-bvn1-val1 restarted. Its BVN1 was back at 07:51:11 and dispatching its share (`send=true` 121 times in 07:5x, the same as its peers). Its Directory never came back.
  - BVN3→Directory: from 08:00, right after acc-bvn3-val1's Directory diverged at 07:59:32.
  - BVN2→Directory: from 08:07.
  - The same two Directory nodes hold the 2,944 stranded submissions. My reading, **not verified**: a BVN node hands what it dispatches to the Directory to its own container's Directory node. That node reads ACTIVE, accepts it, never certifies it, and does not relay it, so the destination has to heal it. That is #4385's missing demotion, with a measured cost. See NEW-C.

## The load generator and the probes

- **Load generator:** 145,643 generated at 75.9 tps (first run 73.9). 0 reads lost. Every NotReady rotated to another endpoint, and none reached "no endpoint would answer". The #4387 fix on the load generator side holds.
- **Read-back probe:** 7,152 reads, 906 failed, 40 timed out at 8 s.
  - The timeouts fall in the rounds at the four pauses (07:55, 08:03, 08:09, 08:16).
  - The failures begin at 08:06 and climb to 125 per round by 08:20, with 0 refused by the query gate. My inference, **not verified**: round-robin reads are landing on Directory nodes that read ACTIVE and hold stale state (the probe logs no cause).
- **Follower:** 3,278 anchored blocks compared, 0 mismatches, never behind. It answered 1,799 of 1,799 reads.

## Does the harness's "rejoined" row agree?

Yes on the verdicts: `rejoined 2 of 10`, and the two are acc-bvn1-val1 BVN1 and acc-bvn3-val1 BVN3 (worst 16.4 s). Each "not established" row names the right reason:
- **The three Directory rows that read ACTIVE** (acc-bvn1-val1 at 10.0 s, acc-bvn2-val2 at 13.0 s, acc-bvn1-val3 at 83.4 s) are refused on executed height. That is correct, and it is the reason #4404 had to exist.
- **acc-bvn3-val1 Directory** is refused on the anchor disagreement at 658. It says `(and 14 more)`, where I count 3 blocks, or 12 sends by (destination, seq).
- **The BOOTING rows** say `stated no … anchor after its start`, which is true.

Where the harness is wrong:
1. **The stranded row says `FINAL ROW MISSING` and reports 1,413 as of 08:05:42.** The final rows did land (`sample=final`, 08:21:00), and they total 2,973.
   - Every joining node's Directory row is empty (it never created the counter).
   - The manifest counts those samples as incomplete and skips them: "26 samples skipped … INCLUDING THE LAST". So one joining node blanks the headline for the rest of the run.
   - `reading-a-run.md` says an empty field is a counter never created and should be skipped **as a field**, not as a sample.
2. **The wedge capture's `stalled 121s`** is the delivery criterion. No partition stopped closing blocks (BVN2 slowed to ~40 a minute during 08:14–08:16).
3. **`nodestate.csv` has no `final` row for acc-bvn3-val1/bvn3**, although it has one for its Directory.
4. **seizewatch** reported `SEIZED` at 07:52:20 on BVN1→Directory gap=61. That is the #4360 form, but here it coincides with real BVN1→Directory healing, which began at 07:51.

## What the run proves

- **The five first-run mechanisms are gone:**
  - **#4397/#4399:** 0 `does not hash`, and 0 Main-not-found on any joiner's own pull.
  - **#4401/#4402:** 0 handoff failures.
  - **NEW-1 of the first run:** no node executed at the wrong block numbers after a failed handoff.
  - **Repeated sends:** no anchor was re-sent in a loop.
  - **#4387 on the load generator side:** fixed.
- **A restarted validator that comes back a block or two behind rejoins its BVN in 9–13 s under 100 tps and stays in agreement.** acc-bvn1-val1 did so for 30 minutes and acc-bvn3-val1 for 22. After that, BVN→BVN delivery does not fall to healing.
- **A Directory handoff can succeed (acc-bvn3-val1, 7 s) and still diverge 24 blocks later on identical inputs.** Nothing caught it until the network's anchor for that block was verified, three blocks later.
- **A node reading BOOTING relays everything it accepts:** acc-bvn1-val3 3,807/3,807, acc-bvn2-val2 3,909/3,909, acc-bvn3-val3 99/99.
- **Joins that do not finish consume the fault margin.** With four validators out, a single pause stopped Directory and BVN2 anchoring for about 110 s. A fifth restart stopped Directory anchoring to the end.

## What it does not prove

- **Nothing past 34 minutes.** No node was restarted twice.
- **That a Directory partition can rejoin under load.** 0 of 5 are executing at the end.
- **That a BVN can rejoin when its node's state is more than a few blocks old, or when its first pass lands before its own state is matched.** 0 of 3 did (acc-bvn2-val2, acc-bvn1-val3, acc-bvn3-val3).
- **Anything about the last restart** beyond 124 s with no load.

## New, or not covered by the nine fixes

- **NEW-A: Directory divergence on identical inputs, 24 blocks after a good handoff.**
  - acc-bvn3-val1, block 658 at 07:59:29. Its accounting line `arrived=30 batches=11 block=658 … round=1358` is identical to acc-bvn3-val2's, and its anchor `seq=598 root=9c5794e5 bpt=5b49b5bd` differs from the peers' `99db2d11/75ea831d`.
  - At 07:59:32: `WARN An executed block's root is not its anchored root block=658 matched=657 module=join partition=acc://dn.acme`.
  - Context, not a cause:
    - its BVN3 had joined at 07:59:15/16;
    - at 07:59:23 that BVN3 executed 14 Directory anchors in one block (`Additional transaction succeeded block=674 … principal=acc://bvn-BVN3.acme/anchors type=directoryAnchor`);
    - its seed was `Seeded the synthetic cache from the chains blocks=12 from=622 … to=633`.
  - For a debugger. It is the #4398 class (same inputs, different root) on the Directory, and it is not at the first block.
- **NEW-2 (first run) is still present:** after that mismatch the re-sync cannot repair the node's own chains. From 07:59:32 to 08:20:54 there are 342 failures on `dn.acme/anchors` and `dn.acme/ledger`, for example `the local chain of 600 entries is not a prefix of the peer's` and `the local chain is at N and the peer served N; it cannot be re-pulled`. There are also 4 `Permanent layer refused a write` (07:59:36–08:00:00).
- **NEW-3 (first run, #4385) is still present, and it now costs more.**
  - On the Directory, the gauge goes ACTIVE at the tracker's first root match, including on a node that never hands off. acc-bvn1-val1 is ACTIVE from 07:51:12 and executed nothing past 184 for 30 minutes. acc-bvn2-val2 was ACTIVE 13 s after start, and acc-bvn1-val3 83.4 s after.
  - acc-bvn3-val1 was never demoted after its divergence.
  - The BVN gauges read BOOTING correctly.
  - The costs:
    - 2,944 stranded submissions on the two ACTIVE-but-stale Directory nodes;
    - BVN→Directory delivery falling to healing (NEW-C);
    - probably the stale pass root below (NEW-B).
- **NEW-B: Directory joins are stuck on passes whose root "the history has passed".**
  - 693 `The root a pass was served at did not prove; the pass is asked for again` read `served at block 661`: 382 on acc-bvn1-val1, 182 on acc-bvn2-val2, 117 on acc-bvn1-val3 and 12 on acc-bvn3-val3, from 07:59:38 to the end.
  - For example: acc-bvn1-val1 07:59:42 `the history has passed 19941882: it was served at block 661, the anchor of block 667 is verified, and the bpt chain does not record it`.
  - 661 is exactly acc-bvn3-val1's frozen Directory height, and that node reads ACTIVE. So, **inferred and not verified** (the line names no peer): it serves pulls from its divergent state.
  - Rotation (state.go:519 "the peers are asked in rotation") did not route around it. acc-bvn1-val1's Directory settled **no** pass after 07:59:32.
  - Also present: 42 `served at block 1549, the anchor of block 1744` on acc-bvn3-val3 at 08:19–08:21. No validator was at 1549 then. The candidate is a stale `captureProvableView`: 86 `A reader is holding an old database version`, for example acc-bvn1-val2 08:19:45 `age=42s current=1787 … version=1754`. Not verified.
- **NEW-C: BVN→Directory delivery lost through the sending container's own Directory node** (see Delivery). It is the same missing demotion, seen from the delivery side.
- **NEW-D: anchors served with no signatures.** acc-bvn3-val3, 08:19:17, 22 lines, BVN3 blocks 1388–1409: `An anchor was refused error={"code":"unauthenticated","codeID":401,"message":"the anchor carries no signatures"} block=1388 module=join partition=acc://bvn-BVN3.acme`. A peer holds BVN3 anchors for about 08:12–08:13 without their signatures. BVN3→Directory anchors were being healed in that window, so healed anchors are one candidate. Not verified.
- **NEW-E: the join never converges while the partition moves.** This is the mechanism behind 6 of the 8 failures; the other two are the Directory rows stuck on NEW-B.
  - **Directory, waiting on collection:** acc-bvn1-val1 matched its own state at 184 at 07:51:07, but `this node has collected only to round 366` for about 30 s. At restart there were 220 `Vote channel full, dropping message` and 13 `Consensus stalled - re-sharing certificates`.
  - **Directory, chasing gaps:** from 07:51:39 the sync chased `The next block has a gap` 34 times and never caught the moving partition. acc-bvn2-val2 did the same 149 times, and acc-bvn1-val3 52 times.
  - **BVN, never matching:** each pass keeps only the accounts at its majority root. acc-bvn2-val2's 67 BVN2 passes averaged 308 refetched against 201 kept, and its spine failed on every pass. The pulled state is never one a block had.
  - **What the two successful BVN joins had:** both matched their own pre-restart state before any pass wrote.
  - For a debugger. This is the next wall after #4397/#4399.
- **For the record:**
  - 26 `Permanent layer refused a write … shape=Message.(hash).Main` on **every** node at 07:49:55/58 and 07:51:49, at the same second. The first run had 37.
  - The join buffer reached 1,680 groups (acc-bvn1-val1 Directory) without an overrun, so #4407 was not exercised.
