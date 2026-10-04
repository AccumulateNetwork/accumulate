# Review: restart, rejoin and catch-up, 2026-10-03 to 2026-10-04

Paul asked for this on 2026-10-03: "Do a full review. Do we have a plan to fix this?" It was brought up to date on 2026-10-04 ("document what you are doing"). It covers what four restart-test runs established, what was fixed, how each fix was proven, and what is next.

**Where it stands, 2026-10-04.** Every disturbance the restart test makes recovers on Docker with every cross-partition flow delivering (run `20261004T055059Z`, lead `a38a66290`). The disturbances are: a graceful whole-network restart, a kill, a torn store, the loss and restore of a quorum, and twelve single-validator restarts. #4515 and #4517 are fixed and closed. No claim about long-run behaviour is made: that is the 12-hour acceptance soak, which is next. The branch is `issue-4205-lead` at `903b0745a` and later.

The data comes from these sources:
- **Docker runs.** Four runs, each with a `SUMMARY.md`, a `chaos.log` and a `restart-test.md`:
  - `test/docker/soak/runs/20261003T182048Z` on lead `84f951d33`;
  - `20261003T191744Z` on `949ac13b5`;
  - `20261003T232435Z` on `bb77f9222`;
  - `20261004T055059Z` on `a38a66290`.
- **The consim reproduction.** `TestRestartOneDirectoryValidatorOfTwelve`, on branch `issue-4515-three-fixes`.
- **The code read for the review.** `pkg/consensus/primary`, `pkg/consensus/restart.go`, `pkg/consensus/dag`, `internal/node/join` and `pkg/database/keyvalue/bcdb`.

## 1. What the restart test now shows

| disturbance | `182048Z` | `191744Z` | `232435Z` | `055059Z` | state |
|---|---|---|---|---|---|
| graceful whole-network restart | passed | passed | passed, 23 s | passed, 18 s | **works** |
| kill whole-network restart | bvn1-val3's Directory lost | passed | passed, 17 s | passed, 18 s | **fixed**, `e01819de4` (2.1) |
| one validator's BVN store torn | repaired by a join | repaired, but its Directory wedged | passed, 42 s | passed, 27 s | **works**; the wedge was #4515 (3) |
| two of BVN2's four lost, then restored | held, said why, resumed | the same | the same | the same, every flow delivering | **works** |
| twelve single restarts (step E, new in `232435Z`) | — | — | all back, but delivery stopped for good | all back in 20–31 s, every flow delivering | **fixed**: #4515 (3) and #4517 (4) |
| a node resumed from its own block says it is synced | never did | did | did | did | **fixed**, `8baa8e7eb` (2.2) |

Times are seconds from the restart to every node ACTIVE, by fresh dashboard rows (5.1).

## 2. Fixed on 2026-10-03

### 2.1 The kill left the store adapter one block short (`e01819de4`)

