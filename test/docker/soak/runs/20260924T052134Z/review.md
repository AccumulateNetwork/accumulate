# Review: run 20260924T052134Z (run-analyst)

Run: `issue-4205-lead` @ 2d969f9e2. This was the first Docker run of the second-pass join (E11, #4205). Plan: 30 minutes at 100 tps, 3 BVNs x 4 validators plus the follower `acc-bvn3-fol1`, with a chaos restart or pause every 120 s + jitter. Stallkill stopped it at 05:46:27Z after 0.39 h. Criterion (manifest header): every restarted validator's anchor body equals its peers' on both partitions, and the Directory reaches 12 of 12.

Sources: `node-logs-live.txt` (ANSI stripped), from which I extracted every `Block execution accounting` and `Sending an anchor` line (31,856 and 70,639 lines); `chaos.log`; `stallkill.log`; `nodestate.csv`; `submissions.csv`; `soak.log`; `monitor.csv`; and the probe dir `probe-20260924T054627Z/*.metrics.txt`.

## Verdict: per disturbance

Six disturbances happened (those below). Nothing else was scheduled inside the run's window. A pause restarts no process, so it gets no join verdict.

| time (Z) | disturbance | Directory partition | BVN partition |
|---|---|---|---|
| 05:25:51 | restart acc-bvn1-val1 | **REJOINED.** Joined at 207 (05:25:57, `executes=208`). The first block after the handoff, 208, ran at 05:27:40, **103 s later**; until then the DAG was stuck at round 414 (`Partition stalled … round=414`). Block 208: round 414, arrived 28, batches 12, the same as acc-bvn1-val3 and acc-bvn2-val3. Its anchors agreed with its peers from 207 to 1394, with no disagreement to the end. | **FAILED (#4398).** Joined at 203 (05:26:01). Block 204: round 406, arrived 32, batches 5, the same as its peers, but its root differs: anchor seq 156 root `fb7a2faf` against the peers' `3c2c702b`. It executed and anchored **204–213, all divergent** (10 conflicting anchors sent to the DN) before `An executed block's root differs from its proven root; syncing again` at 05:26:03. Then 123 re-sync passes to 05:46:54 and no match: #4397 ghost/void leaves (47,331 `could not be pulled` lines); #4399 (`/synthetic … does not hash into the anchored root`, 75 times); and its own divergent chains (below, NEW-2). Last block executed: 214. Gauge: **ACTIVE** throughout. |
| 05:28:12 | pause acc-bvn2-val1 135 s (to 05:30:27) | No verdict. Stood still at 335, back at 485 by 05:30. The Directory stayed 12/12. | No verdict. BVN2 went 341 → 465 by 05:30. BVN2 anchors stayed 4/4. |
| 05:33:09 | restart acc-bvn3-val1 | **FAILED** (a debugger is on it). Collected from 613 and joined at 698 at 05:34:57 (`alreadyInTheState=85 toProduce=22`). `Handoff failed; this node must join again … seed synthetic cache: load Directory receipts: load anchor pool main chain entry 2363: Message.c06fcb7d….Main not found`. **It never joined again**: 675 lines of `Failed to process committed group … determine last anchored block: load anchor 623: Message.c02d7d60….Main not found` through 05:47:00. No Directory block ran after the restart (executed_block gauge 613). It re-sent its join-block anchor (698, `b2f43e92`, which matches its peers) 2,700 times. Gauge: **ACTIVE** from 05:33:36, 81 s before the handoff. | **FAILED.** Never joined: `Joining: collecting committed blocks, executing none` 78 times, 05:33:23–05:46:58, over 72 pull passes. Blocked by #4397 (20,972 lines) and #4399 (13 `/synthetic` refusals). Last block executed: 634, from before the restart. Gauge: BOOTING, correctly. |
| 05:35:39 | pause acc-bvn1-val2 160 s (to 05:38:19) | No verdict. The Directory went from 762 at 05:35 to 935 by 05:38. | No verdict. BVN1 kept closing blocks (val3/val4 at 795 → 838 → 883, about 45 a minute) with **only val3 and val4 executing**. val1 was not executing, but its consensus still voted. That makes 3 of 4 in consensus, so quorum held. |
| 05:40:49 | restart acc-bvn2-val2 | **FAILED, and it sent a divergent anchor (NEW-1).** Collected from 1044 and joined at 1298 at 05:45:41 (`alreadyInTheState=254 toProduce=19`). `Handoff failed … produce buffered group 1 of 19 (round 2692): … load anchor pool main chain entry 4306: Message.54c4f268….Main not found`. **It then executed anyway**: leader rounds 2730 and 2732 as blocks **1299 and 1300**, where every peer has them as 1318 and 1319 (acc-bvn1-val3 has block 1300 at round 2694). It signed and sent a Directory anchor **seq 1154 = block 1300, root `f5b4979b`, bpt `d18b333d`** to all four partitions, 304 times. The 10 agreeing validators' seq 1154 is block 1301, root `de98b6c8`. After that, 76 lines of `load anchor 1153 … not found`. Gauge: **ACTIVE** from 05:41:19, 262 s before the handoff. | **FAILED.** Never joined: 34 `Joining … executing none` lines through 05:46:54, over 33 passes. #4397 (6,584 lines) and #4399 (9 refusals). Last block executed: 1046, from before the restart. Gauge: BOOTING. |
| 05:43:32 | pause acc-bvn3-val2 114 s (to 05:45:26) | No verdict. Back at 1334 by 05:45. | No verdict. BVN3 went 1246 → 1363 by 05:45. |

**Of the 6 partition rejoins the three restarts required, 1 succeeded**: acc-bvn1-val1's Directory. The criterion failed.

## Agreement tables (anchors, grouped by source partition and block)

Validators that sent the majority body, by the minute of first send. `>1` is the number of blocks with more than one distinct root/bpt among the senders. No validator contradicted itself on any block.

| minute (Z) | Directory N of 12 | BVN1 of 4 | BVN2 of 4 | BVN3 of 4 | >1 |
|---|---|---|---|---|---|
| 05:22–05:24 | 12 | 4 | 4 | 4 | 0 |
| 05:25 | 12 | 4, then 3 | 4 | 4 | **6 (BVN1 204–209, acc-bvn1-val1)** |
| 05:26 | 12 (val1's Directory resumes at 05:27:40) | 3 | 4 | 4 | **4 (BVN1 210–213, acc-bvn1-val1)** |
| 05:27–05:32 | 12 | 3 | 4 | 4 | 0 |
| 05:33–05:39 | 11 (acc-bvn3-val1 absent) | 3 | 4 | 3 | 0 |
| 05:40–05:44 | 10 (acc-bvn2-val2 absent too) | 3 | 3 | 3 | 0 |
| 05:45 | 10, plus **1 block anchored by acc-bvn2-val2 alone** (its "1300") | 3 | 3 | 3 | **1 (Directory seq 1154)** |
| 05:46–05:47 | 10 | 3 | 3 | 3 | 0 |

The pauses do not show up as missing senders, because a paused node's anchors go out on resume and agree. The two conflicts are acc-bvn1-val1's BVN1 204–213 and acc-bvn2-val2's Directory seq 1154. Grouping by block alone misses the second, because no peer anchored a block numbered 1300; only grouping by `(source, destination, seq)` finds it.

## What stalled BVN1, and the stop

Stallkill's `stalled 245s: BVN1` is soakmon's **delivery** criterion (`apply_delivery_stall`, soakmon.py:757). Inbound synthetic flows to BVN1, mostly BVN2→BVN1, had been red continuously since about 05:42:22 on the drain-time test (`pending/deliv_rate >= 4*expected`, soakmon.py:2496). The block height never stopped:

- **Block execution:** BVN1 executed through 1406/1407 on val2, val3 and val4 up to the last second. Each executed one block after another; only acc-bvn1-val1 was frozen, at 214.
- **Quorum:** no quorum loss. During the val2 pause the consensus still had val1's votes, plus val3 and val4.
- **Batch store:** no batch-store refusal. BVN1's val2–val4 logged no batch-queue, backpressure or refusal lines after 05:40.
- **What did happen:** BVN2→BVN1 delivery on acc-bvn1-val3 moved in heal-driven jumps: 11020 (05:42:22) → 11307 → 12072 → 13331 → 13798 → 14058 (05:46:18). Between the jumps it held a backlog of 800–1,500 entries; at the end it was waiting on 14059 with 784 held behind it. acc-bvn1-val3 applied **3,506 healed entries from BVN2 and 1,331 from BVN3**, where heals were 0 before the first restart (monitor.csv).

The loss that the healing had to cover has two sources in this run:

1. **acc-bvn1-val1 stranded what was sent to it.** Its BVN1 `certified` froze at 77 at 05:26 while `accepted` climbed to 4,086: 3,979 accepted and never certified, relayed or refused (submissions.csv). The two restarted nodes that read BOOTING relayed essentially everything they accepted: acc-bvn3-val1 3,903 of 3,904, acc-bvn2-val2 8,600 of 8,600. acc-bvn1-val1 reads ACTIVE, so it neither relays nor proposes batches (`Execution lags consensus: proposing headers without batches`). That is #4385's "no rollback" at a cost this run measured.
2. **A source validator that is not executing loses its share of dispatch.** Held entries on BVN2-sourced flows went up at every destination the minute acc-bvn2-val2 restarted: BVN3←BVN2 0 → 1,344, Directory←BVN2 0 → 238, BVN1←BVN2 460 → 1,527. BVN3-sourced flows rose after 05:33 in the same way. acc-bvn3-val3 applied 467 healed entries from BVN1 and 546 from BVN2. This is the #4214 class (dispatch share) and none of the four issues covers it.

**Why the run stopped:** acc-bvn1-val1 sat in a BVN1 re-sync that could not finish while reading ACTIVE, and it silently took and stranded the synthetics sent to it. Combined with acc-bvn2-val2's lost dispatch share, that left BVN2→BVN1 delivering only through healing, with a backlog stallkill judged undrainable for 245 s.

## Load generator and NotReady (#4387's class)

- **Reads lost:** `request failed: BVNx is joining and cannot answer`: **52 reads lost**. 31 were BVN3 (00:33–00:42 local, i.e. 05:33–05:42Z) and 21 were BVN2 (05:41–05:46Z). By operation: 29 `grow`, 20 `add-token`, 2 `add-account`, 1 `add-key-book`.
- **Never routed to another node:** `tools/cmd/loadgen/endpoints.go:64-83` (`poolQuerier`) advances to the next endpoint only when `isNetErr` is true; a NotReady "passes straight through". The operation is logged failed and dropped.
- **No BVN1 lines:** none of the 52 is for BVN1, although acc-bvn1-val1's BVN1 was not executing from 05:26. It reads ACTIVE, so it answers rather than refusing. I did not verify which state it answered from, but the only one it has is block 214 and diverged from 204.
- **The healing requester, same class:** it got **204** `… is joining and cannot answer for what it has not executed` answers (118 BVN3 source, 86 BVN2, 1 BVN1). Each was logged as `Missing synthetics/anchors are still in flight at the source`, i.e. filed as not-yet, which is #4387's defect. Heals were eventually applied through other validators, but that happened by rotation, not by a decision.

## What the harness misread

1. **"every start reached ACTIVE"** is in the Follower table, and there it is vacuous: no follower was started inside the run. The validator row says `NEVER ACTIVE: acc-bvn2-val2 bvn2, acc-bvn3-val1 bvn3`, which is true. Its "worst 29.4 s over 4 starts" lists acc-bvn1-val1 bvn1 (12.1 s), acc-bvn3-val1 directory (26.5 s) and acc-bvn2-val2 directory (29.4 s) as reaching ACTIVE. **All three failed.** What `nodestate.csv` shows against the logs:

   | node / partition | nodestate.csv | logs | executed_block at 05:46 (peers) |
   |---|---|---|---|
   | acc-bvn1-val1 / bvn1 | ACTIVE 05:26:03 | Joined 05:26:01, diverged 204, re-syncing to the end | **214** (1376) |
   | acc-bvn3-val1 / directory | ACTIVE 05:33:36 | `Joining … executing none` to 05:34:53; handoff failed 05:34:57; never executed | **613** (1364) |
   | acc-bvn2-val2 / directory | ACTIVE 05:41:19 | `Joining …` to 05:45:38; handoff failed 05:45:41; ran 2 blocks off-boundary | **1300** (1364, and its 1300 is not their 1300) |

   On the Directory the gauge went ACTIVE **81 s and 262 s before the handoff**, while the node was still logging `The next block has a gap; advancing the sync`. The promotion is the tracker's first root match, not the `Executing()` call before `Handoff`: `internal/node/join/state.go:857` → `tracker.Check` → `PromoteToActive` (tracker.go:194). The comment at state.go:866-870 says so: "The machine goes ACTIVE once, at the first match, and a join that found a gap after it pulls on". #4385 names only the second route. `accumulate_node_executed_block` (the right column) shows the truth and the harness does not use it for this row.
2. **`dn height` in monitor.csv is a single node, the restarted one.** It sat at 207 from 05:26:19 to 05:27:24 while every other Directory node went 215 → 323. That is acc-bvn1-val1's own Directory stall, shown as the network's.
3. **`stalled 245s: BVN1` is a delivery backlog, not a halt**, as described above. The criterion did its job (#4285), but the label says "stalled" of a partition that closed a block every second and delivered in jumps.
4. **A second follower that never existed.** The topology row, `run.json` (`followers: 2`, `nodes: 14`) and the readprobe all count `acc-bvn3-fol2`. It has no log lines, no container in `docker-ps.txt`, and 1,206 of 1,206 reads failed against it. The probe's "13 of 14 nodes" is the same artefact, and `chaos.log` has no `add-follower`.
5. **seizewatch** `SEIZED … BVN2->BVN1 gap=242` at 05:26:47 is the calibrated #4360 artefact in form. At that moment BVN1's held counts really had started rising (263 → 464), because of item 1 in the stall section.
6. **Stranded counter, one instrument question:** acc-bvn1-val1/BVN1 `accepted` and `rejected` both read 4,086 at 05:46:10. If a refused submission is also counted `accepted`, the 3,979 overstates the strand. The 4,837 healed entries into BVN1 say the loss is real, but the harness should confirm the two counters are disjoint.

## What the run proves

- A restarted validator's **Directory** partition can rejoin through the second-pass join under 100 tps and stay in agreement: acc-bvn1-val1 did, from block 208 to 1394, in about 20 minutes. It took 103 s to execute its first block after the handoff.
- The four known defects are live on Docker in the order the debuggers placed them. **#4398:** acc-bvn1-val1 BVN1 204, with the same boundary and a different root, 05:26:01. **#4397:** all three restarted nodes, from their first pass, on ghost*, ghostdata* and void-*/tokens. **#4399:** `/synthetic` refusals on all three. **The Directory handoff's missing anchor-pool message, with no retry:** acc-bvn3-val1 05:34:57 and acc-bvn2-val2 05:45:41, the same failure twice.
- A node reading BOOTING **relays** what it accepts (8,600/8,600 and 3,903/3,904).
- No BVN lost block production to any disturbance here, and none of the three pauses produced a disagreement.

## What it does not prove

- Anything past 25 minutes, or past 3 restarts and 3 pauses. No second restart of a node ever happened.
- That a BVN partition can rejoin at all under load: 0 of 3 did.
- That the Directory rejoin is reliable: 1 of 3 did.
- Anything about the follower beyond acc-bvn3-fol1 staying 0–1 blocks behind and never contradicting a validator (follower-report.md).

## New: not covered by #4397, #4398, #4399 or the Directory-handoff debugger

- **NEW-1: a failed Directory handoff goes on executing at the wrong block numbers and signs a conflicting anchor.**
  - acc-bvn2-val2, 05:45:41–05:45:44: `Handoff failed … buffered group 1 of 19 (round 2692)`, then `Partition resumed producing blocks`.
  - The accounting lines show `block=1299 round=2730` and `block=1300 round=2732`; peers have rounds 2730 and 2732 as 1318 and 1319.
  - Then `Sending an anchor block=1300 root=f5b4979b bpt=d18b333d seq=1154 source=Directory`, 304 times to every partition, against the other ten validators' seq 1154 = block 1301, root `de98b6c8`.
  - This is a Directory validator signing two different bodies under one sequence number. The log says "this node must join again" and "is not executing", and the node did neither.
  - It is related to the handoff failure the debugger is on, but it is a separate hazard: after a failed handoff, execution must stop, not skip the buffered groups.
- **NEW-2: after a mis-executed block, the re-sync cannot repair the node's own chains.**
  - acc-bvn1-val1, from 05:26:03: 124 passes each fail on `acc://bvn-BVN1.acme/ledger` (`block-ledger: the local chain of 189 entries is not a prefix of the peer's`) and on `acc://bvn-BVN1.acme/anchors` (`anchor(directory)-bpt-index: the local chain of 163 entries is not a prefix`).
  - The same window has 12 lines of `Permanent layer refused a write: this shape is not write-once` (bcdb, 05:26:06–05:27:46).
  - Fixing #4398 removes this trigger, but "syncing again" after any divergence is a dead end as built.
- **NEW-3 (adds to #4385): the tracker promotes to ACTIVE at the first root match, mid-join.**
  - The Directory gauge read ACTIVE for 81 s and 262 s before the handoff (state.go:857 / tracker.go:194), not only at `Executing()`.
  - There is also no demotion after either failure mode: the #4398 re-sync and the Directory handoff failure.
  - The measured cost of the missing demotion is 3,979 stranded submissions on acc-bvn1-val1 and the heal load that stopped the run.
- **Dispatch share of a non-executing source validator (the #4214 class) confirmed on this build:**
  - Held entries on BVN3-sourced flows went up after 05:33, and on BVN2-sourced flows after 05:40.
  - Healing covered them: 467 and 546 entries applied into BVN3, and 3,506 and 1,331 into BVN1.
- **Smaller, for the record:**
  - acc-bvn3-val1 re-sent one anchor 2,700 times in 12 minutes.
  - acc-bvn3-val1 logged `A named account is not this partition's and was dropped: acc://bvn-BVN2.acme/network` (8 times from 05:42:19).
  - The dispatcher logged `Dispatch queue past its block bound, dropping oldest` (143 lines) in bursts at each pause (05:28, 05:35–36, 05:43–44). By design, but it feeds the heal counts.
