# Changelog

## 1.4.6.4

Fixes a wire-compatibility break in 1.4.6.3. **1.4.6.3 should not be
deployed.**

- Transaction header (#4096)
  - `HashLock`, added by HTLC in 1.4.6.3, was inserted between `HoldUntil` and
    `Authorities`. Field numbers are positional, so `HashLock` took field 7 —
    the number `Authorities` has held since #3457 — and `Authorities` moved to
    8.
  - A transaction header written by any earlier release, with additional
    authorities set, therefore failed to decode on 1.4.6.3:
    `field HashLock: failed to unmarshal value: failed to read field number:
    field number is invalid`. The same logical transaction also hashed
    differently, which splits a mixed-version network and invalidates
    signatures made over the old hash.
  - This was live, not theoretical: additional authorities are gated behind
    `v2-baikonur` and `v2-vandenberg`, and mainnet runs `v2-vandenberg`.
  - `HashLock` now takes field 8 and `Authorities` returns to 7. Headers
    written by 1.4.6.2 decode unchanged and transaction hashes match it
    exactly. No migration is required, because no released binary ever wrote a
    `HashLock` at field 7 — the field is new in 1.4.6.3.
  - Golden-byte tests now pin the pre-HashLock encoding and the transaction
    hash, as literals rather than values recomputed from the code under test.
  - Every generated type in the repository was compared field by field
    between 1.4.6.2 and 1.4.6.3 — 319 types, this was the only
    incompatibility.

## 1.4.6.3

Fifty-six commits since 1.4.6.2, against two to five in each of the releases
before it, and it defines a new executor version — which would normally argue
for a minor bump. It stays in the 1.4.6.x series deliberately: this release is
the one that gets proven in deployment first, and 1.4.7 is cut once it has.

`v2-kourou` is activated on no network, so everything gated behind it is inert
until a network activates it, and the rest is a fix to behaviour already live —
**with one exception you must read before deploying.** HTLC ships its two new
transaction types with no activation height. See the HTLC entry below.

- Cross-partition recovery (#4087, #4089, #4090)
  - **New executor version `v2-kourou` (9)**, gating collection proofs. It is
    defined and inert — activated on no network — so this release changes no
    behaviour until a network activates it.
  - Recovery is proven against a root the destination ALREADY HOLDS rather than
    one the directory has caught up to. Holding a validated state of a chain
    proves every entry added before it, so one anchor a destination already
    trusts proves any earlier run of that partition's messages by replay; the
    source returns the messages and the merkle state to replay them onto, and is
    never asked to prove anything.
  - This removes the directory from the recovery path, which is what makes
    recovery work while the network is behind — the only time it is needed. The
    previous construction rooted proofs at a directory-receipted root, so
    recovering anchor N required N's own block to have been anchored to the DN,
    and the anchor that would have carried it there was the missing one.
  - Requesters NAME the anchor they hold (`SequenceOptions.ProveAgainstAnchor`),
    so a requester can never be handed a proof it cannot check.
  - Synthetic emission sends one collection proof per package instead of one
    receipt per message (#4090), batching per destination under a 3 MiB budget
    against the 4 MiB `max_tx_bytes`. Groups below two messages keep the
    per-message form, where a list would be no smaller.
  - The source-side anchor push is deliberately KEPT: the pull sees a hole
    because a later message exposes it, and cannot see a tail.
  - **Known limitation:** BVN→BVN streams still route through the directory by
    construction, because partitions hold no anchors from each other. #4086 is
    dissolved for directory-adjacent streams and only mitigated for BVN→BVN.
  - Fixes a goroutine leak in `getPeers` (#4089, duplicate of #4085): every
    successful peer discovery left a goroutine blocked in `chansend` forever,
    because the send was outside the select and neither the timeout nor the
    context could reach it. Measured: one node reached 10,003 goroutines in 6.5h
    before the fix, 473–572 across all twelve nodes over 22.6h after.
  - Validated by a 22.6h chaos soak at `v2-kourou`: 153,533 transactions through
    123 chaos events, 913 anchor and 807 synthetic range pulls plus 385
    reconcile pulls against 3,954 and 551 induced drops, zero wedged streams,
    zero stranded transactions, zero heal errors.
- Staking (#4079 and the requests package)
  - New `pkg/staking/requests`: one implementation of the staking-requests
    vocabulary for the wallet (which writes) and the ASP staking app (which
    reads and fulfils). It exists because the vocabulary was implemented twice
    and disagreed — core/staking#449 rendered contract entries "informational",
    core/wallet#272 called pre-contract entries "refused".
  - `Parse` reads every era on chain because the chain is immutable; `Encode`
    writes only the current contract because a malformed request is accepted,
    billed, and never fulfilled. `Validate` refuses entries that parse but must
    not be acted on — unknown fields, and now multi-payload entries, whose
    fulfilments would be indistinguishable because they share an entry hash.
  - Key books, delegation and side keys documented and backed by executor tests.
- HTLC — hashed time-locked contracts (AIP-48, #3717)
  - Funds lock against `H(secret)` and are claimable only by revealing a
    preimage, with an automatic refund if nobody claims before expiry. This is
    the primitive behind atomic swaps: claiming publishes the secret the
    counterparty needs to claim their side.
  - `SendTokens` gains an optional `HashLock`, which produces a
    **`SyntheticLockedDeposit`** (`0x37`) instead of a normal deposit;
    **`ReleaseLockedOperation`** (`0x18`) claims it with `LockedTxID` and
    `Preimage`. SHA256, SHA256D and HASH160 — the set that makes it
    interoperable with Bitcoin-style counterparties.
  - **NOT version-gated, deliberately.** These are two consensus-visible
    transaction types with no activation height, so they go live the moment a
    node upgrades. On a multi-validator network that is a divergence risk: an
    upgraded node accepts a `ReleaseLockedOperation` a lagging node rejects.
    Accepted because the target network currently runs a single validator,
    which cannot disagree with itself. **Revisit before a second validator
    joins at a different version, or before this reaches a network that has
    one.**
  - Delivered by cherry-picking the five feature commits out of !1158 rather
    than merging it: that branch is 1,485 files and +128,315 lines, almost all
    of it re-adding an older tree, and its other genuine improvements are
    already on main by other paths.
- Networking and deployment (#4091, #4092, #4078, #4081, #4085)
  - Ask the peer tracker what we already know before querying the DHT (#4085).
    Every `RoundTrip` issued a provider lookup: 496 per second on an idle
    six-node network, 331k failed DHT requests in three minutes. The dialer
    already checked the tracker for known-good peers, but that check ran after
    `Discover`, so the lookup was paid for and its result abandoned. The check
    is hoisted above the query, and the tracker is consulted when it has any
    known-good peer rather than four — a partition served by two nodes could
    never reach four, so small networks queried the DHT on every dial forever.
    Measured on the same network: 496/s → 223/s → 124/s. This is the source of
    the kad-dht stream growth whose goroutine leak was fixed as #4089; the
    OOM's remaining causes (no restart policy, mempool exhaustion) stay open
    under #4085.
  - Stop advertising addresses a remote caller cannot dial.
  - Resolve bootstrap peers from the network's URL.
  - Devnet: followers are no longer voting validators; peers are built at the
    Accumulate-P2P port so consensus forms; each partition gets its own port.
- Security (#4026, #4033, #4034, #4039)
  - Bootstrap info server hardened.
- Healing (#4070)
  - A node declines to sequence unless it validates the source partition.
- Testing and tooling
  - Mixed-workload synthetic coverage, a mainnet-shape harness at Vandenberg,
    pprof overlay and an OOM diagnosis script, a two-arm drop harness, and a
    no-fault baseline mode for the synth-heal harness.
  - The soak harness runs at `v2-kourou`, `ACC_DEBUG_DROP_ANCHOR` actually drops
    anchors (it wrapped the wrong dispatcher and was a silent no-op), and the
    soak monitor survives heal types it has never heard of.
  - **Not covered by any soak:** #4090's one-proof-per-package emission merged
    after the 22.6h run started. It has e2e coverage only.

## 1.4.6.2

- Testing (#4076)
  - `TestVersionSwitch` no longer fails a few percent of runs, which is what
    failed the `go test 1/2` job of the v1.4.6.1 release pipeline. Activating a
    protocol version normally converges in ~17 simulator steps but sometimes
    takes exactly 75, and `StepUntil`'s default budget of 50 fell between the
    two modes. How many steps an activation takes is not part of what the test
    asserts, so the budget is now a hang guard rather than a number tuned to
    observed performance. Measured cost of the generous budget when the test is
    genuinely broken: 1.09s.
  - The underlying cause is unfixed and tracked by #4076: simulator message
    ordering is not deterministic, because conductor background tasks run
    concurrently, `Simulator.Step` does not honour the `Deterministic()` option,
    and `orderMessagesDeterministically` is never called outside its own unit
    test.

## 1.4.6.1

- API (#4074)
  - Fix v3 `includeReceipt` queries hanging indefinitely for chain entries with
    old anchors. `SearchIndexChain` walked index chains linearly from the newest
    entry, so receipts for old entries scanned the entire root index chain:
    requests hung (HDD) or took minutes (SSD), ballooned node memory by tens of
    GB, and kept computing after the client disconnected — a wedge-the-node
    vector on any public endpoint. Index chains are ordered, so the search is
    now a binary search (O(log n) reads); semantics are verified unchanged by a
    property test against the previous implementation.

## 1.4.4.2

- Healing (#4064)
  - In-node receiver-pull synthetic healing: a stalled inbound stream pulls the
    missing message from the source partition and the pending tail drains.
    Jittered (10s) check-then-fire keeps N validators to ~one pull. Healed
    `MessageForTransaction` synthetics bundle the companion transaction (#4066).
  - Anchor healing is config-exposed (`enable-anchor-healing`; safe and
    sufficient on single-validator networks).
  - Both default **off**; enable per node with `enable-synthetic-healing` /
    `enable-anchor-healing` in the `coreValidator` configuration.
  - Observability: `ConsensusStatus.syntheticHeals`/`anchorHeals` and the
    `accumulate_crosschain_heals_total` metric (counts heal attempts).
- Deployment fixes (#4065)
  - `DiscoveryMode` is passed through to the p2p node; with
    `discovery-mode = "auto-server"` service discovery works on private
    multi-node networks.
  - `init network` emits bootstrap peers at the AccumulateP2P port (+2),
    matching the CometBFT address conversion — multi-node networks form
    consensus (previously dialed peers two ports low).
- Build
  - Docker image builds again: `golang:1.25` base (go.mod requires 1.24,
    dlv@latest requires 1.25); image includes `accumulated-http`.
- Testing
  - Self-contained docker deployment test (`test/docker/synth-heal`): per-node
    containers + own bootstrap, 2 validators per partition, proves wedge →
    receiver-pull heal on real transport. `ACC_DEBUG_DROP_SYNTHETIC` debug
    hook (no-op unless set) deterministically drops synthetics.
  - e2e healing tests incl. red/green companion-transaction case.


## 1.4

- Protocol
  - Support for EIP-712 (typed data) signatures
  - Support for PKIX ECDSA signatures and AC2/AS2 addresses
  - Support for PKIX RSA signatures and AC3/AS3 addresses
    - **NOTE**, an RSA or ECDSA lite account cannot bootstrap due to [an issue
      with credits][aip-51].
  - Support for WIF addresses
  - Suggested transactions
  - Discount creation of bare ADIs (without a key book)
  - Discount creation of sub-ADIs
- API
  - Support for retrieving a receipt at a specific height
  - Support appending an epilogue to a transaction
  - Support minimum viable Ethereum RPC for MetaMask
- Node operations
  - New and improved node configuration framework
  - Progress towards snapshot sync
  - Support adding an index to a snapshot
  - Support for Bolt and LevelDB
  - Exploratory custom database implementation to improve TPS
- Network operations
  - Improve peer-to-peer service discovery
  - Improve reliability of healing
  - Improve performance of light client indexing
  - Decouple ACME burns (for credits) from anchors
  - Decouple network updates (e.g. the oracle) from anchors
  - Decouple major blocks from anchors
  - Improve reliability of major blocks
  - Improve reliability of anchoring
  - Improve API stability
- Other
  - Reduce overhead of BPT hash calculations
  - Redesign the simulator's consensus model
  - Fix lite account authority handling bugs

[aip-51]: https://gitlab.com/accumulatenetwork/governance/aip/-/issues/51

## 1.3

- Protocol
  - User-specified transaction expiration
  - Rejection of invalid authorities
  - Dynamic inheritance of authorities
  - Additional transaction authorities
  - Reduced cost for creating sub-ADIs
  - Memos and metadata for signatures
  - RSV Ethereum signatures (deprecates DER)
  - Database performance improvements
  - Prevent persistence of bad blocks
  - Bug fixes
- Operations
  - Anchoring improvements
  - Enable snapshot v2
  - Use binary genesis file for new nodes
- SDK
  - Embedded checkpoint for validating network state

## 1.2.10

- API
  - Query responses now include `lastBlockTime`, which is retrieved from the
    consensus layer. This can be used to ensure the response is up to date.
  - Sub-records of query responses (those that have sub-records) now may be
    replaced with an error record, if the sub-record cannot not be found. This
    prevents the entire query from failing if a sub-record cannot be found.
  - Service discovery methods were tweaked to facilitate routing improvements.
  - Refactored request routing to improve API stability.
  - Added a REST API.
- Operations
  - Improved Prometheus metrics.
  - Improved snapshot performance.
  - Improved dispatch for anchors and synthetic transactions.
  - Fixed a bug with the Badger database driver.

## 1.2

- Signature processing overhaul
- Rejections and abstentions
- Transaction review periods
- Snapshot v2
- API improvements

## 1.1.3

- Fixes a bug that can lead to unresolvable synthetic transactions (#3351, !865)

## 1.1 (.2)

1.1.0 was retracted due to a consensus bug. 1.1.1 was retracted due to user
error (the wrong commit was tagged).

### Configuration changes

```toml
##### Required (DNN only) #####

# Add a new section
[p2p]
  # Bootstrap peers for connecting to Accumulate's libp2p network
  bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg",
  ]
  # API v3/libp2p listening addresses
  listen = [
    "/ip4/0.0.0.0/tcp/16593",
    "/ip4/0.0.0.0/udp/16593/quic",
  ]

##### Recommended (DNN and BVNN) #####

# Update the existing section
[snapshots]
  # Disable snapshot collection to reduce resource consumption
  enable = false
```

### API

- For certain types, zero-valued fields will be omitted from JSON output instead
  of being returned as null or zero.
  - `sendTokens.hash`, `signature.transactionHash`, `tokenIssuer.issued`,
    `dataAccount.entry`

## 1.0.4

- Replace Accumulate data entries with double hash data entries and reject
  transactions with bodies that are exactly 64 bytes to resolve a potential
  weakness in the security of communications between network partitions (#3283,
  !810)

## 1.0.3

- Allow the latest protocol version to be reactivated (#3228, !754)

## 1.0.2

- Implement versioning of the core executor code (#3152, !684)
  - Fixes a bug where the version change network update is not published to BVNs
  - Logs an error if multiple database batches concurrently change the same
    value
- Anchors signature chains into the root chain (#3149, !681)
- Miscellaneous fixes and changes (#3154, !685)
  - Fixes an issue with recording signatures
  - Fixes improper forwarding of synthetic transactions
  - Allows updates to the authority set of network accounts
  - Fixes an issue with recording the transaction initiator
  - Rejects malformed envelopes
  - Stops adding empty burns to the ACME token issuer
- Fixes a bug that could lead to global consensus failure due to a faulty error
  message (#3157, !689)

## 1.0.1

- Fix bugs in the SDK (617ff4673919aa0f17596ba2702ee075daca4a3c, 95694666ef9d562497bd43cbb9473533170f9be4)
- Fix error reporting in the ABCI (#49, !657)
- Add a way to determine the status of remote multisig transactions (#50, !658)
