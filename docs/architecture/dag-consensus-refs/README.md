# DAG Consensus Implementation References

## Papers

| Paper | Focus | Link |
|-------|-------|------|
| Narwhal and Tusk | DAG mempool + async consensus | [arxiv:2105.11827](https://arxiv.org/pdf/2105.11827) |
| Bullshark | Partially synchronous DAG BFT | [arxiv:2201.05677](https://arxiv.org/pdf/2201.05677) |
| Mysticeti | Uncertified DAG (lower latency) | [arxiv:2310.14821](https://arxiv.org/pdf/2310.14821) |
| Shoal++ | Pipelining improvements | [NSDI'25](https://www.usenix.org/system/files/nsdi25-arun.pdf) |

## Reference Implementations

| Repo | Algorithm | Status | Lines |
|------|-----------|--------|-------|
| [github.com/PaulSnow/narwhal-reference](https://github.com/PaulSnow/narwhal-reference) | Bullshark | Archived | ~35k |
| [github.com/PaulSnow/sui-consensus-reference](https://github.com/PaulSnow/sui-consensus-reference) | Mysticeti | Production | ~38k |

Both Apache 2.0 licensed. Narwhal is simpler (good starting point), Sui/Mysticeti is production (sub-second latency).

### Decision: Bullshark

**Rationale**: Throughput matters, latency does not.

| Metric | Bullshark | Mysticeti |
|--------|-----------|-----------|
| Throughput | 100k+ TPS | 200k TPS |
| Latency | 2-3s | 0.4-0.5s |
| Core algorithm | ~270 lines | ~4k lines |
| Full implementation | ~3k lines | ~15k+ lines |
| Complexity | Simple | Complex state machines |

Mysticeti's sub-second latency requires complex block synchronization, missing block fetching, and "blame" mechanisms. Not worth 5x implementation complexity when latency isn't a priority.

**Primary reference**: [narwhal-reference](https://github.com/PaulSnow/narwhal-reference) (Bullshark)
**Secondary reference**: [sui-consensus-reference](https://github.com/PaulSnow/sui-consensus-reference) (Mysticeti, for future optimization if needed)

### Code Breakdown (~35k lines total)

| Component | Lines | Purpose |
|-----------|-------|---------|
| primary | 16,053 | Certificate creation, DAG building |
| consensus | 3,121 | Bullshark ordering protocol |
| worker | 2,469 | Batch creation and broadcasting |
| types | 2,330 | Data structures |
| node | 1,946 | Node orchestration |
| network | 1,211 | P2P networking |
| dag | 1,096 | DAG structure and traversal |
| executor | 972 | Transaction execution interface |
| storage | 652 | Persistence layer |

### Key Files to Study

**Core Algorithm (~270 lines total)**
- `consensus/src/bullshark.rs` - Bullshark ordering (~167 lines)
  - `process_certificate()` - Main entry point
  - `leader()` - Leader election per round
- `consensus/src/utils.rs` - DAG traversal (~100 lines)
  - `order_leaders()` - Find linked leaders to commit
  - `order_dag()` - Flatten sub-dag for ordering
  - `linked()` - Check path between leaders

**DAG Structure**
- `dag/src/lib.rs` - Node/vertex with parents, path compression

**Certificate & Batching**
- `primary/src/primary.rs` - Certificate creation
- `worker/src/worker.rs` - Batch handling
- `types/src/` - All data structures (Certificate, Header, etc.)

## Online Resources

- [DAG Meets BFT](https://decentralizedthoughts.github.io/2022-06-28-DAG-meets-BFT/) - Conceptual overview
- [Narwhal GitHub](https://github.com/MystenLabs/narwhal) - Archived repository

## What We Need for Accumulate

### Can Skip/Simplify
- **Storage**: Use existing Accumulate database
- **Executor**: Already have BPT execution model
- **Network**: Adapt existing partition networking
- **Much of Primary**: Simpler certificate model possible

### Must Implement
- **DAG structure**: Core vertex/edge model
- **Bullshark consensus**: ~200 lines core algorithm
- **Worker batching**: Transaction collection
- **Certificate handling**: Signature aggregation

### Integration Points
- Replace CometBFT's ABCI with DAG vertex submission
- Anchor system maps naturally to DAG commits
- BPT updates on committed vertices
- Existing synthetic transaction routing unchanged

## Estimated Implementation Size

| Component | Estimated Go Lines |
|-----------|-------------------|
| DAG structure | 500-1,000 |
| Bullshark consensus | 500-1,000 |
| Vertex/batch handling | 1,500-2,500 |
| Certificate/signatures | 1,000-1,500 |
| Network integration | 1,000-2,000 |
| Storage integration | 500-1,000 |
| Testing | 2,000-3,000 |
| **Total** | **7,000-12,000** |
