# Footprint at rung 100 (2026-08-25T14:51Z, DN height ~950)

Measured on the live 8-node network, not a simulation. This is the first
node-side footprint reading the sub-1GB work has ever had; every number in
docs/plans/sub-1gb-footprint.md before this came from exp/blockfile-sim.

## The reading

| | |
|---|---|
| fleet RSS | 1174–1238 MiB across all 8 validators, tightly clustered |
| cgroup | 75–79% of the 1.5 GiB `mem_limit` |
| live heap after a FORCED GC (bvn1-val1) | **864 MiB** |
| `next_gc` | 996 MiB |
| `heap_sys` | 1435 MiB |
| goroutines | 300, flat across the fleet |

The forced GC is what makes this conclusive. RSS sitting near GOMEMLIMIT could
have been nothing but GC slack — Go using an allowance it was granted. It is
not: 864 MiB survives a collection. That is retained live data.

## Where it is (inuse_space, whole process = both engines, 602 MB sampled)

| site | MB | note |
|---|---|---|
| `goleveldb util.(*BufferPool).Get` | 130 | **NOT a pool.** See below. |
| `leveldb.(*recordCache).put` | 132 | the write-path cache, deliberately kept |
| `consensus/types.UnmarshalCertificate` | 50 | |
| `consensus/types.UnmarshalHeader` | 38 | |
| `goleveldb/memdb.New` | 32 | write buffers, 16 MB x 2 engines |
| `container/list.insertValue` | 31 | LRU spines |
| `worker.(*Worker).createBatch` | 29 | |
| `pubsub pb.(*Message).Unmarshal` | 19 | |

No single hog. It is broadly distributed, which is what makes it hard.

### The BufferPool line is not the 887 MB regression coming back

`BufferPool.Get` being the top inuse site looks alarming given 42fc9888e turned
the pool off. It is a false alarm, and worth writing down so the next reader
does not re-litigate it:

  func (p *BufferPool) Get(n int) []byte {
      if p == nil { return make([]byte, n) }   // <- this path

`DisableBufferPool: true` leaves `bpool` nil (goleveldb table.go:527 guards
construction on it; the unguarded `util.NewBufferPool` at db.go:305 is a local
inside journal recovery, not the steady-state path). So every read and
compaction buffer allocates through this function and is ATTRIBUTED to it,
while nothing is retained by it. This is precisely the "churn tax" the code
comment predicted. The pool is off and staying off.

## What this means for the 1 GB target

The plan doc's ~680 MB is a budget of CONFIGURED CACHES. The live set is 864 MB
on top of the runtime, and it grows with chain height. Two consequences:

1. **Sub-1GB is unreachable while `GOMEMLIMIT` is 1200 MiB.** Go is entitled to
   1.2 GB and takes it. Lowering the ceiling is not a fix on its own either:
   with an 864 MB live set, an 800 MiB ceiling puts the GC into the continuous
   collection that collapsed run 20260824T123427Z (1250 MiB against a ~1.1 GB
   live set, blocks stretched to 3s, dead in 15 minutes). The ceiling can only
   follow the live set down.
2. **So the remaining lever is the live set itself**, which is item 6 — the
   hybrid store — plus the record cache. The measured case for item 6 was the
   write-stall collapse (12 s -> 35 ms) and the ability to tolerate a 32 MB
   block cache. This reading adds the memory case: 130 MB of leveldb read and
   compaction churn is work the hybrid does not do at all for immutable
   entries, because they are never overwritten and never compacted.

## Caveat

One rung, 40 minutes, 100 tps, height ~950. Whether the live set is flat or
still climbing is the question the rest of this run answers — the doc's
"drifts upward for the first hours of every run" symptom is exactly what a
12-hour hold is for. Do not quote 864 MB as a steady state yet.
