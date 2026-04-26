# Accumulate Simulator Architecture

## Overview

The Accumulate simulator is a self-contained testing environment that simulates a complete Accumulate network without requiring actual network connectivity or CometBFT consensus. It provides deterministic block execution, multi-partition support, and API compatibility with the real network.

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────────┐
│                        Test Harness                             │
│                  (test/harness, test/validate)                  │
├─────────────────────────────────────────────────────────────────┤
│                      API Layer                                  │
│        services.Network (message.Client) + P2P nodes           │
├─────────────────────────────────────────────────────────────────┤
│                     Simulator                                   │
│              (orchestrates partitions, hub)                     │
├─────────────────────────────────────────────────────────────────┤
│                     Partitions                                  │
│         Directory (DN) + Block Validators (BVNs)               │
├─────────────────────────────────────────────────────────────────┤
│                       Nodes                                     │
│      (consensus.Node, database, services, eventBus)            │
├─────────────────────────────────────────────────────────────────┤
│                   Consensus Layer                               │
│     (SimpleHub, Dispatcher, state machines, mempool)           │
├─────────────────────────────────────────────────────────────────┤
│                   Executor Layer                                │
│          (execute.Executor, ExecutorApp)                       │
├─────────────────────────────────────────────────────────────────┤
│                   Database Layer                                │
│          (database.Database, keyvalue stores)                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Core Components

### 1. Simulator (`simulator.go`)

The top-level orchestrator that manages all partitions and coordinates execution.

```go
type Simulator struct {
    deterministic bool              // Run in deterministic mode
    networkId     string            // Network identifier (e.g., "TestValidateAPI")
    logger        log.Logger        // Logger
    router        *Router           // Routes accounts to partitions
    services      *services.Network // API v3 service routing
    tasks         *taskQueue        // Background task execution
    partIDs       []string          // Ordered list of partition IDs
    partitions    map[string]*Partition  // Partition instances
    hub           consensus.Hub     // Message hub for consensus
}
```

**Key Functions:**
- `New(opts ...Option)` - Creates simulator with options
- `Step()` - Executes one block on all partitions
- `Submit(envelope)` - Routes and submits transaction
- `Services()` - Returns the `message.Client` for API calls
- `SetService(address, handler)` - Replaces a service handler
- `DatabaseFor(account)` - Returns database for account's partition
- `ListenAndServe(ctx, opts)` - Starts HTTP/P2P listeners

### 2. Partition (`partition.go`)

Represents a single network partition (DN or BVN).

```go
type Partition struct {
    protocol.PartitionInfo       // ID, Type (Directory/BlockValidator)
    sim        *Simulator        // Parent simulator
    logger     log.Logger
    mu         *sync.Mutex
    nodes      []*Node           // Validator nodes in this partition
    submitHook SubmitHookFunc    // Optional hook for submissions
}
```

**Key Functions:**
- `Submit(envelope, pretend)` - Submit transaction to partition
- `View/Update(fn)` - Database access
- `SetSubmitHook/SetBlockHook` - Testing hooks
- `initChain(snapshot)` - Initialize from genesis snapshot

### 3. Node (`node.go`)

A single validator node within a partition.

```go
type Node struct {
    id         int                       // Node index within partition
    network    *accumulated.NodeInit     // Node configuration
    partition  *Partition                // Parent partition
    logger     log.Logger
    eventBus   *events.Bus               // Event subscription
    nodeKey    []byte                    // libp2p node key
    privValKey []byte                    // Validator private key
    peerID     peer.ID                   // libp2p peer ID
    consensus  *consensus.Node           // Consensus state machine
    database   *database.Database        // Account database
    services   *message.Handler          // Message service handler
}
```

**Implements:**
- `api.ConsensusService` - Consensus status
- `api.Submitter` - Transaction submission
- `api.Validator` - Transaction validation

### 4. Router (`router.go`)

Routes accounts and transactions to the correct partition.

```go
type Router struct {
    tree      *routing.RouteTree        // Routing rules from globals
    logger    logging.OptionalLogger
    overrides map[[32]byte]string       // Manual route overrides
}
```

**Key Functions:**
- `RouteAccount(account)` - Get partition for account
- `Route(envelopes...)` - Get partition for envelope
- `SetRoute(account, partition)` - Override routing
- `willChangeGlobals(event)` - Updates routing table from globals

---

## Services Layer

### services.Network (`services/services.go`)

Manages service registration and message routing within the simulator.

```go
type Network struct {
    Services Services          // Service handlers by address
    *message.Client            // API client for queries
}

type Services map[string]map[peer.ID]Handler
type Handler = func(message.Stream)
```

**Key Functions:**
- `RegisterService(id, address, handler)` - Register service (no overwrite)
- `Replace(id, address, handler)` - Replace service handler
- `GetHandler(address)` - Get handler for address
- `Dial(ctx, addr)` - Create stream to service (routes internally)

