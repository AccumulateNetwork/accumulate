# Review: restart, rejoin and catch-up, 2026-10-03

Paul asked for this on 2026-10-03: "Do a full review. Do we have a plan to fix this?" It covers what the two restart-test runs of 2026-10-03 established, what is fixed, what is not, and the plan.

The data comes from these sources:
- **Docker runs.** `test/docker/soak/runs/20261003T182048Z` on lead `84f951d33`, and `20261003T191744Z` on lead `949ac13b5`. Each has a `SUMMARY.md`, a `chaos.log` and a `restart-test.md`.
- **The consim reproduction.** `TestRestartOneDirectoryValidatorOfTwelve`, on branch `issue-4515-three-fixes`.
- **The code read for the review.** `pkg/consensus/primary`, `pkg/consensus/restart.go`, `pkg/consensus/dag`, `internal/node/join` and `pkg/database/keyvalue/bcdb`.

## 1. What the restart test now shows

| disturbance | first run (`84f951d33`) | re-run (`949ac13b5`) | state |
|---|---|---|---|
| graceful whole-network restart | passed | passed | **works**, twice |
| kill whole-network restart | partitions passed, but bvn1-val3's Directory was lost | passed, every node back within 33 s | **fixed** (`e01819de4`); see 2.1 for what is not proven |
| one validator's BVN store torn | the tear was found and repaired by a join | the same | **works**, twice |
| a restored validator's **Directory** after a short stop | rejoined in 5 s | wedged for 15 minutes | **open: #4515** |
| two of BVN2's four validators lost, then restored | held at 0 blocks, said why, resumed in 11 s | the same | **works**, twice |
| a node that resumed from its own block says it is synced | it never did | did so 100 blocks after its root was proved | **fixed** (`8baa8e7eb`), confirmed on Docker |

## 2. Fixed today

### 2.1 The kill left the store adapter one block short (`e01819de4`)

- **Run 20261003T182048Z.** After `docker rm -f`, bvn1-val3's Directory store had `block.json` at 866 while every permanent shard manifest said 865. The adapter took its version from the manifest, and the store then refused every seal.
- **The fix.** The adapter now takes the higher of the two. A join whose pull failed on that refusal also returned for good. A failed pull is now logged, counted and retried.
- **Tests.** Both fixes have tests that fail without them.
- **On Docker.** The re-run's kill step was clean. That shows a kill no longer loses a node. It does not show the re-run's kill produced the record-ahead-of-manifest state at all, since the adapter logs nothing when it applies.
- **Store-side cause.** The store does not write `block.json` and its shard manifests as one unit (BlockchainDB#99). No blocks were lost. An earlier claim that the dynamic layer lost about 140 blocks compared the ledger height with the store's own block counter, and is withdrawn. E13 (durability at the committed log) is still not built.

### 2.2 A node that resumed from its own block never said it was synced (`8baa8e7eb`)

Only a join's overlap started the 100-block synced hold, and a node that pulled nothing has no overlap. The hold now starts at the first block whose root a signed anchor proves. It was confirmed on Docker in the re-run: acc-bvn3-fol1 was synced at 19:28:37Z, about 100 seconds after its root was proved.

## 3. Open: #4515, a restored Directory validator wedges in catch-up

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

**Not yet shown on Docker.** That is plan item 1.

**A test flaw, found and fixed on the way.** One run failed with the restarted node at height 369 of 400, still catching up. The simulation stopped when the furthest node of each partition reached 400. consim's new `TargetEveryNode` waits for every node, bounded by the run's duration, so only a node that stops short fails.

## 4. Other findings of the day

- **A kill-dwell miscount, harness.** The first run's kill dwell reported 1,000 blocks in 5 minutes. `monitor.csv` shows about 1 block per second throughout. The driver read its starting heights from a dashboard that had not yet re-read the nodes. The results are unaffected, but the driver's dwell needs a fresh read before it starts counting.
- **#4516, a log line.** An ACTIVE node logs "The join is waiting" every minute, because the anchor collector's asks after the handoff set the join's waiting step again.
- **#4511 is probably #4515.** It is a Directory that froze for three hours in run 20261002T145702Z, after two restored validators stopped voting. That run's live log was deleted, so it cannot be confirmed. It should be closed once item 1 passes, with that caveat.
- **A burst on restart.** A restored Directory node logged 197 "Vote channel full, dropping message" in its first second. The primary's one event loop takes headers, votes and gossiped certificates, while the frontier floods in. With the release fix nothing is lost for good, because votes are rebroadcast. But the burst is real, and it is what Paul's serialized boot (item 2) would remove.

## 5. The plan

1. **Prove #4515's fix on Docker.**
   - Re-run `restart-test.py` on `1b2e27224`. Step C is the case that failed.
   - Step C alone restarts one validator once, and on unchanged code the wedge struck about 1 restart in 10. So also add a step that restarts single validators one after another, about 10 times across the Directory's twelve, and requires every Directory node ACTIVE after each.
   - Done when every step passes and every single-validator restart is ACTIVE within its bound.
2. **A serialized boot: a decision for Paul.**
   - **The proposal.** Paul, 2026-10-03: "What I think we need to do on boot is to slow down the whole process where we focus on serializing steps that in a fully running protocol does not require serialization." A restored or rejoining node would be in catch-up mode until its DAG reaches the frontier. In that mode it would author nothing, vote on nothing and hold no frontier certificate. One goroutine would ask for rounds from where its DAG ends and insert them in order. The node would switch to the live protocol within a round or two of the frontier.
   - **What the evidence says.** #4515's cause was the missed release. Releases were missed on live paths too, the node's own certificate and duplicates, so the fix had to be in the live protocol either way. A serialized boot would not have fixed it alone.
   - **What it would buy.** No restart burst, and no buffer of frontier certificates the node cannot prune. Its order would also be easier to reason about.
   - **What it would cost.** A second mode in the primary, and a slower boot.
   - **My recommendation.** Spec it now, and build it if item 1 shows restart catch-up is still slow or bursty on Docker. Do not build it ahead of that evidence. This is Paul's call.
3. **Make the harness dwell count correctly.** It should re-read heights after the restart settles, before it starts counting. This is small and goes with item 1.
4. **Earlier open items, unchanged.** #4514, #4504, BlockchainDB#99 with E13, and #4516.
5. **The acceptance run.** A 12-hour soak with single-validator restarts and kills as chaos. No claim about restart behaviour stands on runs shorter than that.
