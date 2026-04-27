# Minimum-Data Node Bootstrap

**Status:** Draft
**Owner:** Paul Snow
**Scope:** Bring up a new Accumulate node without downloading or replaying ~99.9999% of historical chain data, with cryptographic proof that the running state derives from the genesis block. Backfill history asynchronously after the node is running.
**Non-goals:** Replacing Accumulate's existing sync engine (we add a startup phase, not a new sync). Forward derivation from genesis (rejected — see §4). Validator-mode bootstrap before reaching ACTIVE state.

Tracking issue: #3953. Child issues are referenced by number throughout (#3954, #3957–#3970).

---

## 1. Motivation

A new Accumulate node today must reach consensus on the full chain state — either by syncing from genesis or by trusting an out-of-band snapshot file. Both paths cost time and bandwidth: large snapshots, long replay windows, manual artifact distribution.

The realistic chain history is dominated by transactions that are no longer needed to participate. A validator only needs the *current* state of accounts to vote on the next block. A query node needs current state plus whatever history its callers ask for.

This proposal:

- Lets a new node boot from a tiny seed (the binary's pinned genesis snapshot hash) and converge to a fully participating validator in minutes rather than hours.
- Produces — and persists — a cryptographic proof that the running state derives from the genesis block. No "trust this snapshot file" step.
- Lets validators come online before history is fully backfilled, without compromising query correctness (state-aware peer routing handles this).
- Backfills history asynchronously, optional, never blocking the running node.

The user-facing artifact is a new `accumulated bootstrap` subcommand on the existing binary. No new artifact to distribute.

## 2. Design decisions

### 2.1. Proof construction is **backward**, not forward

The launcher pulls current account state from peers and **back-walks** each main chain it cares about, verifying signatures at each step against the keypage state authoritative at that step's block time. Recursion bottoms out at the genesis snapshot, whose hash is pinned in the binary.

Rejected alternative: walking anchors *forward* from genesis to tip, applying `Updates` to evolve a validator-set timeline. Forward derivation has a chicken-and-egg first-anchor problem, requires us to derive state we already have on hand, and conflates "verify authority" with "reproduce state."

Back-walking is purely a verification activity. The state itself is delivered (current snapshot from peers), and the back-walk only proves the validators who attest that state trace back to genesis.

### 2.2. Three-state node model

```
BOOTING → ACTIVE → COMPLETE
```

| State | Proof | Full current state | Full history | Mode | Trust model |
|---|---|---|---|---|---|
| **BOOTING** | partial | no | no | Receiver | Trusts validator quorum signatures (back-walked to genesis); verifies leaves via Merkle proofs against committed `StateTreeAnchor`. **Cannot execute transactions.** |
| **ACTIVE** | yes | **yes** | no | Full node (live) | Re-executes transactions; **independently verifies** anchors by recomputing BPT root locally. **Validator-eligible.** |
| **COMPLETE** | yes | yes | yes | Full node (archive) | Same as ACTIVE plus serves historical queries. |

The model mirrors light-client / full-node split in other systems. BOOTING is light-client mode (signature trust + proof verification); ACTIVE upgrades to full-node mode (computational verification).

ACTIVE is **testable**: `local_bpt_root == most_recent_anchor.StateTreeAnchor`. Once true, every read-set leaf is local — validator participation has no hydration latency on the consensus path.

### 2.3. Three hydration sources, in parallel

Once the back-walk completes (proving authority) and current state is being delivered, three sources drive the BPT toward completeness simultaneously:

1. **Live traffic listener** (active prefetch). Subscribe to incoming blocks; pre-fetch any account they reference that we don't yet have. Warms the *hot* working set ahead of local need — the path that makes validator participation viable quickly.
2. **BPT enumeration consumer** (#3969). Walk the BPT page-by-page; fetch leaves we don't have. Drives the node toward full completeness regardless of traffic patterns.
3. **Passive fetch on touch.** When a local query or block-apply touches an unknown account, fetch on demand. Safety net for anything missed.

Together they reach ACTIVE faster than any one alone. New accounts created after enumeration begins are picked up by the live traffic listener, so the enumeration doesn't need a snapshot semantic.

### 2.4. State advertisement enables transparent proxying

A node publishes its current state (BOOTING / ACTIVE / COMPLETE) plus a verifiable `BptRootMatched` value to peer discovery (#3970). State advertisement drives two routing behaviors:

- **Client routing** — clients aware of node states can pick the right peer for a given query.
- **Peer-internal forwarding** — a node receiving a query it can't serve locally **transparently proxies** the request to a peer that can. From the caller's perspective, every node serves every query; the routing happens inside the network.

Concretely, an ACTIVE-but-not-COMPLETE node receiving a historical query forwards it to a COMPLETE peer (chosen from the advertised set), verifies the response (e.g., via Merkle proof against committed `StateTreeAnchor` for state queries), and returns it to the caller. There is no silent failure; the node either answers correctly or returns an explicit error if no COMPLETE peer is reachable.

Capability map:

- ACTIVE+ peers serve (or proxy on behalf of) all queries; serve current state, validator participation, and bootstrap data for new launchers directly.
- COMPLETE peers additionally serve historical queries beyond the rolling window directly.
- BOOTING peers do not appear in routable peer lists; their advertisement signals "do not route here yet."

The `BptRootMatched` value lets consumers (clients and proxying peers) spot-check ACTIVE claims by querying `GetBptLeaf` on the peer and verifying the proof anchors at the claimed root.

**Backwards compatibility (load-bearing).** Existing nodes on mainnet today predate this design and do not advertise a `BootstrapState`. The rules:

- A node with no advertised state is treated as **COMPLETE for legacy queries** — i.e., it gets rolled into the routable peer pool as a full archive node, requiring no operator action.
- A node with no advertised state **cannot serve the new bootstrap-supporting APIs** (#3957, #3958, #3969, the advertisement endpoints themselves) — it predates them.
- Bootstrapping launchers therefore require at least one peer that advertises state (and thus exposes the new APIs). This is the rollout requirement: the new APIs must be deployed to enough peers before launchers can use them.

This means existing mainnet keeps working unchanged; the new design adds capabilities to upgraded peers without disrupting un-upgraded ones.

### 2.5. New service methods extend the existing service framework

All bootstrap-supporting service methods are added to the existing v3 service framework (same pattern as `MetricsService` at `internal/api/v3/metrics.go`). JSON-RPC, REST, websocket, and P2P bindings come for free. Lite clients and explorers benefit from the same primitives.

### 2.6. The delivery bundle (#3961) carries validator signatures

A new launcher fetches a peer-served bundle that delivers current state for the minimum bootstrap set in one round trip rather than account-by-account. The bundle is **signed by the partition validator quorum at the bundle's height**. The signatures aren't load-bearing for the proof-of-derivation guarantee — that comes from the receiver-side back-walk — but they're a useful peer-attestation primitive: a client fetching from a single peer can verify the bundle without round-tripping to multiple peers.

Decision: bundles **keep** their signatures.

### 2.7. Existing sync is unchanged

Once the launcher hands off to `accumulated run`, the existing block-production / consensus / sync code drives the node forward. The bootstrap mechanism produces a node that's ready to participate; it does not replace any running-node behavior.

## 3. Architecture

### 3.1. End-to-end flow

```
              (binary contains pinned genesis snapshot hash)
                              |
                              v
       +------------- accumulated bootstrap ---------------+
       |                                                   |
       |  1. Pin block H = current_tip - confirmation_depth|
       |  2. Pull current state at H for minimum set       |
       |     (delivery bundle #3961 or per-account via API)|
       |  3. Back-walk main chains (#3960) using:          |
       |       ResolveKeyBookAt (#3957) for keypage @ time |
       |  4. Persist proof artifact (#3965)                |
       |                                                   |
       +---------------------+-----------------------------+
                             |
                             v
       +-------------- accumulated run --------------------+
       |                                                   |
       |  state = BOOTING                                  |
       |  Hydrators (#3964) running in parallel:           |
       |    - Live traffic listener -> #3958 (GetBptLeaf)  |
       |    - BPT enumeration       -> #3969 (GetBptPage)  |
       |    - Passive touch         -> #3958               |
       |                                                   |
       |  When local_bpt_root == anchor.StateTreeAnchor:   |
       |    state = ACTIVE                                 |
       |    advertisement layer (#3970) publishes ACTIVE   |
       |    validator participation enabled                |
       |    transactions now execute locally               |
       |                                                   |
       |  Optional: history backfill (#3967) running       |
       |    when retention target reached: state = COMPLETE|
       |                                                   |
       +---------------------------------------------------+
```

### 3.2. The proof artifact

A bootstrapped node persists, and can re-serve, a proof of derivation composed of:

1. **Pinned genesis snapshot hash.** Compiled into the binary.
2. **Validated graph.** For each main chain back-walked: every entry that was verified, its block time, the signer's URL, the verification result. Memoization records keyed by `(account, block_time)` (cached keypage-at-time resolutions). Genesis-termination markers showing which earliest entries reference the genesis snapshot.
3. **Pulled current state at H.** The minimum bootstrap set as captured at the pinned block.
4. **Anchor stream from H forward.** Each anchor's signatures verified against the validator set at its block time (resolved via the same back-walk machinery).

Persistence is forward-compatible (versioned envelope, reject unknown major versions, tolerate unknown minor versions). On binary upgrade, a pin-hash mismatch aborts startup unless explicit migration is run.

### 3.3. Genesis termination

Keybook creation at genesis bypasses transaction signing — `createOperatorBook()` (`internal/node/genesis/bootstrap.go`) writes records directly without a signed creation transaction. The recursion bottoms out by **verifying that the chain's earliest entry references an account / keybook present in the genesis snapshot at the pinned hash**, not by reaching a signed entry. The pinned trust artifact is the hash of the genesis *snapshot*, not a single block hash.

### 3.4. Two verification rules

The back-walker handles both kinds of authentication present on Accumulate's chains:

- **User-signed entries.** Signatures live on the *signer's* signature chain (`batch.Account(signerUrl).Transaction(txn_hash).Signatures()`), not on the destination's main chain. Each verification step laterally navigates from a main-chain entry to the signer's account to fetch signatures, then resolves the signing keypage at the entry's block time, then recurses on the signing keybook's own main chain.
- **Synthetic entries.** Cross-partition forwards, anchor results, and other protocol-produced transactions carry only an `InternalSignature` — metadata with `Cause` and `TransactionHash` but no cryptographic material. Verification: trace `Cause` to the producing transaction; recurse on it; additionally verify the synthetic was included in a validator-quorum-signed anchor. The validator set at that block time is itself resolved via `ResolveKeyBookAt` against the operators / partition keybook.

The back-walk is therefore a **graph traversal** across multiple accounts, not a single-chain rewind. Nodes are `(account, transaction, block_time)` tuples; edges follow signature dependencies. Memoization keyed by `(account, block_time)` handles cyclic dependencies between keybooks (legitimate when keybooks mutually sign each other's key changes).

### 3.5. BPT structure dense; account data may be unloaded

The BPT structure (every `(key_hash, value_hash)` leaf) is filled **first**, before any account data is collected. The launcher fetches BPT pages via `GetBptPage` (#3969) until the entire keyspace is enumerated and inserts each leaf entry into the local BPT. There are **no placeholder leaves** in the BPT; every leaf is fully present as a `(key, value_hash)` pair.

Account *data* (the serialized state that hashes to a leaf's `value_hash`) is then collected separately. Each account is fetched via `GetBptLeaf` (#3958) or per-account queries; the launcher verifies the fetched data hashes to the matching leaf's `value_hash` and stores it.

Two-stage completeness:

- **BPT-structure complete.** Local BPT root matches the most recent anchor's `StateTreeAnchor`. Cryptographic completeness of the *structure* — verifiable, testable, but no transaction execution possible yet.
- **All accounts loaded (= ACTIVE).** Every account behind every leaf is loaded. Computational completeness — transaction execution becomes possible; node may participate as a validator.

Read semantics during BOOTING:

- Reading a leaf's `value_hash` from the BPT always succeeds (structure is dense).
- Reading the *account* behind a leaf returns the account data if loaded, or "not yet loaded" if not — at which point the read either queues a fetch and stalls the caller, or returns the not-loaded indicator depending on context.

After ACTIVE: every account is loaded; "not yet loaded" should not be observed. If it is, that's a bug. The BPT structure continues to mutate via local execution (new leaves on account creation, leaf-hash changes on state change).

The BPT structure during BOOTING also evolves: as live blocks arrive, anchors carry new `StateTreeAnchor` values; the launcher fetches any newly-changed leaves and updates the local structure to match. Trust model unchanged: the BPT root commitment from validators is taken as authoritative; the launcher fetches matching leaves and verifies hash chain.

### 3.6. Block-time lookup

Every main-chain entry needs its block timestamp for `ResolveKeyBookAt`. Empirical probing showed the v3 API's `QueryChainEntries(... Expand: true)` already returns `LastBlockTime` per entry, so the back-walker piggybacks on the queries it's already making — no new method required. (#3968 remains deferred for non-walking consumers.)

### 3.7. Confirmation depth

Bootstrap pins block H at `current_tip - confirmation_depth`, **not** the live tip. Today's CometBFT semantics late-commit anchors: an anchor is only durably committed when the *following* block commits. Pulling from the live tip risks pinning to an anchor that gets rolled back. Confirmation depth of at least 1 is required for CometBFT; the exact default is an open question (probably 2–3 to be safe). Revisit when DAG-BFT lands.

### 3.8. Account-loading API shape (open)

The existing `pkg/database/bpt/` API and account access path assume the account is present. Introducing "loaded vs. unloaded" semantics affects the read path. Two approaches:

1. **Extend the existing account API** with load-state-aware reads. Less code, but every read site must understand "not yet loaded" semantics.
2. **Separate loading layer** that wraps the standard account access, traps misses, queues fetches, and returns the loaded-state result. Cleaner isolation; read sites in execution paths don't see the loading machinery.

Decision deferred to implementation start — pick when scaffolding #3962.

## 4. Trust model

The only out-of-band trust input is the pinned genesis snapshot hash compiled into the `accumulated` binary. Everything else is derived locally:

- **Validators trusted to sign anchors:** their authority is verified by back-walking the operators keybook to genesis. The pinned hash anchors that back-walk.
- **State at any past block:** trusted because the validator quorum at that block (verified above) signed off on its `StateTreeAnchor`. The sparse-BPT leaves we hydrate are individually proof-checked against that root.
- **State at the pinned block H:** trusted via the Merkle path to H's anchor, which itself is signed by validators verified above.
- **State at the live tip (during BOOTING):** trusted by the same chain — each new anchor's signatures verified against the back-walked authority. The node accepts the network's signed `StateTreeAnchor` as the authoritative current-root claim and fetches matching leaves toward it.
- **State at the live tip (after ACTIVE):** trusted by *local re-execution*. The node no longer trusts signatures alone for state correctness; it computes the next root and matches it. This is the upgrade BOOTING → ACTIVE represents.

Threats it defends against:
- Single peer feeding incorrect state: detected because the back-walk to genesis fails or because BPT leaves don't hash to their committed values.
- A future validator quorum colluding to commit invalid state: detected only at ACTIVE (when local computation diverges). During BOOTING, this remains a pure trust dependency on the validator quorum — the same dependency every existing Accumulate node has.
- Binary tampering: out of scope; pinned hash is only as trustworthy as the binary itself.

## 5. Bootstrap algorithm

### 5.1. Phase 1 — BOOTING (interactive)

1. **Pin block H** at `current_tip - confirmation_depth`. All initial state and main-chain entries are pulled as they existed at H.
2. **Pull current state at H** for the minimum bootstrap set (§5.4). Use the delivery bundle (#3961) when available, fall back to per-account queries.
3. **Back-walk** the keybooks and in-set accounts as a graph traversal (§3.4). Memoize by `(account, block_time)`. Terminate at genesis snapshot.
4. **Fill the BPT structure.** Walk every BPT page via #3969; insert each `(key_hash, value_hash)` into the local BPT. Continue until the local BPT root matches the anchor's `StateTreeAnchor` at H.
5. **Persist** the proof artifact (#3965) before handing off. Node state = BOOTING (BPT-structure complete; account data still loading).
6. **Hand off** to `accumulated run`.

### 5.2. Phase 2 — BOOTING → ACTIVE (background)

After handoff, the existing run loop receives blocks normally. The BPT structure stays current — live anchors deliver new `StateTreeAnchor` values; the launcher fetches any newly-changed leaves to keep the structure aligned. Three hydrators run concurrently to fill in **account data** behind known leaves:

- **Live traffic listener.** Subscribes to live blocks; for each transaction, identifies referenced accounts and queues their data for fetch. Warms the hot working set ahead of need.
- **BPT enumeration consumer.** Walks the BPT (now locally complete in structure) and fetches account data for any leaf whose account isn't yet loaded. Drives systematic completeness.
- **Passive touch.** Local code paths that hit a not-yet-loaded account queue a fetch.

Each fetched account is verified to hash to the matching leaf's `value_hash` before being stored.

When every account behind every leaf is loaded:

- State transitions to ACTIVE (persisted by #3965).
- Advertisement layer (#3970) publishes the new state.
- Validator participation becomes eligible.
- Transaction execution path activates: incoming blocks now apply locally and the node verifies anchors by computation.

### 5.3. Phase 3 — ACTIVE → COMPLETE (background, optional)

History backfill (#3967) pulls older blocks and transactions until the configured retention is filled. State transitions to COMPLETE; advertisement updates.

### 5.4. Minimum bootstrap set

Before `accumulated run` can take over, the launcher must have *full pulled state* and *completed back-walks* for these accounts:

- DN's `Network` account (partition list, network globals).
- Operator key book(s) for the DN and the partition the node will participate in.
- Anchor ledger(s) for that partition (and the DN if the node is in the DN).
- Synthetic transaction ledger(s).
- System ledger(s).
- Any account explicitly named in the bootstrap config.

Everything else is sparse — accessible via lazy hydration after handoff.

## 6. Components

Each maps to a GitLab issue. Refine descriptions there as implementation begins.

### 6.1. Service methods (new on existing service framework)

| Issue | Method | Purpose |
|---|---|---|
| #3957 | `ResolveKeyBookAt(url, block_time)` | Central back-walk primitive. Resolves a keybook (or partition validator set) at a given block time by walking its main chain. Cache aggressively. |
| #3958 | `GetBptLeaf(key)` | Returns leaf + Merkle proof against current BPT root. Used by all three hydrators. |
| #3969 | `GetBptPage(start_hash, count)` | Paginated enumeration of BPT leaves by hash range. Drives systematic completeness. |
| #3954 | `GetTrustAnchor` | Peer-served delivery bundle. Convenience; not load-bearing for proof. |
| #3955 | `GetAnchor(major, minor)` | Reduced-priority — inclusion proofs / lite clients. |
| #3956 | `GetAnchorsRange(from, to)` | Reduced-priority — lite-client streaming. |

### 6.2. Foundations

| Issue | Component | Purpose |
|---|---|---|
| #3960 | Back-walking main-chain validator | Constructs the proof of derivation as a graph traversal; emits the persistable artifact. |
| #3961 | Current-state delivery bundle | Peer-served bundle of current state + validator-quorum signatures for one-round-trip bootstrap. |
| #3962 | Sparse BPT | Placeholder/hydrated leaves; mode-dependent read semantics. |
| #3964 | Background BPT hydrator | Multi-source: traffic listener + BPT enumeration consumer + passive touch. Signals BOOTING → ACTIVE. |
| #3965 | Persistence and restart | Persists the proof, the sparse BPT, and the node state machine. Forward-compatible schema. |
| #3970 | Node-state advertisement | Publishes BOOTING / ACTIVE / COMPLETE with `BptRootMatched`; clients route by capability. |

### 6.3. Wiring

| Issue | Component |
|---|---|
| #3959 | `accumulated bootstrap` subcommand (config file + interactive prompts) |
| #3967 | Background history backfill (optional, post-bootstrap) |

### 6.4. Closed (superseded)

| Issue | Why closed |
|---|---|
| #3963 | No forward replay needed under back-walk model. |
| #3966 | No forward phase ordering needed (DN-first sync) under back-walk model. |
| #3968 | Per-entry `LastBlockTime` already returned by existing API; back-walker uses it directly. (Kept open as deferred for non-walking consumers.) |

## 7. Recommended implementation order

1. **`ResolveKeyBookAt` (#3957).** Smallest standalone primitive, well-bounded. Server-side keybook main-chain walker with memoization. Bindings for JSON-RPC / REST / websocket / P2P stamp out from the existing pattern.
2. **Back-walking main-chain validator (#3960).** Uses #3957. Produces the proof-of-derivation artifact end-to-end. Testable in isolation against mainnet (the `cmd/backwalk-probe` prototype validates the API surface).
3. **Sparse BPT (#3962) + `GetBptLeaf` (#3958) + `GetBptPage` (#3969).** Current-state hydration primitives. The sparse-BPT API shape decision (§3.8) gates this.
4. **Background hydrator (#3964).** Three sources running concurrently. Drives BOOTING → ACTIVE.
5. **Persistence (#3965).** Node-state machine, proof artifact, sparse BPT.
6. **Node-state advertisement (#3970).** Without this, ACTIVE peers can't be safely routed to. Hard prerequisite for #3959's `target-state = ACTIVE` exit semantic.
7. **`accumulated bootstrap` subcommand (#3959).** Wires it all together.
8. **Background history backfill (#3967).** Drives ACTIVE → COMPLETE; optional.

Steps 1–2 are the highest-value standalone work — they produce a usable artifact (the back-walking validator) before any of the bigger pieces are touched. They also de-risk the design empirically.

## 8. Empirical findings

A throwaway prototype (`cmd/backwalk-probe/`) was run against `https://mainnet.accumulatenetwork.io/v3` to validate the design's load-bearing assumptions. Findings:

- **System account main chains are very short.** `dn.acme/operators`, `dn.acme/operators/1`, `dn.acme/network`, `dn.acme/ledger` each have **1** main-chain entry — not mutated since genesis. The operator keybook back-walk on mainnet today is a one-step lookup.
- **The interesting bulk is on `dn.acme/anchors`.** 20,438 main-chain entries (anchor transactions); 20,437 signature-chain entries (`*messaging.BlockAnchor` messages with validator quorum signatures embedded). All anchor txns are synthetic (`Cause` populated).
- **Storage asymmetry confirmed.** User keypage signatures come back via the API's `Signatures` expansion. Validator quorum signatures live *inside* the `*messaging.BlockAnchor` message body — the back-walker needs both extraction paths.
- **Walk speed.** ~4 ms per entry over JSON-RPC with `Expand: true`. 500 entries in 1.92 s. Linear extrapolation: ~80 s for 20,438 anchor entries. A full back-walk of all relevant chains over a remote API: **5–15 minutes** estimated. Local-database walks (post-handoff) would be far faster.
- **Per-partition routing required.** BVN-local accounts return empty when queried via the DN endpoint. The launcher must address each partition's API endpoint directly. Tracked in #3959.

These findings support the design as specified. The prototype is left at `cmd/backwalk-probe/main.go` for follow-up probing.

## 9. Open questions

These are not blockers for starting implementation but should be resolved before the corresponding component lands:

- **Sparse-BPT API shape (#3962).** Extend existing batch / database vs. separate `SparseBatch` type. §3.8.
- **Confirmation depth default.** Probably 2–3 for CometBFT; revisit for DAG-BFT.
- **Heartbeat interval and TTL** for advertisements (#3970).
- **Whether `accumulated bootstrap` chains into `run` automatically** or exits with a hint. Probably auto-chain for the common case, with a flag to opt out.
- **Bundle generation cadence for #3961.** On-demand vs. every-major-block.
- **Whether #3955 / #3956 survive** if no concrete consumer materializes; close if not.

## 10. Out of scope

- **Pre-July-2025 chain reorganization.** Main chains span the July 13, 2025 database reorg. Initial design assumes continuous chains; the reorg is handled as a follow-up.
- **DAG-BFT migration.** Anchor structure and finality semantics may shift. Adjustments handled when DAG-BFT lands.
- **Validator-mode bootstrap before ACTIVE.** Initial design gates validator participation on ACTIVE; pre-ACTIVE validator participation is not in scope.
- **Forward derivation alternatives.** Rejected — see §2.1.
- **Migrating an existing full node to bootstrapped sparse state.** Bootstrap is for new nodes; existing nodes retain their full state.
- **Cross-version migration of the proof artifact format.** Out of scope for v1; revisit when format evolves.

---

## Appendix A: Reframings during design

The design went through several reframings before settling. Recording them here so reviewers see the path:

1. **Forward-walk → back-walk.** The first sketch derived state forward from genesis by walking anchors and applying `Updates`. Replaced with backward verification when it became clear the proof is purely a verification activity, not a state-derivation activity.
2. **Single trust anchor → genesis snapshot hash.** The pinned trust input is the hash of the genesis snapshot, not a single block hash. Genesis records bypass transaction signing; recursion bottoms out by snapshot membership.
3. **Passive lazy hydration → multi-source hydrator.** Adding the live traffic listener and BPT enumeration alongside passive touch cuts the time to ACTIVE substantially and re-enables validator-mode bootstrap.
4. **Observer-only → three-state machine.** Validators can come online at ACTIVE rather than waiting for COMPLETE. State advertisement (#3970) makes this safe by routing historical queries away from ACTIVE-but-not-COMPLETE peers.
5. **BPT-as-dense → sparse with explicit BOOTING-receiver semantics.** During BOOTING the BPT only grows via verified fetches; transactions don't execute locally until ACTIVE. Trust is by signature; verification by computation only after the upgrade.

## Appendix B: References

- Tracking issue: #3953.
- Children: #3954, #3955, #3956, #3957, #3958, #3959, #3960, #3961, #3962, #3964, #3965, #3967, #3968, #3969, #3970.
- Closed: #3963, #3966.
- Prototype: `cmd/backwalk-probe/main.go`.
- Related verifications:
  - Anchor signature storage: signatures live on the destination account's signature chain (`internal/core/execute/v2/block/sig_common.go:216-254`).
  - Synthetic transaction authentication: `InternalSignature` carries no crypto; auth by inclusion in validator-signed anchor (`protocol/types_gen.go`, `protocol/signature.go`).
  - Genesis termination: keybook creation bypasses transaction signing (`internal/node/genesis/bootstrap.go:423-442`).
  - Block timestamps: block-level, returned per chain entry on the v3 API.
