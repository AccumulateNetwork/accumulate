# Review: run 20260924T093936Z (run-analyst)

Run: `issue-4205-lead` @ be4ddd0ca, the third Docker run of the second-pass join (E11, #4205). It adds #4385, #4415, #4413 and #4414 to the second run (`20260924T074702Z`, whose `review.md` is the baseline here). It does not carry #4411, #4412, #4416, #4418 or #4421.

- **Setup:** 30 minutes at 100 tps planned. 3 BVNs x 4 validators, plus the follower `acc-bvn3-fol1`.
- **How it ended:** stallkill stopped it at **10:06:16Z**, 26.7 minutes after load began, with `STOPPING: stalled 248s: BVN3`.
  - The capture ran until 10:08:24. The load generator was then killed (exit 143) and teardown started at 10:09:00.
  - The stall was a **delivery** stall: soakmon's `stalledBy=delivery`, with `blocksStalledFor=0`. BVN3 closed blocks throughout (1490 → 1618).
- **Disturbances that happened:** eight, all as scheduled. Four restarts and four pauses. The last pause (acc-bvn2-val3, 10:06:30 for 114 s) ended at the second of the teardown, so its recovery was never observed.

Sources:
- `node-logs-live.txt` (ANSI stripped): 34,632 `Block execution accounting` lines and 72,787 `Sending an anchor` lines.
- `nodestate.csv`, `submissions.csv`, `monitor.csv`, `chaos.log`, `stallkill.log`, `wedgewatch.log`, `loadgen-stats.json`, `readprobe*`, `streams-final.txt`.
- The two metrics scrapes: `delivery-stall-20260924T100245Z/` (10:02:45) and `probe-20260924T100616Z/` (10:06:16).

## Verdict: per restart and partition

A pause restarts no process and gets no verdict. All four paused nodes resumed executing in agreement; acc-bvn2-val3's recovery was not observed.

| restart | partition | joined | first block vs peers | agreed to the end | handoff failures | gauge (log / scrape / nodestate) | mechanism |
|---|---|---|---|---|---|---|---|
| acc-bvn1-val1 09:43:57 | **BVN1** | **Yes.** At 09:44:09: `Joined; executing from the block after the state block=201 executes=202`, 12 s after the restart. It restarted 4 blocks behind (200 vs 204) and matched its own state (`alreadyInTheState=0 toProduce=8`). | 202: round 402, arrived 38, batches 4, identical on val1–val4. | **Yes.** 1,340 accounting blocks (202–1606), the same set as val2 and val4, 0 disagreeing. 1,214 anchors (201–1590), 0 disagreeing. | 0 | `it is ACTIVE` at 09:44:09. Gauge 2 in both scrapes. nodestate `reached` at 15.7 s. Correct. | — |
| | **Directory** | **No.** Executed 206 for the whole run. | — | — | **63.** 09:44:42–09:48:40, blocks 241→463. Every one ends `seed synthetic cache: load Directory receipts: load anchor pool main chain entry N: Message.….Main not found`, with N from 762 to 1554. | Never ACTIVE. Gauge 0 in both scrapes. nodestate final BOOTING. Correct. | **#4421** (09:44:42–09:48:40), then **#4411**: from 09:48:17 `cannot hand off at block 379 (round 764), behind round 898 where this node's buffer starts`. After that it only pulled (4 accounts every ~6 s to 10:06:30), `lastBlock=463`, and the buffer grew to 1,081 groups. |
| acc-bvn3-val1 09:50:25 | **BVN3** | **No.** It restarted 5 behind (581 vs 586). 102 `Joining…` lines to 10:08:25. 80 `The spine did not verify; it is asked for again` (09:50:38–10:08:09). Its first pass: `again=139 kept=117`. Executed 582. | — | — | 0 | Never ACTIVE. Gauge 0. Final BOOTING. Correct. | #4411 |
| | **Directory** | **No.** 35 `The block after the state has not been collected yet` (09:50:31–09:51:49), then 32 `The next block has a gap; advancing the sync` (to 09:54:04), then pulls to 10:06:30. Executed 576. | — | — | 0 | Never ACTIVE. Gauge 0. Final BOOTING. Correct. | #4411 |
| acc-bvn2-val2 09:57:54 | **BVN2** | **No.** 59 `Joining…` and 47 spine failures. Executed 1001. | — | — | 0 | BOOTING. Correct. | #4411 |
| | **Directory** | **No.** 251 gap lines (09:58:02–10:08:26). Executed 998. | — | — | 0 | BOOTING. Correct. | #4411 |
| acc-bvn1-val3 10:03:43 | **BVN1** | **No, within the 2 min 33 s it had before stallkill.** 26 `Joining…` and 21 spine failures. Executed 1328. | — | — | 0 | BOOTING. Correct. | #4411 |
| | **Directory** | **No, within the same 2 min 33 s.** 118 gap lines. Executed 1328. | — | — | 0 | BOOTING. Correct. | #4411 |

**The harness row agrees:** `rejoined 1 of 8`, the one being acc-bvn1-val1 BVN1 at 15.7 s. The second run had 2 of 10.

**Against the manifest's expectation ("barely-behind BVN restarts rejoin"):**
- acc-bvn3-val1 restarted 5 blocks behind and did not rejoin BVN3.
- The only BVN rejoin is the one that matched its own state before any pull wrote, which is the same pattern the second run showed.

## Agreement tables

**Accounting.** 6,080 (partition, block) groups, each with the same (round, arrived, batches) on every validator that logged it. **0 disagreeing.**

**Anchors**, grouped by (`source`, block), follower excluded. 5,609 groups.
- **0 with more than one (root, bpt).**
- 0 blocks where a node contradicted itself.
- At most 1 send per (node, source, destination, block), so no re-send loops.

Validators that sent the (single) body, by the minute a block was first sent:

| minute (Z) | Directory of 12 | BVN1 of 4 | BVN2 of 4 | BVN3 of 4 | blocks with >1 body |
|---|---|---|---|---|---|
| 09:40–09:43 | 12 | 4 | 4 | 4 | 0 |
| 09:44–09:49 | 11 (acc-bvn1-val1 absent) | 4 | 4 | 4 | 0 |
| 09:50–09:56 | 10 (acc-bvn3-val1 absent too) | 4 | 4 | 3 | 0 |
| 09:57–10:02 | 9 (acc-bvn2-val2 absent too) | 4 | 3 | 3 | 0 |
| 10:03–10:05 | 8 (acc-bvn1-val3 absent too) | 3 | 3 | 3 | 0 |
| 10:06–10:08 | 7 (acc-bvn2-val3 paused) | 3 | 2 | 3 | 0 |

**7 of 12 is below the 8-of-12 anchor threshold, and 2 of 4 is below 3.**
- The last Directory anchor any BVN received was block 1493, at 10:06:32. For example, acc-bvn1-val2: `Received directory anchor … source-block=1493`.
- The last BVN2 anchor the Directory received was block 1505, at 10:06:31.
- The margin problem is the same as the second run's: restarts that never rejoin use up the fault margin. Here it came 16 s after stallkill had already fired.

## The six questions

**(1) Did any node read ACTIVE while not executing in agreement? No.**
- Every node that failed or kept re-syncing its join read BOOTING throughout, on both partitions:
  - the `accumulate_node_state` scrapes: 0 for every restarted pair except acc-bvn1-val1/bvn1, which read 2;
  - `nodestate.csv`: no `reached` row for any of them, and a final row of BOOTING;
  - the log: no `it is ACTIVE` line except acc-bvn1-val1 BVN1's.
- Every node that did read ACTIVE (the 16 never-restarted pairs and acc-bvn1-val1/bvn1) executed in agreement: 0 accounting disagreements and 0 anchor disagreements over the whole run.
- **What was not exercised:** the demotion transitions. No node diverged, so there was no root-mismatch re-sync; `This node is not executing in agreement; it is BOOTING` never appears. No node was ACTIVE when its handoff failed either. #4385 proved "not ACTIVE until a handoff succeeds". It did not prove "ACTIVE → BOOTING".

**(2) Stranded.**
- The final row (`sample=final`, 10:08:24, 26 rows) sums to **1**, on acc-bvn2-val1/BVN2. The second run had 2,973, of which 2,944 were on stale-ACTIVE Directory nodes.
- **Submissions to BOOTING nodes were relayed:**

  | node | relayedTaken / accepted |
  |---|---|
  | acc-bvn1-val1/Directory | 1,447 / 1,447 |
  | acc-bvn1-val1/BVN1 | 19, taken during its 12 s BOOTING window |
  | acc-bvn1-val3/BVN1 | 1,131 / 1,131 |
  | acc-bvn2-val2/BVN2 | 13,420 / 13,420 |
  | acc-bvn3-val1/BVN3 | 6,518 taken + 1 refused / 6,519 |
  | follower | 9,773 / 9,773 |

- The Directory rows of acc-bvn3-val1, acc-bvn2-val2 and acc-bvn1-val3 are empty: nothing was submitted to them. **The only BOOTING Directory node that received submissions is the one whose container's BVN node was ACTIVE and dispatching (acc-bvn1-val1).** That supports the second run's NEW-C reading: a BVN node hands its Directory-bound entries to its own container's Directory node. The cost of that path is now 0, where the second run lost 2,944.
- **Caveat:** stallkill ended the run, so the load generator was killed mid-flight and this is not a drained sample. Load had slowed to about 900 a minute during the capture (10:07–10:08).
- **Harness error:** the manifest's headline, `181 … FINAL ROW MISSING — soakmon's exit write did not land`, is wrong. The final row landed. It was skipped as incomplete because acc-bvn2-val3 was paused and answered no scrape, and the wording then claims a lost write.

**(3) Did BOOTING nodes serve pulls?**
- `served at block` lines: **0**, where the second run had 693. `The root a pass was served at did not prove`: 0. `not its anchored root`: 0.
- The pull client logs no peer, so I cannot name servers from the log. What the metrics show instead: the four BOOTING nodes **refused** pull-shaped queries (`accumulate_node_not_querying_total` at 10:06:16):

  | node | QueryAccountWithReceipt (Directory) | BptPageQuery (Directory) | ChainQuery (Directory) | ChainQuery (own BVN) | DefaultQuery (own BVN) |
  |---|---|---|---|---|---|
  | acc-bvn1-val1 | 278 | 30 | 351 | 4 | 46 |
  | acc-bvn3-val1 | 744 | 146 | 308 | 494 | 26,084 |
  | acc-bvn2-val2 | 620 | 156 | 203 | 356 | 16,527 |
  | acc-bvn1-val3 | 211 | 49 | 75 | 254 | 3,375 |

  No other node carries the series.
- The healers got 503 `… is joining and cannot answer for what it has not executed` answers (the second run had 286). These are BOOTING nodes refusing, as #4385 intends. They are still logged as `Missing … are still in flight at the source`, which is the #4387 filing.
- **Not verified:** that no non-querier path (anchorsrc, batch fetch) served from a BOOTING store. Nothing in the log says it did.

**(4) Healing: who asked, per partition, over time.**
- **The fixed pair is gone.** At every destination, three or four validators rotate minute by minute:
  - BVN1 destination: val1, val2, val3 and val4 all ask BVN2→BVN1 (464/539/307/1,249 entries).
  - BVN3: val2, val3 and val4 ask BVN2→BVN3 (969/514/1,449).
  - BVN2: val1–val4 ask BVN3→BVN2.
  - Directory: 9–10 validators ask per flow.
- BOOTING nodes asked for nothing after their restart. acc-bvn1-val1 asked only as an ACTIVE BVN1 node (59 requests).
- **Heals** (`monitor.csv`): 0 until 09:44:28, 42 at 09:45:33, 1,421 by 09:58:38, 3,562 by 10:00:49 and 8,346 at 10:06:16. The final figure is 8,405. The second run reached about 10,858 by 08:18:58.
- The healing follows each source that lost an executing validator: BVN3→* from 09:51, BVN2→* from 09:58 and BVN1→* from 10:04. That is the dispatch-share loss (#4214/#4409), unchanged.

**(5) Why the run stopped.**
- **The immediate cause is new.** BVN1→BVN3 synthetic delivery froze at **3857**, identically on all three executing BVN3 validators, from 10:01:16 to the end, with **no hole**. Each logged, for example (acc-bvn3-val3, 10:07:42):
  `Stream position advanced=0 block=1578 delivered=3857 held=1122 ledger=synthetic module=stream reach=4979 sighted=4979 source=BVN1 waiting=0`
- `waiting=0` means staging holds an entry for every number above Delivered in the scan (staging.go:1044). So no heal is ever requested for 3858, and none was: every BVN1→BVN3 request after 10:01:14 names later ranges.
- How it built up:
  - At 10:01:14 acc-bvn3-val3 requested 3821–3826 and 3871–3885. At 10:01:16 (block 1209) delivery jumped 3820 → 3857 and stopped.
  - soakmon's ledger read saw `recv 4909 / deliv 3857` at 10:06:15, and streams-final shows `5023 / 3857`.
  - Delivery was red from about 10:02:08, and stallkill fired 248 s later.
- **Which entries.** 3858–3860 came from BVN1 block 1138 (`Stream produced block=1138 count=3 destination=BVN3 from=3858`, 10:00:29). The only validator that sent that block was **acc-bvn1-val1**: `Dispatching synthetic transactions for block anchor-block=1155 block=1138 … send=true` at 10:00:36; val2, val3 and val4 have `send=false`. acc-bvn1-val1 is the rejoined node, and its Directory node was BOOTING at 206.
- The send also fell inside acc-bvn3-val2's pause (10:00:31–10:01:42).
- acc-bvn1-val1 sent its share normally all run (260 `send=true` against val2's 287), and BVN1→BVN3 delivered through its earlier blocks. **So it is not simply "the rejoined node's sends are bad".** Whether this is a consequence of the join (a synthetic whose proof comes from a container whose Directory never rejoined) is **not verified**. It needs a debugger.
- BVN1 block 1138's entries to BVN2 (3689–3692) and to the Directory (2047–2048) were delivered.
- **Behind that:** the join failures had already taken the Directory to 8 of 12 and BVN2 to 3 of 4. The pause at 10:06:30 then stopped Directory and BVN2 anchoring, 16 s after stallkill fired. Had stallkill not fired, that pause would have become the next stall, and that one *is* a join-failure consequence.

**(6) Counts against the second run.**

| line | second run | this run | where |
|---|---|---|---|
| `Main not found` | 16 | 126 (= 63 x 2 in each handoff-failure pair) | All acc-bvn1-val1 Directory, 09:44:42–09:48:40, `load anchor pool main chain entry N`. This is the #4421 shape on a *restarted* Directory node, not a fresh BVN. |
| `does not hash into the anchored root` | 0 | 4 | INFO `A pulled account did not verify`, refetched: `dn.acme/anchors` (acc-bvn1-val1 09:54:59; acc-bvn3-val1 10:04:25) and two `lg-…/tokens` (acc-bvn3-val1 09:56:06, 10:03:32). |
| `Handoff failed` / `The handoff failed` | 0 | 126 / 63 | Metric `accumulate_join_handoff_failures_total{partition="Directory"} 63` on acc-bvn1-val1 in both scrapes. |
| `not its anchored root` | 1 | **0** | |
| `carries no signatures` | 22 | **0** | #4413 |
| `served at block` (NEW-B) | 693 | **0** | |
| stranded, final row | 2,973 | **1** | |
| heals (monitor) | ~10,858 | 8,405 | The run was 3.5 min shorter. |
| loadgen reads lost | 0 (44,231 NotReady + 872 transport retried) | **0** (43,823 + 476 retried) | Generated 126,915 at 78.9 tps. |
| healer NotReady filed as in flight | 286 | 503 | |
| `Dispatch queue past its block bound` | 53 | 162 | 19 at 09:4x, 55 at 09:5x, 88 at 10:0x |
| `Permanent layer refused a write` | 26 | 26 | Two per node: 09:42:2x, then 09:47:18 (acc-bvn1-val1 at 09:44:32 and acc-bvn2-val1 at 09:48:19 instead). |
| read-back probe failed | 906 / 7,152 | 1,541 / 5,744 | See below. |

## What the run proves

- **#4385 holds for the promotion side.** No node read ACTIVE without a successful handoff, and no stale node served, stranded or signed. The two costs the second run measured are gone: 2,944 stranded becomes 0 on the joining Directory nodes, and 693 pulls at a stale root become 0.
- **BOOTING nodes relay every submission and refuse every read**, measured on four nodes.
- **#4413:** 0 anchors refused for no signatures.
- **#4415:** healing requesters rotate across 3–4 validators per destination, where the second run had the same pair all run.
- **No divergence anywhere in 26.7 minutes:** 0 accounting and 0 anchor disagreements. acc-bvn1-val1 BVN1 stayed in agreement for 24 minutes after its rejoin.

## What it does not prove

- **Demotion (ACTIVE → BOOTING).** Nothing diverged and no ACTIVE node failed a handoff.
- **That any Directory can rejoin under load:** 0 of 4.
- **That a BVN more than about 4 blocks behind can rejoin:** acc-bvn3-val1 at 5 behind did not.
- **Anything past 26.7 minutes.** No node was restarted twice. The last restart had 2.5 minutes. The last pause's recovery was not observed.
- **#4412 and #4403** were not exercised (no divergence).

## New

- **NEW-1: a stream stuck with no hole (the stop cause).** BVN1→BVN3 delivered=3857 on acc-bvn3-val2/3/4 from 10:01:16, `waiting=0`, `held` 69 → 1,122, and no heal requested. The evidence is in (5).
  - The candidates are an entry held for 3858 that staging will not release (a copy whose hash is not the validated one, or a proof waiting on an anchor), or a release rule that stops at a held entry. Not root-caused.
  - A debugger should start from the BVN3 staging state for (BVN1, synthetic, 3858) and from acc-bvn1-val1's dispatch of block 1138.
  - Paul's two-gap-kind rule has no third state for "held but not deliverable". That is where this lives.
- **NEW-2: #4421's failure on a restarted Directory node.** In the log, #4421 is filed for a fresh BVN node.
  - Verbatim, acc-bvn1-val1 at 09:44:42Z: `ERROR Handoff failed; the join syncs again error={… "produce buffered group 1 of 6 (round 486): produce block: begin block: seed synthetic cache: load Directory receipts: load anchor pool main chain entry 762: Message.6e13222e….Main not found"} block=241 module=dagbft partition=Directory`.
  - It repeated 63 times over 4 minutes, advancing with the pull, until the buffer passed the pulled state (#4411).
  - The first run had this same failure without the retry (acc-bvn3-val1 05:34:57, acc-bvn2-val2 05:45:41).
- **NEW-3: 141 submissions refused with `key is not an active validator for Directory` (112) / `for BVN3` (29).** They appear as `TRACE-SUBMIT: validation failed`, on 9 validators: 1 at 09:55, 1 at 10:00, 23 at 10:01, 70 at 10:07 and 46 at 10:08.
  - The message names the *source* partition of a synthetic or anchor whose signer is not active there (msg_synthetic.go:215, msg_block_anchor.go:285).
  - The only key in the network that is inactive on Directory and BVN3 is the follower's.
  - My reading, **not verified**: something signed by acc-bvn3-fol1 is being submitted, most likely a heal or anchor answer, and 10:07–10:08 is the anchor-heal surge. The follower logged no `send=true`. The line names no key.
- **Harness:**
  - (a) The stranded headline says `FINAL ROW MISSING` when the final row landed; it was skipped because the paused acc-bvn2-val3 was blank.
  - (b) The validator read-probe counts NotReady as `failed` (readprobe.py:296). The follower row separates it. The 1,541 failures begin at the first restart and jump when acc-bvn3-val1 joins, which is consistent with reads landing on BOOTING nodes. Not verified: the probe logs no cause.
  - (c) The manifest has stallkill `stopped (UTC) 10:08:24Z`, but the decision was 10:06:16Z. The two minutes between were the capture.
- **For the record:**
  - 722 `Vote channel full` at restarts.
  - 48 `Consensus stalled`.
  - 52 `A reader is holding an old database version`, 48 of them at 10:0x.
  - acc-bvn1-val1 and acc-bvn3-val1's Directory joins logged 263 and 108 `A named account is not this partition's and was dropped account=acc://bvn-BVN2.acme/network` (also seen in the first run).
