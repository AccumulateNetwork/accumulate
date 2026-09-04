# Run 20260904T221627Z — history-read attribution

40 min, 500 tps, chaos off, BlockchainDB, on `issue-4219-absence-reads`
@42263d7da: R1 (a mutable miss never walks permanent history) plus the
instrumentation — `shallowMisses` by shape, `fallbackWalks`, and
`historyReads` {hits, misses, distinct, callers} per shape in every store's
`stats.json`, collected per node (the collector bug that wrote one node's
file under every name is fixed; 16 distinct files). Load 1,198,780
generated, 136 rejected, 26,480 skipped; every followed transaction
delivered. This run measures read patterns; it is not a stability claim.

## The question: cache, or smarter?

A cache pays for repeats. Over the eight BVN stores, 2,250 commits each:

| shape read from history | hits | distinct keys | hits per key | verdict |
|---|---|---|---|---|
| `RootChain.Index.Element` | 424,392 | 6,720 | **63** | the one repeat reader: `getRootReceiptForBlock`'s binary search over the root index chain re-reads the same entries every block. A cache would pay; recording the root index position per block removes the search. |
| `Message.Main` | 442,652 | 442,652 | 1.0 | dispatch reading a block's synthetics once their anchor is older than the window. No cache can help a single read. The reader should carry the block's messages, and C6 keeps the anchor leg under the window. |
| `RootChain.States` | 9,280 | 7,628 | 1.2 | receipts; negligible |
| `RootChain.Element` | 4,820 | 4,820 | 1.0 | negligible |
| `SyntheticIndexIndex` | 4,836 | 4,836 | 1.0 | one read per anchored block; a dynamic-layer record |

Everything else that walked history found nothing. **113.8 million walks on
the BVN stores, 886,000 hits: 99.2% of the walks proved a key absent before
its first write.** Per node per block that is ~6,300 walks, on these shapes:

| permanent shape missed | misses (8 nodes) | first-write site |
|---|---|---|
| `Message.Main` | 26.9 M | recording a message and its status; `putMessageWithStatus`; local deliveries |
| `SignatureChain` element + index | 17.2 M each | every signature, credit payment, signature request, authority signature |
| `MainChain` element + index | 7.3 M each | the chain update for every transaction; building synthetics |
| `Transaction.Main` | 6.4 M | **the v1 record shape, never written by v2**: `recordTransaction`, `TransactionMessage.Process`, `resolveTransaction` read it through the message record's fallback |
| `RootChain` element + index | 6.2 M each | anchoring every touched chain at block close |
| index chains, sequence chains, replicas, scratch, block ledger, BPT chain | the rest | one first write per entry or per block |

Mutable shapes no longer walk (R1): `Transaction.Status`, `Produced`,
`Cause`, `Signatures`, `History` are 45% of all shallow misses and cost only
the dynamic layer's own lookup now.

**Answer: smarter.** Remove the two absence proofs per first write (the
value layer's version pre-read, D7; the chain's element-index check, D8) and
the dead `Transaction.Main` read, and 99% of the walks are gone. Cache one
thing, or better remove it: the root-index search.

## Latency grew at the same minute as before

BVN2's executor fell behind its DAG from minute 17 of load, as in
20260904T180918Z, and this longer run shows what the shorter one could not:
the executor's throughput fell as it aged.

| minute of load | BVN2 lag behind its DAG | BVN2 tx/s executed | BVN2 blocks/min |
|---|---|---|---|
| 7–13 | 1 block | 350–400 | 59 |
| 17–20 | 11–39 | 230–380 | 37–62 |
| 25–30 | 120–241 | 165–270 | 25–40 |
| 35–40 | 351–500 | 150–240 | 22–35 |
| 45–48 | 693–776 | 87–170 | 13–25 |

The Directory held at a lag of 0–1 throughout. Removing the mutable walks
did not move the knee: the permanent-shape walks remained at ~6,300 a block,
and each one probes every history segment's filter from disk, so the cost
of proving an absence grows with the history. That is consistent with an
executor that slows as it ages under constant load, but this run did not
record the segment count, so it is the explanation the numbers point to,
not one they prove. The next run should carry the store's segment count in
`stats.json`.

## Instrumentation notes

This run's image sampled callers into one map per shape, so the "callers"
column mixes hit and miss callers and is dominated by the first-write sites;
f1fff6869 splits them for the next run. No wedge or hourly probe fired, so
there is no CPU profile from this run.
