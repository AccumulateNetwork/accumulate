# Restart and quorum-loss test, re-run: run 20261003T191744Z

The run used lead `949ac13b5` and `restart-test.py` at 1 tx/s. It re-ran run 20261003T182048Z with that run's three fixes. The step record is in `chaos.log` and the table is in `restart-test.md`. The run went from 19:17Z to 20:03Z on 2026-10-03.

## Result

Steps A, B and D passed. Step C failed on a new defect, #4515, which has since been reproduced in-process. The previous run's three fixes were checked on Docker.

| step | driver | what happened |
|---|---|---|
| A: graceful whole-network restart | PASSED | All nodes were ACTIVE within 3 s, and the partitions made 286 to 291 blocks in 5 min. |
| B: kill whole-network restart | PASSED | All nodes were ACTIVE within 33 s, and the partitions made 294 to 297 blocks in 5 min. No store refused a seal, no pull failed and no join gave up. This was the step that lost bvn1-val3's Directory last run. |
| C: one validator's store torn | FAILED | bvn2-val4's BVN2 found the tear, rejoined and was ACTIVE. Its **Directory** wedged in certificate catch-up and never rejoined (#4515). |
| D: two of BVN2's four lost, then restored | PASSED | BVN2 made 0 blocks in 5 min and said "lost its quorum" while the others ran on. After the restore all four partitions made 281 to 292 blocks in 5 min. |

## The previous run's fixes, checked on Docker

- **`8baa8e7eb`: a resumed node says it is synced.** acc-bvn3-fol1 resumed from its own block after the graceful restart. A signed anchor proved its root at 19:27:00Z. It logged "The node is synced: its own root, proved by its signed anchor, held with no regression" at 19:28:37Z for the Directory and 19:28:42Z for BVN3. The dashboard showed it synced.
- **`e01819de4`: the adapter resumes at `block.json`, and a failed pull is retried.** Step B ran clean. That shows the kill path works. It does not show that this kill produced the `block.json`-ahead-of-manifest state the fix handles, because the fix logs nothing when it applies.

## #4515: a Directory validator restarted alone wedges in catch-up

**What happened on Docker.**
- **The restart.** bvn2-val4 was stopped for about 6 s. Its Directory restored round 2098, the last header it had authored, with its DAG holding certificates to round 2104.
- **The wedge.** Catch-up asked for rounds 2106 to 2169, which is the current round minus one onward, for 11 minutes. Every certificate that came back was already in the pending buffer: `buffered` 97,136, `inserted` 1, `verified=0`. The join proved the state again and again but could not hand off, because consensus "collected only to round 2104".
- **Evidence.** `wedge-bvn2-val4-directory/` holds goroutines, metrics and the key log lines.

**Cause, found in-process.**
- **The reproduction.** `TestRestartOneDirectoryValidatorOfTwelve` in pkg/consensus/consim has 12 Directory validators and restarts one alone after a 2 s stop. It wedged in 2 of 13 runs.
- **What the diagnostic showed.** On a failure, the node's current round was 93, its DAG's top was round 59 and its 240-slot pending buffer was full. It lacked 49 parents at rounds 96 to 107, all of which a peer held.
- **The defect.** `RestoreAuthored` raises the round to the last authored header, which can be rounds past the restored certificates. Both catch-up paths, `requestRoundCatchUp` and `recoverFromStall`, start at the current round minus one. The rounds between the DAG's end and the current round were therefore never requested, and every certificate that arrived waited on parents in them.
- **The fix.** Catch-up starts at the highest round where the DAG holds a quorum of certificates (`catchUpFrom`). `TestCatchUpAsksFromWhereTheDAGEndsNotFromTheAuthoredRound` fails without the fix.

**Also seen in step D.** When the wedged node was restored, it found no consensus checkpoint matching its executor's block. It started at round 0, logged "Stranded beyond the catch-up horizon — a fresh fast-sync is required" (gap 4524, above the GC depth of 2000), took the join's fresh path, and was ACTIVE at 19:59:10Z and synced at 20:01:14Z. A fresh join works. The wedge belongs to the checkpoint-restore path only.

## Not checked

- **Two automatic captures.** `wedge-20261003T195511Z` and `delivery-stall-20261003T195428Z` fall in step D while BVN2 was deliberately stopped. They were not read.
- **Long-run behaviour.** This was a 46-minute run, which proves nothing about the network over time. That needs at least 12 hours.

Bulk logs deleted after the summary: `node-logs-live.txt` and `node-logs.txt`.