### Service Registration Flow

During simulator initialization (`factory.go`):

1. **Faucet Service** - Registered at simulator level (factory.go:112-113)
   ```go
   handler, _ := message.NewHandler(message.Faucet{Faucet: (*simFaucet)(s)})
   s.services.RegisterService("", api.ServiceTypeFaucet.AddressForUrl(protocol.AcmeUrl()), handler.Handle)
   ```

2. **Per-Node Services** - Registered in `makeCoreApp()` (factory.go:518-557)
   - `ServiceTypeNode` - Node info/service discovery
   - `ServiceTypeConsensus` - Consensus status
   - `ServiceTypeSubmit` - Transaction submission
   - `ServiceTypeValidate` - Transaction validation
   - `ServiceTypeQuery` - Account queries
   - `ServiceTypeEvent` - Event subscriptions
   - `ServiceTypeNetwork` - Network status
   - `ServiceTypeSequencer` - Private sequencer

3. **P2P Registration** - In `listenP2P()` (api.go:294-311)
   - Registers services with libp2p node
   - Faucet only registered on first DN node

---

## Consensus Layer

### consensus.Hub (`consensus/simple.go`)

Distributes messages to all registered modules.

```go
type SimpleHub struct {
    mu      *sync.Mutex
    context context.Context
    modules atomic.Pointer[[]Module]
}
```

**Key Functions:**
- `Register(module)` - Add module to receive messages
- `Unregister(module)` - Remove module
- `Send(messages...)` - Broadcast to all modules, collect responses
- `With(modules...)` - Create hub with additional modules

### consensus.Node (`consensus/node.go`)

The consensus state machine for a single validator.

```go
type Node struct {
    mu             sync.Locker
    app            App                     // ExecutorApp
    record         Recorder
    context        context.Context
    network        string                  // Partition ID
    blockState     blockState              // Current block state machine
    submitState    map[*messaging.Envelope]submitState
    self           *validator              // This node's identity
    validators     []*validator            // All validators
    mempool        *mempool                // Pending transactions
    lastBlockIndex uint64
    lastBlockTime  time.Time
    executeHook    ExecuteHookFunc
}
```

**Implements Module interface:**
- `Receive(messages...)` - Process consensus messages

### Consensus Messages

```
Messages:
├── StartBlock        - Trigger new block execution
├── SubmitEnvelope    - Submit transaction to partition
├── proposeLeader     - Propose block leader
├── proposeBlock      - Leader's block proposal
├── acceptBlockProposal - Accept block proposal
├── finalizedBlock    - Block execution results
├── committedBlock    - Block commit result
├── ExecutedBlock     - Block completed notification
├── acceptedSubmission - Envelope accepted
└── EnvelopeSubmitted - Envelope submission result
```

### Dispatcher (`consensus/dispatcher.go`)

Routes synthetic/anchor messages between partitions via the hub.

```go
type Dispatcher struct {
    router routing.Router
    mu     *sync.Mutex
    queue  []Message
}
```

**Key Functions:**
- `Submit(ctx, dest, envelope)` - Queue envelope for destination
- `Receive(messages...)` - Return queued messages (clears queue)

---

## State Machine Transitions

### Block Execution State Machine

```
              ┌─────────────────┐
              │   (quiescent)   │
              └────────┬────────┘
                       │ StartBlock
                       ▼
              ┌─────────────────┐
              │ didProposeLeader│◄─── proposeLeader from others
              └────────┬────────┘
                       │ threshold reached
                       ▼
         ┌─────────────┴─────────────┐
         │                           │
    (is leader)               (not leader)
         │                           │
         ▼                           ▼
┌─────────────────┐        ┌─────────────────┐
│ didProposeBlock │        │ wait for block  │
│  (propose to    │        │   proposal      │
│   network)      │        │                 │
└────────┬────────┘        └────────┬────────┘
         │                          │ proposeBlock
         │ acceptBlockProposal      │
         │◄─────────────────────────┘
         ▼
┌─────────────────┐
│ didFinalizeBlock│ ← Execute block, broadcast results
└────────┬────────┘
         │ threshold reached
         ▼
┌─────────────────┐
│ didCommitBlock  │ ← Commit to database
└────────┬────────┘
         │ threshold reached
         ▼
    ExecutedBlock
```

### Submission State Machine

```
    SubmitEnvelope
         │
         ▼
┌─────────────────────┐
│ didAcceptSubmission │◄─── acceptedSubmission from others
└────────┬────────────┘
         │ threshold reached
         ▼
   (add to mempool)
         │
         ▼
   EnvelopeSubmitted
```

### Voting Threshold

