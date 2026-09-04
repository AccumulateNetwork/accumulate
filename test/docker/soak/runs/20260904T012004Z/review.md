# Acceptance run #5 — review

S3 plus C4 and C5 (`cf4ea995f`), chaos off. 12 h / 500 tps, bcdb, 1 s blocks.
Stopped by stallkill at 1.75 h: Directory stalled 242 s. The longest run of
the series; loadgen 1,034,560 generated, rate collapsed to 166 tps at the end.

## Minutes 0–15: the first time everything held

All three partitions at 1.0 s per block, 500.6 tps with nothing rejected,
batches of ~30, own store 41–127 KB and peer store 185–351 KB per partition,
zero re-proposals, missing-batch deferrals 6–9 a minute, heap 284–385 MiB,
RSS 590 MiB. The BVN2 asymmetry of run #4 was gone: it had been the batch
plane's own spiral.

## Minutes 15–90: CPU rose, then plateaued

Fleet CPU 6.6 → 10.8 → 17.6 cores by minute 15 with execution work flat, then
13.5–17 cores to the end. The 18-minute profile: GC 14.5% (allocation at ~290
MB/s per node, 35% of it marshaling synthetic and sequenced messages through
`encoding.Hash`, #4211); `segment.lookup` 18% (history bloom walks over ~40
segments per shard, BlockchainDB#86); the API handler serving pulls 25% at the
hour-one profile. Heap 430–730 MiB, RSS ~900 MiB. The store's segments per
shard fell from 40 to 35 as merges ran.

BVN2 originates 2.5× BVN1's synthetics (98,858 vs 38,761 by minute 40) — the
load generator's active senders hash there — so it reached its own-store
share first and was throttled by refusal: 41 transitions, 187,000 submissions
skipped by minute 38, 4.7 s per block. Bounded, as S3 intends; slower, as the
offered load dictates.

## Healing at zero drops

No fault was injected and the network healed 139,852 times in 40 minutes. On
bvn1-val1, 22,166 pulls for 9,846 distinct numbers — 55% repeats, up to nine
for one number — each rebuilt with receipts at the source. This is the traffic
behind the marshaling churn. Spec: "Healing is monotonic, and the source
already has the answer"; H6 #4212, H1 #4193.

## What ended it

**H7.** From 02:30 pull requests began timing out. The reconcile ran on every
block (`reconcileInterval` = 1) as a new goroutine with a 90 s context, so
dozens overlapped per node; each continued past its deadline over up to 200
sequence numbers per source, every one failing on arrival. 9,392,756 "failed
to request missing synthetic: context deadline exceeded" lines in forty
minutes, ~4,000 a second network-wide; 1,686,543 "unable to decode request
from peer" on the API servers; the loadgen lost its peers ("no live peers for
submit"); the Directory stopped producing at 03:03 and the run was stopped at
03:07. Fixed on the branch: single-flight reconcile, stop at the deadline,
per-source back-off 1–2–4…64 blocks, one failure line per activation, the
timed-out read logged at Debug.

**C5b.** BVN2 nodes also waited on a batch retired 57 minutes earlier: a
digest that sat in the availability queue got into a header at round 4524
although its batch had been certified and executed at block 1261. C5 stopped
re-proposal of certified batches but not this path. Fixed: what a header takes
from the queue is filtered to batches still in the active store and named by
no certified header.

## Against the criteria

| criterion | 0–15 min | 15–90 min |
|---|---|---|
| memory flat | yes | no — heap 300 → 600 MiB with healing and store growth |
| GC not the workload | yes | **no** — up to 9.9 fleet GC cores; #4211 |
| CPU flat | yes | **no** — rose to ~15 fleet cores, then plateaued |
| block work bounded | yes | yes |
| commits durable | yes | yes |
| consensus memory bounded | **yes**, first run | yes — refusal held BVN2 |
| logging bounded | yes | **no** — 11 million lines from the reconcile storm; H7 |

## Order of work

H7 and C5b are fixed on the branch; run #6 tests them. Then H1 and H6, which
remove the healing traffic, before S4 and S5, which would only make it cheaper.

Files: `mem.csv`, `storage-stats.csv`, `probe-20260904T013858Z` (18 min),
`hourly-20260904T022250Z` (1 h), `wedge-20260904T030542Z`,
`probe-20260904T030736Z`.
