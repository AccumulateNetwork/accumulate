# DAG Consensus Implementation Plan

**Issue**: #3718, #3722
**Date**: March 2026
**Status**: Planning

---

## Overview

Replace CometBFT with custom DAG-based consensus (Bullshark-style) built in Go. This addresses:
- Node boot issues (peer discovery, state sync)
- Throughput limitations (200-500 TPS → 100k+ TPS)
- Data duplication (eliminate blockstore.db)
- Snapshot failures (BPT-centric sync)

## Algorithm Decision: Bullshark ✓

| Metric | Bullshark | Mysticeti |
|--------|-----------|-----------|
| Throughput | 100k+ TPS | 200k TPS |
| Latency | 2-3s | 0.4-0.5s |
| Core algorithm | ~270 lines | ~4k lines |
| Full impl | ~3k lines | ~15k+ lines |

**Decision**: Bullshark. Throughput matters, latency does not.

Mysticeti's sub-second latency requires complex synchronization for missing blocks, "blame" mechanisms, and commit finalization state machines. Not worth 5x complexity.

**Primary reference**: [Narwhal/Bullshark](https://github.com/PaulSnow/narwhal-reference) (Apache 2.0)
**Secondary reference**: [Sui/Mysticeti](https://github.com/PaulSnow/sui-consensus-reference) (if sub-second latency needed later)

---

## Architecture

### Current (CometBFT)
```
Transactions → CometBFT Mempool → Leader Proposes Block → Vote → Commit → ABCI → BPT
```

### Proposed (DAG)
```
Transactions → Worker Batches → Gossip → DAG Vertices → Bullshark Ordering → BPT
```

### Component Map

| CometBFT | DAG Replacement |
|----------|-----------------|
| Mempool | Worker (batch collection) |
| Block proposal | Header/Certificate creation |
| Tendermint P2P | libp2p + GossipSub (already have) |
| Block storage | None (BPT is authoritative) |
| State sync | BPT snapshots + chain proofs |

---

## Gossip Network Layer

### What We Have (pkg/api/v3/p2p/)
- **libp2p host** with DHT (Kademlia) for peer discovery
- **GossipSub** for pubsub messaging
- NAT traversal, relay, hole punching
- Service discovery via topic subscription

### What We Need to Add

**1. Reliable Broadcast Topics**

| Topic | Purpose | Messages |
|-------|---------|----------|
| `acc/{partition}/batches` | Worker batch announcements | BatchDigest |
| `acc/{partition}/headers` | Header broadcasts | Header |
| `acc/{partition}/certs` | Certificate broadcasts | Certificate |
| `acc/{partition}/votes` | Vote collection | Vote |

**2. Request/Response Protocols**

| Protocol | Purpose |
|----------|---------|
| `/acc/batch/1.0.0` | Fetch batch by digest |
| `/acc/cert/1.0.0` | Fetch certificate by digest |
| `/acc/sync/1.0.0` | DAG sync (catch-up) |

**3. Reliable Delivery Guarantees**
- GossipSub provides probabilistic delivery
- Add pull-based fallback for missing data
- Certificate availability = 2f+1 validators have it

### Implementation

```go
// pkg/consensus/gossip/gossip.go

type GossipLayer struct {
    host      host.Host
    pubsub    *pubsub.PubSub
    topics    map[string]*pubsub.Topic
    partition string
}

func (g *GossipLayer) BroadcastBatch(batch *Batch) error
func (g *GossipLayer) BroadcastHeader(header *Header) error
func (g *GossipLayer) BroadcastVote(vote *Vote) error
func (g *GossipLayer) BroadcastCertificate(cert *Certificate) error

func (g *GossipLayer) SubscribeBatches() <-chan *Batch
func (g *GossipLayer) SubscribeHeaders() <-chan *Header
func (g *GossipLayer) SubscribeVotes() <-chan *Vote
func (g *GossipLayer) SubscribeCertificates() <-chan *Certificate
```

---

## Phase 1: Core Data Structures

**Duration**: 1-2 weeks
**Lines**: ~1,500

### Types (pkg/consensus/types/)

```go
// Round number
type Round uint64

// Transaction batch
type Batch struct {
    Transactions [][]byte
    digest       BatchDigest // cached
}

// Header references batches and parent certificates
type Header struct {
    Author    PublicKey
    Round     Round
    Epoch     uint64
    Payload   map[BatchDigest]WorkerID  // batch → worker
    Parents   []CertificateDigest       // 2f+1 parent certs
    Signature Signature
    digest    HeaderDigest // cached
}

// Certificate = Header + aggregated signatures from 2f+1 validators
type Certificate struct {
    Header             Header
    AggregatedSig      AggregateSignature
    SignedAuthorities  []ValidatorID  // who signed
}

// Vote on a header
type Vote struct {
    HeaderDigest HeaderDigest
    Round        Round
    Epoch        uint64
    Author       PublicKey
    Signature    Signature
}
```

### DAG Structure (pkg/consensus/dag/)

```go
// DAG stores certificates organized by round
type DAG struct {
    mu     sync.RWMutex
    rounds map[Round]map[PublicKey]*Certificate

    // Garbage collection
    gcDepth          Round
    lastCommitRound  Round
}

func (d *DAG) Insert(cert *Certificate) error
func (d *DAG) Get(round Round, author PublicKey) *Certificate
func (d *DAG) GetRound(round Round) []*Certificate
func (d *DAG) HasQuorum(round Round) bool  // 2f+1 certs in round?
```

---

## Phase 2: Worker (Batch Creation)

**Duration**: 1 week
**Lines**: ~1,000

### Worker Role
- Collect transactions from clients
- Create batches when full or timeout
- Broadcast batch digests via gossip
- Serve batch data on request

```go
// pkg/consensus/worker/worker.go

type Worker struct {
    id          WorkerID
    partition   string
    gossip      *GossipLayer
    pending     [][]byte
    batchSize   int
    batchTime   time.Duration
}

func (w *Worker) Submit(tx []byte) error
func (w *Worker) Start(ctx context.Context) error

// Internal
func (w *Worker) createBatch() *Batch
func (w *Worker) broadcastBatch(batch *Batch)
func (w *Worker) serveBatchRequests()
```

---

## Phase 3: Primary (Certificate Creation)

**Duration**: 2 weeks
**Lines**: ~2,500

### Primary Role
- Create headers referencing worker batches
- Collect votes from other validators
- Aggregate signatures into certificates
- Broadcast certificates

```go
// pkg/consensus/primary/primary.go

type Primary struct {
    key         PrivateKey
    partition   string
    gossip      *GossipLayer
    dag         *DAG
    committee   *Committee

    // Current round state
    round       Round
    batches     map[BatchDigest]struct{}
    votes       map[HeaderDigest][]*Vote
}

func (p *Primary) Start(ctx context.Context) error
func (p *Primary) OnBatchAvailable(digest BatchDigest)
func (p *Primary) OnVoteReceived(vote *Vote)
func (p *Primary) OnCertificateReceived(cert *Certificate)

// Internal
func (p *Primary) createHeader() *Header
func (p *Primary) collectVotes(header *Header) []*Vote
func (p *Primary) createCertificate(header *Header, votes []*Vote) *Certificate
func (p *Primary) advanceRound()
```

### Certificate Availability

Before creating a header, must have:
- 2f+1 certificates from round-1 (parents)
- Batch data available for all referenced batches

---

## Phase 4: Bullshark Consensus

**Duration**: 1-2 weeks
**Lines**: ~1,000

### Core Algorithm

```go
// pkg/consensus/bullshark/bullshark.go

type Bullshark struct {
    committee        *Committee
    dag              *DAG
    gcDepth          Round
    lastCommitRound  Round
    lastCommitted    map[PublicKey]Round
}

// Called when a new certificate is added to the DAG
func (b *Bullshark) ProcessCertificate(cert *Certificate) []ConsensusOutput {
    round := cert.Round()

    // Only elect leaders for even rounds
    leaderRound := round - 1
    if leaderRound % 2 != 0 || leaderRound < 2 {
        return nil
    }

    // Already committed this round?
    if leaderRound <= b.lastCommitRound {
        return nil
    }

    // Get leader for this round
    leader := b.electLeader(leaderRound)
    if leader == nil {
        return nil
    }

    // Does leader have f+1 support from round's certificates?
    if !b.hasSupport(leader, round) {
        return nil
    }

    // Commit! Walk back through linked leaders
    return b.commitLeaderChain(leader)
}

func (b *Bullshark) electLeader(round Round) *Certificate {
    // Deterministic leader election based on round
    // Stake-weighted selection seeded by round number
}

func (b *Bullshark) hasSupport(leader *Certificate, round Round) bool {
    // Count certificates in `round` that reference `leader`
    // Return true if stake >= f+1
}

func (b *Bullshark) commitLeaderChain(leader *Certificate) []ConsensusOutput {
    // 1. Find all linked leaders back to lastCommitRound
    // 2. For each leader (oldest first), flatten its sub-dag
    // 3. Return ordered list of certificates to commit
}
```

### Helper Functions

```go
// Order leaders linked to current leader
func orderLeaders(leader *Certificate, dag *DAG, lastCommit Round) []*Certificate

// Check if path exists between two leaders
func linked(leader, prevLeader *Certificate, dag *DAG) bool

// Flatten sub-dag referenced by leader (depth-first)
func orderDag(leader *Certificate, dag *DAG, gcDepth Round) []*Certificate
```

---

## Phase 5: Execution Integration

**Duration**: 2 weeks
**Lines**: ~2,000

### Replace ABCI with DAG Commits

```go
// pkg/consensus/executor/executor.go

type Executor struct {
    bullshark   *Bullshark
    bpt         *BPT
    eventBus    *events.Bus
    anchors     *AnchorManager
}

func (e *Executor) OnCertificateCommitted(output ConsensusOutput) error {
    cert := output.Certificate

    // 1. Execute all transactions in certificate's batches
    for _, batchDigest := range cert.Header.Payload {
        batch := e.fetchBatch(batchDigest)
        for _, tx := range batch.Transactions {
            e.executeTransaction(tx)
        }
    }

    // 2. Update BPT
    e.bpt.Commit()

    // 3. Create anchor if needed
    if e.isAnchorPoint(cert) {
        e.anchors.CreateAnchor(cert, e.bpt.Root())
    }

    // 4. Emit events
    e.eventBus.Publish(events.BlockCommitted{...})
}
```

### Anchor Integration

Anchors map naturally to committed certificates:
- DN anchors: After each leader commit
- BVN anchors: Sent to DN after major blocks
- Synthetic routing: Unchanged

---

## Phase 6: Node Bootstrap & Sync

**Duration**: 2 weeks
**Lines**: ~2,000

### Bootstrap Protocol

```go
// pkg/consensus/sync/bootstrap.go

type Bootstrap struct {
    gossip    *GossipLayer
    dag       *DAG
    bpt       *BPT
}

func (b *Bootstrap) JoinNetwork(ctx context.Context) error {
    // 1. Connect to bootstrap peers (already have via libp2p DHT)

    // 2. Fetch latest BPT snapshot
    snapshot := b.fetchLatestSnapshot()

    // 3. Verify snapshot with operator signatures
    if !b.verifySnapshot(snapshot) {
        return errors.New("invalid snapshot")
    }

    // 4. Load BPT state
    b.bpt.LoadSnapshot(snapshot)

    // 5. Sync recent DAG (catch-up)
    b.syncDAG(snapshot.LastRound)

    // 6. Join consensus
    return nil
}
```

### BPT-Centric Sync

No CometBFT blockstore needed:
```
1. Get BPT snapshot (state at round N)
2. Verify with 2f+1 operator signatures
3. Sync DAG from round N to current
4. Ready to participate
```

---

## Phase 7: Migration Path

### Step 1: Parallel Operation
- Run DAG consensus alongside CometBFT
- DAG produces "shadow" commits
- Verify DAG output matches CometBFT

### Step 2: Switchover
- Network upgrade transaction activates DAG
- CometBFT stops proposing
- DAG takes over ordering

### Step 3: Cleanup
- Remove CometBFT integration code
- Remove blockstore.db
- Update node configuration

---

## Testing Strategy

### Unit Tests (~2,000 lines)
- DAG structure operations
- Bullshark algorithm
- Certificate creation/verification

### Integration Tests
- Multi-node consensus
- Partition failures
- Network partitions

### Simulator Tests
- Existing simulator framework
- Replace CometBFT mock with DAG mock

---

## Implementation Order

| Phase | Component | Dependencies | Est. Lines |
|-------|-----------|--------------|------------|
| 1 | Types & DAG | None | 1,500 |
| 2 | Gossip Layer | Types | 1,000 |
| 3 | Worker | Gossip, Types | 1,000 |
| 4 | Primary | Gossip, DAG, Worker | 2,500 |
| 5 | Bullshark | DAG | 1,000 |
| 6 | Executor | Bullshark, BPT | 2,000 |
| 7 | Bootstrap | Gossip, DAG, BPT | 2,000 |
| 8 | Migration | All | 1,000 |
| 9 | Tests | All | 2,000 |
| | **Total** | | **~14,000** |

---

## Directory Structure

```
pkg/consensus/
├── types/
│   ├── batch.go
│   ├── header.go
│   ├── certificate.go
│   ├── vote.go
│   └── committee.go
├── dag/
│   ├── dag.go
│   └── traversal.go
├── gossip/
│   ├── gossip.go
│   ├── topics.go
│   └── protocols.go
├── worker/
│   └── worker.go
├── primary/
│   ├── primary.go
│   ├── header_builder.go
│   └── certificate_aggregator.go
├── bullshark/
│   ├── bullshark.go
│   ├── ordering.go
│   └── leader.go
├── executor/
│   ├── executor.go
│   └── anchor.go
└── sync/
    ├── bootstrap.go
    └── snapshot.go
```

---

## References

- [Narwhal Reference Implementation](https://github.com/PaulSnow/narwhal-reference)
- [Bullshark Paper](https://arxiv.org/pdf/2201.05677)
- [dag-consensus-refs/README.md](dag-consensus-refs/README.md)
- [consensus-and-state-optimization.md](consensus-and-state-optimization.md)
- [node-boot-issues.md](node-boot-issues.md)