From `voting.go`:
```go
// reachedThreshold returns true if 2/3 of the validators have voted.
func (v votes) reachedThreshold() bool {
    r := len(v.validators) - len(v.votes)
    return r*3 <= len(v.validators)
}
```

---

## Data Structures

### Mempool (`consensus/mempool.go`)

Tracks pending transactions awaiting inclusion in blocks.

```go
type mempool struct {
    count      int
    mu         sync.Mutex
    logger     logging.OptionalLogger
    pool       map[[32]byte]*mpEntry    // By envelope hash
    candidates []*mpEntry               // For next proposal
}

type mpEntry struct {
    order int                    // Arrival order
    hash  [32]byte              // Envelope hash
    env   *messaging.Envelope   // The envelope
}
```

**Key Functions:**
- `Add(envelope)` - Add to pool with arrival order
- `Propose(block)` - Get ordered envelopes for proposal
- `CheckProposed(block, envelope)` - Verify envelope in pool
- `AcceptProposed(block, envelopes)` - Remove proposed, prepare candidates

### TaskQueue (`task_queue.go`)

Manages background tasks during block execution.

```go
type taskQueue struct {
    errg atomic.Pointer[errgroup.Group]
}
```

**Key Functions:**
- `Go(fn)` - Launch background task
- `Flush()` - Wait for all tasks, swap error group

---

## API Integration

### Two Paths for API Calls

1. **Direct (Internal)** - via `services.Network.Client`
   ```
   Test → sim.Services() → services.Network.Client → services.Services.Dial() → handler
   ```

2. **P2P (External)** - via libp2p network
   ```
   Test → p2p.Client → libp2p → p2p.Node → registered handler
   ```

### Service Handler Chain

For a query through P2P:
```
p2p.Client.Query()
    → libp2p DHT discovery
    → p2p.Node receives stream
    → n.services.Handle (message.Handler)
    → Querier service
    → apiimpl.Querier.Query()
    → database
```

### simFaucet vs v3impl.Faucet

**simFaucet** (`faucet.go`):
- Directly updates database
- Returns fake TxID `[32]byte{1}`
- Transactions NOT queryable
- Fast, no consensus needed

**v3impl.Faucet** (`internal/api/v3/faucet.go`):
- Creates real transactions
- Submits through normal flow
- Transactions ARE queryable
- Requires consensus execution

---

## Factory/Build Pattern

### Build Hierarchy

```
simFactory (options)
    └── Build() → Simulator
            └── networkFactory.Build() → Partition
                    └── nodeFactory.Build() → Node
```

### simFactory Fields

```go
type simFactory struct {
    // Configuration
    network       *accumulated.NetworkInit
    storeOpt      OpenDatabaseFunc
    snapshot      SnapshotFunc
    recordings    RecordingFunc
    abci          abciFunc
    initialSupply *big.Int

    // Behavior flags
    dropDispatchedMessages      bool
    skipProposalCheck           bool
    ignoreDeliverResults        bool
    ignoreCommitResults         bool
    deterministic               bool
    dropInitialAnchor           bool
    disableAnchorHealing        bool
    interceptDispatchedMessages DispatchInterceptor

    // Cached state
    logger           log.Logger
    taskQueue        *taskQueue
    router           *Router
    hub              consensus.Hub
    services         *services.Network
    dispatcherFunc   func() execute.Dispatcher
    networkFactories []*networkFactory
}
```

---

## Execution Flow

### 1. Simulator Creation

```
New(opts...)
    → simFactory.Build()
        → create Router
        → create SimpleHub
        → create services.Network
        → register simFaucet
        → for each partition:
            → networkFactory.Build()
                → for each node:
                    → nodeFactory.Build()
                        → create database
                        → create eventBus
                        → register services
                        → create consensus.Node
        → hub.Register(node.consensus)
    → for each partition:
        → partition.initChain(snapshot)
            → consensus.Node.Init()
                → restore snapshot
                → init executor
    → set initial supply
```

### 2. Transaction Submission

```
sim.Submit(envelope)
    → router.Route(envelope)
    → sim.SubmitTo(partition, envelope)
        → partition.Submit(envelope, false)
            → hub.Send(SubmitEnvelope{...})
                → consensus.Node.Receive()
                    → processSubmission()
                        → app.Check() (validate)
                        → state machine
                        → mempool.Add()
                → return EnvelopeSubmitted
```

### 3. Block Execution (Step)

```
sim.Step()
    → hub.Send(StartBlock{})
        → each consensus.Node.Receive()
            → proposeLeader()
            → [voting]
            → proposeBlock() / acceptBlockProposal()
            → [voting]
            → finalizeBlock()
                → app.Execute()
                    → executor.Begin(params)
                    → for each envelope:
                        → block.Process(envelope)
                    → block.Close()
            → [voting]
            → commitBlock()
                → app.Commit()
                    → blockState.Commit()
                    → eventBus.Publish(DidCommitBlock)
            → [voting]
            → completeBlock()
        → return ExecutedBlock
    → tasks.Flush() (background tasks)
```