- **Run 20261003T182048Z.** After `docker rm -f`, bvn1-val3's Directory store had `block.json` at 866 while every permanent shard manifest said 865. The adapter took its version from the manifest, and the store then refused every seal.
- **The fix.** The adapter now takes the higher of the two. A join whose pull failed on that refusal also returned for good. A failed pull is now logged, counted and retried.
- **Tests.** Both fixes have tests that fail without them.
- **On Docker.** The re-run's kill step was clean. That shows a kill no longer loses a node. It does not show the re-run's kill produced the record-ahead-of-manifest state at all, since the adapter logs nothing when it applies.
- **Store-side cause.** The store does not write `block.json` and its shard manifests as one unit (BlockchainDB#99). No blocks were lost. An earlier claim that the dynamic layer lost about 140 blocks compared the ledger height with the store's own block counter, and is withdrawn. E13 (durability at the committed log) is still not built.

### 2.2 A node that resumed from its own block never said it was synced (`8baa8e7eb`)

Only a join's overlap started the 100-block synced hold, and a node that pulled nothing has no overlap. The hold now starts at the first block whose root a signed anchor proves. It was confirmed on Docker in the re-run: acc-bvn3-fol1 was synced at 19:28:37Z, about 100 seconds after its root was proved.

## 3. Fixed: #4515, a restored Directory validator wedged in catch-up

### 3.1 What happens

The re-run's step C stopped bvn2-val4 for about 6 s.
- **The restore.** Its Directory restored round 2098, then advanced its round to 2107 as certificates arrived.
- **The wedge.** For eleven minutes, every certificate it received was already in its pending buffer: `buffered` 97,136, `inserted` 1, `verified=0`.
- **Its peers were fine.** They answered every request and never served empty, while the Directory moved past round 3,370.
- **What it cost.** The join proved the state every few seconds and could not hand off, because consensus delivered nothing past round 2104. A wedged validator does not vote either.

**#4511 is probably the same defect.** In run 20261002T145702Z, two restored Directory validators were alive and did not vote. That took the Directory below its quorum and the network froze for three hours. So #4515 is not a corner case. One restarted Directory validator at the wrong moment can halt the network.

**Reproduced in-process.** `TestRestartOneDirectoryValidatorOfTwelve` uses 12 Directory validators at 20 tps and restarts one after a 2 s stop. On unchanged code it wedged in 2 of 13 runs, and the failures look like the Docker one.

### 3.2 What the code does on a restart

- **The restore.** It loads every persisted certificate into the DAG without parent checks: the checkpoint's tail, plus the DAG from four rounds below each authored header. The last authored header goes back to the primary. The primary then starts at once, in its normal live mode: it authors, votes, takes gossip, and asks for missing rounds when a header arrives from more than one round ahead.
- **Three goroutines insert into the DAG concurrently.**
  - The primary's event loop inserts gossiped certificates.
  - The sync-response goroutine inserts catch-up answers.
  - A separate goroutine inserts each certificate the node itself forms.
- **A held certificate waits on its parents until a release reaches it**, and three things defeat the release:
  - **No release for own certificates.** The own-certificate goroutine never releases the certificates waiting on that certificate.
  - **No release on a duplicate.** A copy of a certificate already in the DAG counts as a duplicate and releases nothing.
  - **A race.** A child whose insert failed computes its missing parents and is then added to the buffer. If another goroutine inserts the parent between those two steps, the parent's release runs before the child is registered, and nothing releases the child again.
- **The buffer fills with frontier certificates.** It holds `2 × 10 × committee` entries, 240 for the Directory. Gossip delivers the live frontier from the first second. Frontier certificates are above the node's round, so they are never pruned. Once the buffer is full, a certificate the node needs next is dropped, and only its parents are asked for.
- **Catch-up is slow.** A round request is answered with at most 100 certificates, about 8 Directory rounds. Each request is answered by one peer, while the others see the answer and stay silent. Round catch-up runs at most once a second, and stall recovery at most every 2 s.

### 3.3 What was tried, on branch `issue-4515-three-fixes`

| change | result |
|---|---|
| catch-up starts at the DAG's quorum frontier | **withdrawn.** The diagnostic behind it read the DAG's rounds unsorted. The round advances only on a quorum at the current round, so catch-up already started there. |
| a full buffer evicts its highest round for a lower one | 5 failures in 20 together with the above, worse than unchanged code |
| held certificates whose parents are in the DAG are released, on the duplicate path, after each Add and on every round tick; own certificates release their waiters | **the fix**, 3.5 |

### 3.4 The cause: a release that never comes

A diagnostic added to the test (`Primary.PendingDiagnosis`) caught a wedge on unchanged lead code:

```
node Directory/7: round 114, DAG top 113, quorum frontier 113;
pending 240, of which 1 wait only on parents already in the DAG; 0 parents lacked
```

- **Nothing was missing.** The node lacked no certificate that a peer would have to send.
- **One certificate was never released.** It had every parent in the DAG and was still held, because the release missed it.
- **Everything else waited on it.** The other 239 waited on parents that were themselves held.
- **Copies did not help.** Each copy that arrived was reported held without a retry. This is Docker's `verified=0`.

`TestOwnCertificateReleasesTheCertificatesHeldForIt` shows one missed path deterministically. A peer's certificate held for this node's own certificate stays held after the own certificate is inserted.

### 3.5 The fix (`1b2e27224`)

A held certificate is released whenever its parents are in the DAG, whoever inserted them.
- **Own certificates.** Inserting one releases what waits for it.
- **Duplicates.** A copy of a certificate already in the DAG releases what waits for it.
- **The race.** A certificate just held releases itself if one of the parents it was held for has meanwhile been inserted.
- **The backstop.** Once a second, every held certificate whose parents are in the DAG is released.

The rule is in the spec, consensus.md, "Certificate catch-up".

| | wedges |
|---|---|
| unchanged code | 3 in 29 runs |
| with the fix | 0 in 45 runs. If the fix did nothing, a streak that long has under a 1% chance. |

Also run with the fix:
- `-race` on the primary package: passes.
- The consim restart and follower tests: pass.
- The other consensus packages: pass.

**On Docker.** Runs `20261003T232435Z` and `20261004T055059Z` restarted single Directory nodes 29 times, in steps C, D and E, with no wedge. Each was back in under 91 s. #4515 is closed. #4511 and #4504 are closed as probably the same defect. Their own evidence could not confirm it.

**A test flaw, found and fixed on the way.** One run failed with the restarted node at height 369 of 400, still catching up. The simulation stopped when the furthest node of each partition reached 400. consim's new `TargetEveryNode` waits for every node, bounded by the run's duration, so only a node that stops short fails.

## 4. Fixed: #4517, single restarts stopped cross-partition delivery for good

### 4.1 What happened

The run `20261003T232435Z` added step E, which restarts each of the twelve validators alone, alternating graceful and kill. Every node came back ACTIVE and every partition kept closing blocks, so the driver passed. But cross-partition delivery stopped at about 00:04Z and never recovered. The worst stall grew to 1,123 s, and seven synthetic flows were frozen at teardown. BVN2's Directory anchor stream stood at 1,980 from 00:07:59Z, right after bvn2-val1 restarted and bvn2-val2 was killed.

### 4.2 The chain, as Paul's questions took it apart

| Paul asked | what the logs and code showed |
|---|---|
| "How can healing fail to fix synthetic flows?" | Healing ran. BVN2 answered "the directory has not receipted the block yet" to 227 of 260 asks. A source may serve a synthetic only once it holds the Directory's receipt, which arrives in a Directory anchor. |
| (cache) "Just because the cache is wiped out doesn't mean the cache cannot be rebuilt." | Right. Everything a source produced is in its chains, but a source answered anchors only from memory. That became R9-2. |
| "One validated anchor, the last one, would validate all the past anchors. Why didn't any anchor show up?" | Every heal toward the Directory was skipped as `lagging`: 86, 52 and 22 on three nodes, none sent. The guard's premise, that what a lagging node lacks is in its own backlog, is false when the backlog waits on that stream. It was a deadlock. That became R9-1. |
| "A later anchor WILL show up with signatures. Why didn't that happen?" | All 12 Directory validators sent their copies. But a lagging primary proposed headers with no batches at all, so copies that landed on lagging BVN2 nodes were never ordered. No anchor after 1,980 reached 8 distinct executed signers. A second deadlock. That became R9-3. |
| "Are the signatures collected at the DN, or submitted as their own txs?" | Their own transactions. Each validator signs and submits its own copy, and the destination counts distinct signers among executed copies. Since Vandenberg, a signature over the Directory's own copy is also valid on a BVN's copy, so one Directory answer carries the whole quorum. |
| "Isn't staging for each account independent?" | Staging is. Three things outside it couple streams: the join's hand-off waits on every stream's gap, and both the lag rule and the healing guard act on the whole partition. Executing a Directory anchor is also what proves inbound synthetics and receipts outbound ones, so one stuck anchor stream stops every flow at that BVN. |
| "Are these signatures in the anchor chain? … an account state the blockledger updates?" | Not in the anchor chain. Each distinct signer is on the anchor pool's signature chain, which the block ledger names and the root chain anchors. The quorum set itself is a record committed only while pending. |

### 4.3 The fixes (PLAN R9; spec `9f29304fb`)

| item | change | proof |
|---|---|---|
| R9-1 `34fe6205f` | Anchor streams are asked for whatever the lag. The guard keeps skipping synthetic streams. | `TestRequester_LaggingNodeDoesNotAsk` fails without it |
| R9-2 `1fbaa6212` | An anchor the cache lacks is answered from the store: the `anchor-sequence` chain, the stored message and the held quorum, through a deep view. | `TestSequencer_AnchorNotInTheCacheIsAnsweredFromTheStore` fails without it |
| R9-3 `1ef91b5f6` | A batch sealed with a `SubmitUser` transaction is marked user. While lagging, a header takes system batches only. Invariant 9 is aligned. | `TestCreateHeader_CarriesNoBatchesWhileExecutionLags` fails on the old builder |
| R9-4 `a38a66290` | In-process reproduction: a lagging BVN with Directory anchor 3 below quorum and its copies dropped. | Stuck for 400 steps before R9-1, recovers 5 of 5 after |

**A side effect, `9b8bc734b`.** Two consim overload tests asserted the old pathology, a backlog dumped as one block that doubled until the partition stopped. With R9-3 that no longer forms, even without the header cap: the largest block is about 290 transactions and the partition keeps producing. Lag under that overload still drifts past the bound, which is cross-partition back-pressure, DIFFERENCES C7, unchanged.

**On Docker, run `20261004T055059Z`.** Steps A to E all recovered with every flow delivering. During the twelve single restarts, delivery hiccups cleared within a minute. Which of the three fixes did the work on Docker is not separated. #4517 is closed.

## 5. Other findings, and what became of them

### 5.1 The test driver (`restart-test.py`)

Each of these was found because a run passed or failed for the wrong reason.

| flaw | fix |
|---|---|
| A dwell read its starting heights from a dashboard not yet re-read, and reported 1,000 blocks where `monitor.csv` showed 315. | `bb77f9222`: a dwell starts from live heights. |
| "Every node ACTIVE" passed 2–3 s after a whole-network restart, on stale rows. | `bb77f9222`: a restarted node's row must come from its new container. |
| It passed "0 s after the restart" on containers whose rows were missing. | `2480f4099`: a missing row is not ACTIVE. |
| It passed run `232435Z` while delivery was stalled for twenty minutes. | `492184cac`: a dwell fails on a delivery stall. |
| That check then failed step D on the flows out of the deliberately stopped BVN2. | `753435139`: a stall sourced only from a partition meant to stand is excused. |

### 5.2 Corrections made to the record

- **The kill lost no blocks.** The "about 140 lost blocks" compared a ledger height with BlockchainDB's own block counter. Corrected in run `182048Z`'s summary and on BlockchainDB#99.
- **#4515's first explanation was wrong.** "Catch-up starts too high" rested on an unsorted round list. It was withdrawn in run `191744Z`'s summary and on #4515.
- **Anchor copies were ordered.** A note on #4517 said they were never ordered. In fact copies were ordered, just never 8 signers for one anchor.

### 5.3 Still open

- **#4516.** An ACTIVE node logs "The join is waiting" every minute, because the anchor collector's asks after the handoff re-arm the join's waiting step.
- **BlockchainDB#99 and E13.** The store's block record and manifests are not written as one unit, and durability at the committed log is not built.
- **#4514.** From the earlier plan, unchanged.
- **The restart burst.** About 100 to 200 dropped votes per restart, harmless so far. It is what the serialized boot, DIFFERENCES C15, would remove. That boot is deferred, since Docker no longer shows restart catch-up stuck or slow.

## 6. Next

1. **The acceptance run.** A 12-hour soak with single-validator restarts and kills as chaos, the dashboard shown before load. No claim about restart behaviour stands on runs shorter than that.
2. **Separating the three fixes on Docker,** if the soak raises a question that needs it.
3. **Cross-partition back-pressure (C7)** stays the known gap under sustained overload.
