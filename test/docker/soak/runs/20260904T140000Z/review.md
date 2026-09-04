# E8 check run — review

Run #6's code plus two-store staging (`issue-4217-two-store-staging` at
`7241e2b0b`), chaos off, 500 tps, 1 s blocks, bcdb. Meant to run 30 minutes;
stopped by stallkill at 17 minutes with the Directory's executor wedged at block
603. One question was asked: with the missing-anchor leg closed, do heals fall?

## The leg is closed

| bvn1-val1, 17 min | count |
|---|---|
| synthetics judged proven by the proven set | 87,908 |
| judged unproven and collected (held at their number) | 20,074 |
| proofs staged for an anchor not yet executed / validated | 70 / 945 |
| BVN2→BVN1 stream at 7 min | received 18,516 = delivered 18,516 |

A package that reaches BVN1 before the Directory anchor that proves it is held
and executes when the anchor lands. In run #6 the same case was a hole the
healer filled for a third of every stream.

## What the healer did instead

Minute by minute on `bvn1-val1`, gap-scan pulls (BVN1←BVN2): 1, 83, 160, 3,
1,279, 159, 71, 512, 246, 64, 476, 871, 1,883, 2,089, 877, 303. Every early
pull was for a distinct number, none repeated, and the stream had no backlog:
the healer asks for any hole at the next activation, and with everything now
sighted the moment it arrives, an out-of-order package one block late is a
visible hole for one activation. Run #6 hid those because nothing was sighted.
This is the immediate-pull behaviour H8 replaces with two cycles of patience.

## What ended it

The Directory. From minute 5 its executor fell behind its own consensus
(executing round 1,054 while voting on 1,203 at minute 8, 603 blocks executed
against round 1,637 at the wedge). Its reconcile scan then saw its inbound
tails as overdue and pulled 200-entry ranges from both BVNs: 1,000–5,000
`synthetic-range` heals per node. Each range answer is re-submitted into the
Directory's own mempool as 200 messages each carrying the same 200-element
proof, so the Directory's own batch store reached 235 MB against a 32 MB
share, its blocks grew to 600 messages, and its executor fell further behind —
the C6 spiral with range recovery as fuel.

Those range answers could never deliver. Their proofs terminate at a root of
the **source** (`rangeProofAnchor`), and the destination checks a proof against
the Directory anchor chain only, so they were `missing` — 33,432 on `bvn1-val1`
— collected and never proven. In run #6 the same messages were parked pending
and pulled again; that is why BVN→Directory streams delivered less than half of
what was produced there. Recorded as DIFFERENCES H9; the path is retired with
H8.

## Verdict

E8 does what it claims for BVN↔BVN traffic and the counters to prove it
exist. The network still cannot run, because healing is still the per-message,
no-patience, source-rooted mechanism H8 replaces, and because nothing bounds
how far consensus runs ahead of execution (C6). Order stands: H8, then C6, then
the next check.

Files: `mem.csv`, `storage-stats.csv`, `wedge-20260904T141901Z`,
`probe-20260904T142053Z`, `node-logs-live.txt`.