---

## Key Interfaces

### execute.Executor

```go
type Executor interface {
    LastBlock() (*BlockParams, [32]byte, error)
    Init([]*ValidatorUpdate) ([]*ValidatorUpdate, error)
    Validate(*messaging.Envelope, bool) ([]*protocol.TransactionStatus, error)
    Begin(BlockParams) (Block, error)
}
```

### consensus.App

```go
type App interface {
    Info(*InfoRequest) (*InfoResponse, error)
    Check(*CheckRequest) (*CheckResponse, error)
    Init(*InitRequest) (*InitResponse, error)
    Execute(*ExecuteRequest) (*ExecuteResponse, error)
    Commit(*CommitRequest) (*CommitResponse, error)
}
```

### consensus.Module

```go
type Module interface {
    Receive(...Message) ([]Message, error)
}
```

### api.Submitter / api.Querier / api.Faucet

Standard Accumulate API interfaces implemented by services.

---

## Configuration Options

| Option | Description |
|--------|-------------|
| `WithNetwork(net)` | Network configuration |
| `WithDatabase(fn)` | Database opener function |
| `WithSnapshot(fn)` | Snapshot provider |
| `WithRecordings(fn)` | Recording output files |
| `Deterministic()` | Deterministic execution |
| `DropDispatchedMessages()` | Drop internal dispatches |
| `DropInitialAnchor()` | Drop initial anchors |
| `DisableAnchorHealing()` | Disable anchor healing |
| `SkipProposalCheck()` | Skip proposal validation |
| `IgnoreDeliverResults()` | Ignore tx result mismatches |
| `IgnoreCommitResults()` | Ignore commit hash mismatches |
| `Genesis(time)` | Create genesis snapshot |
| `InitialAcmeSupply(v)` | Set initial ACME supply |
| `UseABCI()` | Use ABCI app wrapper |

---

## P2P Integration

### ListenAndServe Flow

```
sim.ListenAndServe(ctx, opts)
    → for each partition:
        → for each node:
            → node.listenAndServeHTTP()
                → setup HTTP mux
                → register v2/v3 endpoints
            → node.listenP2P()
                → p2p.New()
                → connect to previous nodes
                → register services with p2p.Node
                → (first DN node) register faucet
```

### Service Registration in P2P

From `api.go:294-311`:
```go
p2p.RegisterService(api.ServiceTypeConsensus.AddressFor(n.partition.ID), n.services.Handle)
p2p.RegisterService(api.ServiceTypeMetrics.AddressFor(n.partition.ID), n.services.Handle)
p2p.RegisterService(api.ServiceTypeNetwork.AddressFor(n.partition.ID), n.services.Handle)
p2p.RegisterService(api.ServiceTypeQuery.AddressFor(n.partition.ID), n.services.Handle)
p2p.RegisterService(api.ServiceTypeSubmit.AddressFor(n.partition.ID), n.services.Handle)
p2p.RegisterService(api.ServiceTypeValidate.AddressFor(n.partition.ID), n.services.Handle)
p2p.RegisterService(api.ServiceTypeEvent.AddressFor(n.partition.ID), n.services.Handle)

// Faucet only on first DN node
if n.partition.Type == protocol.PartitionTypeDirectory && n.id == 0 {
    handler := n.partition.sim.services.Services.GetHandler(faucetAddr)
    p2p.RegisterService(faucetAddr, handler)
}
```

---

## File Index

| File | Purpose |
|------|---------|
| `simulator.go` | Main Simulator struct, New(), database access |
| `factory.go` | Build pattern, service registration |
| `partition.go` | Partition management, Submit() |
| `node.go` | Node struct, consensus/submit interfaces |
| `api.go` | HTTP/P2P listeners, Services() |
| `router.go` | Account routing |
| `dispatcher.go` | Cross-partition message dispatch |
| `faucet.go` | simFaucet implementation |
| `consensus.go` | Submit/Step via hub |
| `task_queue.go` | Background task management |
| `options.go` | Configuration options |
| `services/services.go` | Service registration/routing |
| `consensus/node.go` | Consensus state machine |
| `consensus/state.go` | State machine helpers |
| `consensus/state_node.go` | Node message handling |
| `consensus/state_submit.go` | Submission state machine |
| `consensus/state_block.go` | Block execution state machine |
| `consensus/simple.go` | SimpleHub implementation |
| `consensus/dispatcher.go` | Synthetic message dispatcher |
| `consensus/app.go` | ExecutorApp |
| `consensus/mempool.go` | Transaction mempool |
| `consensus/voting.go` | Vote tracking |
| `consensus/messages.go` | Message type definitions |
| `consensus/types.go` | Module/Hub interfaces |
