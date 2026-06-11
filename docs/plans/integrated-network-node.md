# Integrated network node — fold bootstrap + gateway into `accumulated`

Status: draft / design
Date: 2026-05-21

## 1. Problem

The network is deployed as three separate binaries:

| Binary | Role |
|---|---|
| `accumulated` | Validators and followers (consensus). |
| `accumulated-bootstrap` | Bootstrap / peer-discovery node. |
| `accumulated-http` | HTTP / JSON-RPC API gateway. |

Each is a separate build artifact, version, and deployment procedure. Worse, network
topology lives in static, hand-edited places:

- The bootstrap multiaddr is compiled into `pkg/accumulate/api.go:18`.
- The gateway peer map is a static JSON file (`accumulated-http --peers`, added in v1.4.4.1).

When a validator is added or removed, or a node changes IP, someone edits files and
redeploys. There is no single binary, and the network does not manage its own topology.

### Goal

One binary — `accumulated` — whose role is selected by configuration. It can be a
validator, follower, gateway, bootstrap node, or any combination. Nodes discover each
other and track topology changes (validator set, IP/multiaddr) without hand-edited
config or redeploys.

## 2. Current architecture (what we build on)

`cmd/accumulated/run/` is already a declarative, config-driven framework:

- `Config` holds `Configurations []Configuration`, `Services []Service`, a shared
  `P2P` block, logging, and instrumentation.
- A **`Configuration`** (`types.go:33`) is a macro: `Type()` + `apply(inst, cfg)`.
  `apply` mutates the `Config` — adding services and setting P2P defaults. Existing
  implementations: `CoreValidatorConfiguration`, `GatewayConfiguration`,
  `FollowerConfiguration`, `DevnetConfiguration`.
- A **`Service`** (`types.go:38`) is a running component: `Type()` + `start(inst)` +
  `ioc.Factory`. Existing services: `Storage`, `Consensus`, `Network`, `Metrics`,
  `Events`, `Http`, `Router`, `Faucet`, `Subnode`.
- All services share **one libp2p host** via `inst.P2P()`.

Two consequences matter here:

1. **`GatewayConfiguration` already exists** (`run/gateway.go`). It wires a `P2P`
   block + `HttpService` + `RouterService`. `accumulated-http` is largely a
   standalone re-implementation of something the framework already does.
2. **The bootstrap components are *not* in the framework.** `InfoServer`,
   `ConnectionManager`, `ActiveDiscovery`, and `PartitionTracker` live in
   `package main` of `cmd/accumulated-bootstrap/`. The bootstrap binary's `run()`
   hand-wires them against its own libp2p host. The `InfoServer` already serves a
   live topology view: `/peers`, `/peers/{partition}`, `/partitions`, `/connect`,
   `/stats`, `/connections`, `/debug/dht`.

So integration is mostly *relocation into the existing framework*, not new
infrastructure.

## 3. Target architecture

### 3.1 New `BootstrapService`

Move `InfoServer`, `ConnectionManager`, `ActiveDiscovery`, and `PartitionTracker`
from `cmd/accumulated-bootstrap/` into `cmd/accumulated/run/` and expose them as a
`BootstrapService` implementing the `Service` interface.

Critically, the service binds to the **instance's shared P2P host** (`inst.P2P()`)
instead of creating its own. That is the integration win: a validator or follower
already runs a libp2p host; adding bootstrap duties is just attaching the discovery
and info components to it.

`BootstrapService.start(inst)`:
- requires `P2P.DiscoveryMode == ModeAutoServer` (DHT server mode);
- starts the info HTTP server (default `:8080`);
- starts `ConnectionManager` and `ActiveDiscovery` against `inst.P2P().Host()` and
  `inst.P2P().DHT()`.

#### Responsibilities

The bootstrap server has four jobs, and only these four:

1. **Collect peers** — track the libp2p peers it discovers via the DHT and the
   CometBFT endpoints they advertise (§4.7).
2. **Liveness** — periodically probe each tracked peer and mark it alive or dead;
   a peer that stops answering ages out. The version probe (below) is the liveness
   signal.
3. **Pull versions** — query each peer's running `accumulated` build version.
4. **Report version consensus** — expose `/consensus/{partition}` answering "are all
   of this partition's validators running the same version?" (§4.8). This is the gate
   operators check before activating a consensus-critical protocol upgrade.

A peer's partition is taken from authoritative data only: the node's own
advertisement (§4.7, authenticated by the libp2p handshake) and, for validators, the
on-chain validator set (§4.1). There is **no** "unknown partition" bucket and **no**
protocol-ID partition guesser — both were speculative scaffolding (introduced by an
AI-generated "optimize the bootstrap server" commit, never specified, contradicting
§4.1's "membership is on-chain") and are removed.

### 3.2 New `BootstrapConfiguration`

A macro, parallel to `GatewayConfiguration`. Its `apply()`:
- sets `P2P.DiscoveryMode = ModeAutoServer`;
- adds a `BootstrapService` with listen defaults.

A node config then declares its role with one block:

```toml
[[Configurations]]
type = "bootstrap"
```

### 3.3 Gateway: collapse `accumulated-http` into `GatewayConfiguration`

`GatewayConfiguration` already produces the P2P + HTTP + Router stack. Fold the
remaining `accumulated-http`-only behavior (peer-map handling, CORS, TLS / Let's
Encrypt, connection limits) into `GatewayConfiguration` fields and the existing
`HttpService`. The standalone binary is then redundant.

### 3.4 Schema changes

`schema.yml` drives generated code. Add `BootstrapService` to `ServiceType` and
`BootstrapConfiguration` to `ConfigurationType`, add the new config fields, then
run the `go:generate` step in `run/types.go` to regenerate `schema_gen.go` /
`types_gen.go`.

The H4 binding (§4.6) also adds a `libp2p-peer-id` field to the validator record
in the on-chain network definition — a protocol-level schema change, separate
from the `run` config `schema.yml` above, regenerated through its own
`go:generate`.

### 3.5 Roles are combinations

Because every service shares one P2P host and the framework already composes
`Configurations`, a single node can be any combination:

| Config blocks | Resulting node |
|---|---|
| `core-validator` | Validator |
| `follower` | Follower |
| `gateway` | API gateway |
| `bootstrap` | Bootstrap / discovery node |
| `follower` + `bootstrap` + `gateway` | One host doing all three |

## 4. Network configuration management

This is the part that makes the network self-managing rather than file-managed.

### 4.1 Two kinds of "configuration"

- **Identity / membership** — who the validators are, which partitions exist.
  This is **on-chain**: the Directory's network definition. `network-status`
  already reports partitions and their validator sets. It is authoritative and
  signed by consensus.
- **Reachability** — the current multiaddr(s) for a given peer. This is **not**
  on-chain (it changes too often) and is resolved by the libp2p DHT + the
  persistent peer database (`peerdb.json`).

The static `--peers` JSON and the hardcoded bootstrap multiaddr conflate the two.
The design separates them.

### 4.2 Validators added or removed

A validator change is an on-chain event. Every consensus node and follower already
sees it — it is part of the state they replicate. The `BootstrapService`'s
`PartitionTracker` keys discovery off the on-chain partition set, so a new
validator is discovered and connected without any file edit. A removed validator
ages out of the peer set.

The gateway resolves a partition to peers through the same path: ask the local
on-chain definition "who serves Cyclops?", then resolve those identities to
addresses via discovery.

### 4.3 Nodes changing IP address

A libp2p node that changes address re-announces to the DHT under the **same peer
ID** (identity is the key, not the address). Discovery and `peerdb.json` pick up
the new multiaddr. No human action, provided peer identity is stable.

The one identity that must never silently change is the **bootstrap node key** —
it is compiled into every binary via `BootstrapServers`. See accman #17.

### 4.4 Seed list: cold start only

A node needs a **seed** — one or more reachable addresses — only to make first
contact. Everything after that is dynamic (§4.2, §4.3): nodes sync topology with
each other peer-to-peer and persist what they learn to `peerdb.json`.

Seed resolution order at startup:

1. `--peer` / `--seed` flag, if given — explicit operator override.
2. Cached `peerdb.json` from a previous run, if it still has usable peers — a node
   that was already on the network does not need an external seed at all.
3. A **GitLab-hosted seed list** — the canonical, version-controlled, MR-reviewed
   copy. Fetched once at cold start over HTTPS.
4. A compiled-in default — last-resort fallback if GitLab is unreachable. The
   compiled-in copy also carries the seed-list verifying public key, the trust
   anchor for every other source (M2 / §9).

Once joined, the node has the on-chain network definition and the DHT; the seed
list is never consulted again until the next cold start with no usable
`peerdb.json`. The network is **not** dependent on GitLab at runtime — only an
otherwise-peerless cold start touches it.

Integrity of the seed fetch **is** security-critical. A consensus node could
tolerate a bad seed — it only has to reach one honest peer, after which on-chain
consensus anchors all trust. But a *gateway* cold-starting from a malicious seed
is eclipsed from birth and serves attacker-chosen data to clients (see H3 / §9).
The seed list is therefore signed and every source is signature-verified (M2 /
§9); the resolution order above is an availability preference, not a trust ranking.

The gateway's `--peers` JSON collapses into this same mechanism — it is just one
seed source, not "the peer map."

Optionally, expose a `network-peers` query method on the existing service
framework (same pattern as `MetricsService`) so a gateway or external client can
fetch a *live* partition→peer map from any node — steady-state, not a seed.

### 4.5 Trust boundary

Dynamic resolution must not let a hostile peer redirect a gateway to a fake
validator. The split in §4.1 contains this: validator **identity** (public keys)
comes from on-chain consensus; discovery only resolves **addresses** for keys that
are already known-good. A bad actor can at worst advertise a bad address for a
known key — a connection failure, not an impersonation.

Two caveats from the security review (§9):
- This containment relies on an authenticated binding from a validator's
  on-chain identity to its libp2p peer ID. CometBFT consensus keys and libp2p
  host keys are different keys, so the binding must be explicit — see §4.6 (H4).
  Until it ships, partition→peer resolution stays on the static map.
- It holds for *validator* traffic, which is block-signed end-to-end. It does
  **not** hold for *gateway query responses* (balances, tx status, account
  state), which are not block-signed: an eclipsed gateway can forge them (H3).

### 4.6 Validator identity binding (H4)

Discovery authenticates *addresses*, not *identities* (§4.5). The missing link is
an authenticated binding from a validator's on-chain identity to its libp2p peer
ID — without it, any peer can advertise "I serve Cyclops."

The binding lives **on-chain**: a `libp2p-peer-id` field on each validator entry
in the DN's network definition. That definition is already consensus-signed and
replicated to every node, so the binding inherits consensus anchoring — no new
key, no new trust authority.

- **Who sets it.** A governed network-definition transaction signed by
  **partition-operator quorum** against the partition's operator key book — the
  same authority that adds or removes a validator. A validator does *not*
  self-bind its own peer ID: quorum approval ensures a single compromised
  validator key cannot redirect its partition's identity.
- **Rotation.** Changing a validator's libp2p host key is an on-chain transaction
  under operator quorum. Peer-ID changes are rare, so governance latency is
  acceptable. Address changes are unaffected — still handled by the DHT under a
  stable peer ID (§4.3).
- **Lifecycle.** The entry is *created* when a node is promoted to validator and
  *removed* on demotion — both are already partition-operator-quorum
  network-definition transactions. `promote-validator` (env2, `AddValidator`)
  must be extended to set `libp2p-peer-id` in the same envelope. A promoted
  follower keeps its existing libp2p host key, so its peer ID is already known
  to the network and is simply registered — no re-bootstrap.
- **Resolution.** "Who serves Cyclops?" is answered from the on-chain definition
  (authenticated peer IDs); discovery then resolves only addresses for those peer
  IDs. An impostor cannot produce a peer ID in the on-chain set.
- **Downstream.** Because the binding is in consensus-replicated state, a gateway
  trusts it with no separate freshness or revocation protocol — consensus
  ordering is the freshness guarantee. This is what lets H3 hold.

Interim: until the field and the quorum-authenticated resolution path ship,
partition→peer resolution stays on the static map (§9). When it ships, the
existing validator set must be **backfilled** with peer IDs in the same change
that switches resolution off the static map — a validator with no entry would
otherwise become unresolvable.

### 4.7 Consensus (CometBFT) peer management — the bootstrap server is bypassed today (#4043)

§4.1–§4.6 describe the **libp2p** overlay: the DHT, discovery, the gateway's
partition→peer resolution. That is the network the bootstrap server manages. But a
node's ability to *join consensus* does not run over libp2p at all — it runs over
**CometBFT's own P2P network**, which the bootstrap server never touches. This is why,
operationally, "the bootstrap server is not used by nodes": it serves the wrong
network's addresses for the one thing that gates membership.

#### The two disjoint peer networks

Accumulate runs two separate peer-to-peer networks with different keys, addresses, and
peer sets:

| | libp2p overlay | CometBFT consensus |
|---|---|---|
| Identity | libp2p `peer.ID` | CometBFT `NodeID` (address of the consensus p2p key) |
| Address | libp2p multiaddr (AccP2P port) | `host:port` (Tendermint P2P port) |
| Peer set | DHT + bootstrap `PartitionTracker` | `persistent_peers` string |
| Source | bootstrap server (**dynamic**) | baked in at init/migrate (**static**) |

The bootstrap server's `PartitionTracker`
(`cmd/accumulated-bootstrap/partition_tracker.go`) originally stored only `peer.ID` +
libp2p multiaddrs, with the partition *inferred* from libp2p protocol IDs (the
guesser removed per §3.1). It had no notion of a CometBFT `NodeID` or a Tendermint
P2P endpoint. Consensus peering, meanwhile, is built
in `cmd/accumulated/run/consensus.go` purely from the static `DnBootstrapPeers` /
`BvnBootstrapPeers` written once by `cmd_init_network.go` / `cmd_migrate.go`. Nothing
updates `persistent_peers` at runtime from the bootstrap server.

Consequence: when a validator changes IP or the validator set changes,
`persistent_peers` goes stale and the bootstrap server **cannot** repair it. The §4.3
"nodes change IP, DHT picks it up under a stable peer ID" guarantee covers the libp2p
overlay only — it does **not** extend to consensus reachability. This is the gap that
makes the bootstrap server irrelevant to actual node membership.

#### The runtime plumbing already exists

The live `run` framework already has the mechanism to make consensus peering dynamic;
only the *source* of the peer list is static:

- `run/consensus.go` holds `node.Switch()` and already runs a `SyncMonitor` goroutine
  that calls `Switch().DialPeersAsync(...)` to re-dial peers when sync stalls
  (`internal/node/daemon/sync_monitor.go`).
- It re-dials `d.config.P2P.PersistentPeers` — the static list. The missing piece is a
  *dynamic* source feeding that dial path.

#### Design (Option A — bootstrap server brokers CometBFT reachability)

Apply the same identity/reachability split as §4.1, now to the consensus layer. The
bootstrap server brokers **addresses**; consensus membership (who is a validator) stays
anchored on-chain.

1. **Advertise.** Each node announces its CometBFT endpoint `{partition, host, P2P
   port, RPC port}` over a libp2p stream authenticated by its host key. The CometBFT
   `NodeID` is **not** asserted: the bootstrap derives it from the peer's libp2p key
   (`consensuspeer.NodeIDFromPeerID`), so it cannot be forged. (The RPC port is carried
   so §4.8 need not assume the default P2P+1 layout.)
2. **Track.** The bootstrap stores each peer's advertised endpoint(s) in a
   per-partition index (`consensusByPartition`), kept **separate** from the libp2p
   `PeerPartitionInfo` so a dual node serving DN + a BVN is disambiguated. It exposes
   a partition→`[NodeID@host:port]` list via `/consensus-peers/{partition}`.
3. **Consume.** Each node periodically pulls its partition's consensus peer list from
   the bootstrap server and feeds it into CometBFT via the already-wired
   `Switch().DialPeersAsync` — so the bootstrap server becomes the live source of
   consensus peers, replacing the frozen baked-in list. The static list remains the
   cold-start seed (§4.4); the broker supplies steady-state updates.

#### Why this is safe at the address layer

CometBFT's secret-connection handshake verifies that the remote actually holds the key
matching the advertised `NodeID`. A forged or stale address therefore *fails to
connect* — it cannot impersonate a validator. This is the same containment as §4.5: a
hostile broker entry is a connection failure, not an identity substitution. Validator
identity stays consensus-anchored; only reachability is brokered.

The weak binding is libp2p-`peer.ID` ↔ CometBFT `NodeID`: a peer asserts its own
host:port (the `NodeID` itself is derived from the libp2p key, not asserted). For
*dialing*, a bad address assertion is self-defeating (the handshake rejects it). The authenticated version of that binding is Option B below, and it is the
consensus-layer analogue of the libp2p `libp2p-peer-id` binding in §4.6 — a different
key, but the same on-chain mechanism.

#### Option B — on-chain CometBFT endpoints (later hardening)

Put the CometBFT `NodeID` (and optionally endpoint) on each validator entry in the
on-chain network definition, set under partition-operator quorum, exactly as §4.6 does
for the libp2p peer ID. This makes the binding authenticated end-to-end and removes the
self-assertion in Option A's step 1. It is heavier (schema change + governed update
path + one-time backfill) and is sequenced *after* A: ship A for dynamic reachability
now, layer B on for authenticated identity later. The two share the §4.6 governance and
backfill machinery.

#### Placement

- Option A gates on **Phase 1** (the bootstrap components move into the framework, so
  the advertise/track/consume path can attach to the shared host) and is independent of
  the libp2p resolution work — it does not need H4. Until A ships, `persistent_peers`
  stays static (today's behavior).
- Option B sequences with **Phase 3 / §4.6 (H4)**: it reuses the same on-chain
  network-definition change and backfill, adding the CometBFT `NodeID` field alongside
  `libp2p-peer-id`.

### 4.8 Fleet version consensus — upgrade-readiness gate (#4043)

A consensus-critical protocol change — a new `ExecutorVersion` activated network-wide
by `ActivateProtocolVersion` — is only safe to activate once **every validator runs a
binary that implements it**. If a validator is still on an older binary when the
version activates, it computes a different app hash at the activation height and the
partition halts. Operators need one authoritative answer to: *is the fleet ready?*

The bootstrap server already sees every node (it brokers their consensus reachability,
§4.7), so it is the natural place to answer this. It needs **no new advertisement
field** — the data is already exposed by each node:

1. **Authoritative roster.** The bootstrap needs the partition's *expected* validator
   set so it can report a **missing** validator, not merely disagreement among the ones
   it reaches. Two equivalent sources exist: the on-chain `NetworkDefinition.Validators`
   (via v3 `network-status`) and CometBFT's `/validators` RPC — the same Ed25519
   consensus keys, since genesis registers the priv_validator key directly into the
   on-chain roster (`init.go` → `AddValidator`) and `DiffValidators` projects that same
   set back to CometBFT. The implementation uses **`/validators`**: it reuses the RPC
   transport already needed for step 2 and avoids wiring a v3 message client into the
   bootstrap. The roster is fetched from the first reachable peer of the partition, with
   a last-good cache when none answer.
2. **Per-node probe.** For each consensus peer it tracks (host from the §4.7
   advertisement), the bootstrap calls the node's CometBFT RPC:
   - `/status` → `validator_info.pub_key` (consensus ED25519 key) and `voting_power`
     (whether it is currently a validator). `SHA256(pub_key)` matches the on-chain
     `PublicKeyHash`.
   - `/abci_info` → the ABCI app `Data` field, which carries `{Version, Commit}` =
     `accumulate.Version` (the git-describe build string). This is the binary version.

   RPC is bound to `0.0.0.0`, so an off-host bootstrap can reach it, and the
   `exp/tendermint` HTTP client already implements `Status()` / `ABCIInfo()` and the
   P2P→RPC port offset.
3. **Liveness.** A node that does not answer the RPC is marked not-present; this probe
   is also the §3.1 liveness signal.

**Endpoint.** `GET /consensus/{partition}` returns the per-validator version map and a
single boolean:

```json
{
  "partition": "Cyclops",
  "validatorConsensus": true,
  "agreedVersion": "v1.4.4-beta.3-18-gb22356d13",
  "versions": { "v1.4.4-beta.3-18-gb22356d13": ["<keyHash>", "<keyHash>"] },
  "validators": [
    { "keyHash": "…", "version": "v1.4.4-beta.3-18-gb22356d13", "present": true }
  ]
}
```

`validatorConsensus` is **true** iff every on-chain validator active on the partition
is present *and* all present validators report the same version. Any missing validator
or any version split yields **false** — "do not activate yet".

**Authority.** Validator membership is the on-chain set (step 1); version and consensus
key come from the validator's own running node (step 2), matched by `SHA256(pubkey)` so
a node cannot be counted as a validator it is not. This is an *operational readiness*
signal, not a security boundary — the fleet is operator-controlled, so the self-reported
version is trusted. A validator-key-signed version attestation (the §4.6 / Option-B
analogue) is the cryptographic hardening and is deferred.

**Assumptions & edge cases.**

- *Ed25519 consensus keys.* The `SHA256(pubkey)` match relies on validators using
  Ed25519 consensus keys — `DiffValidators` and the executor reject non-32-byte keys,
  and genesis registers the priv_validator key directly into the on-chain roster
  (`init.go` → `NetworkDefinition.AddValidator`). A future consensus key type would
  break both registration and this match; Ed25519 is an explicit precondition.
- *RPC reachability.* The probe needs the node's CometBFT RPC, which defaults to a
  `0.0.0.0` bind. The advertisement carries the RPC port explicitly (§4.7 step 1), so
  no port-layout assumption is made; but a node whose operator binds RPC to loopback
  (or firewalls it) is unprobeable and counts as **not present** — fail-safe, the gate
  reads "not ready".
- *Catching-up validators.* A state-syncing validator (`/status` `CatchingUp=true`) is
  counted by version but flagged; its binary version still matters for the gate.
- *Promotion/demotion window.* The on-chain roster (step 1) and the live CometBFT
  validator set lag each other by the ABCI `ValidatorUpdates` applied at block end, so a
  just-promoted/demoted node can transiently appear missing. This resolves to a
  conservative "not ready" — a false-negative, never a false-positive.
- *Liveness & debounce.* `validatorConsensus` is single-failure-sensitive: one transient
  RPC timeout flips it to `false`. The probe runs on the same ~15 s cadence as the
  consensus-peer probe; a validator unanswered for >60 s is marked not-present.
  Operators should require N consecutive `true` reads (suggest N≥3) before activating,
  not a single poll.
- *Freshness dependency.* The roster query needs at least one reachable peer serving the
  v3 NetworkService for the partition; the answer is only as fresh as that peer's
  replicated state.

**Why now.** This is the go/no-go gate for the Cyclops BPT repair (`V2CyclopsBptRepair`,
#4020): the repair rewrites 22 BPT leaves at its activation height and must execute
identically on every validator. `/consensus/Cyclops` returning `true` is the check
before broadcasting that activation.

## 5. Deployment model

One artifact — `accumulated` — deployed everywhere. Role is the TOML config.

accman supplies, per node:
1. the `accumulated` binary (single version to track);
2. the node key (for bootstrap nodes this is the *managed network identity* — see
   accman #17; for others, a per-node key);
3. a small seed peer list;
4. the role config (`core-validator` / `follower` / `gateway` / `bootstrap`).

Everything else — the validator set, partition assignments, current peer
addresses — the node learns at runtime. accman no longer edits topology files on
every membership or IP change.

## 6. Migration plan

| Phase | Work | Outcome |
|---|---|---|
| 1 | Move bootstrap components into `run`; add `BootstrapService` + `BootstrapConfiguration`; schema regen. `accumulated-bootstrap` becomes a thin wrapper that emits the equivalent config. | `accumulated` can be a bootstrap node. No behavior change. |
| 1 | Consensus peer brokering (§4.7, #4043, Option A): advertise CometBFT `{NodeID, host:port}` over libp2p, track it on the bootstrap server, feed it into `Switch().DialPeersAsync`. | `persistent_peers` is dynamic; the bootstrap server actually manages consensus reachability. |
| 2 | Fold `accumulated-http` behavior into `GatewayConfiguration` / `HttpService`. `accumulated-http` becomes a thin wrapper. | `accumulated` is a full gateway. |
| 3 | Add seed-vs-map separation and the optional `network-peers` query method; demote static `--peers`. | Topology is discovered, not file-managed. |
| 4 | accman deploys the unified binary in all roles (ties to accman #9, #17). Delete the standalone binaries after one release of overlap. | One binary network-wide. |

### 6.1 Backwards compatibility — old nodes must always rejoin

A node running a pre-integration binary must be able to cold-start and rejoin the
network at any point during and after the migration. This is a hard constraint:
if a stopped node cannot get back on the network, the migration has stranded it.

- **The bootstrap node's libp2p identity is invariant.** The integrated
  `BootstrapService` runs on the *same* libp2p host key as today's
  `accumulated-bootstrap`. Every deployed binary has that peer ID compiled in via
  `BootstrapServers`; minting a new key would strand every node that has not yet
  upgraded. accman must carry the existing key into the integrated node.
- **The libp2p bootstrap / DHT path stays wire-compatible.** Folding `InfoServer`
  into `BootstrapService` is an HTTP-side reorganization; the libp2p host, its DHT
  server mode, and its protocol IDs — the path nodes actually use to discover
  peers — must not change.
- **The seed list stays parse-compatible.** M2 (§9) adds a signature; it must be
  additive — a detached signature, or a field old parsers ignore — so a node on
  old code can still fetch and parse the GitLab seed list. New code verifies the
  signature; old code ignores it and is no worse off than today.
- **Removing `/connect` (H1) does not affect rejoin** — `/connect` is an HTTP
  tooling endpoint; nodes bootstrap over libp2p/DHT, not `/connect`.
- **Consensus peer brokering (§4.7) is additive.** The static `persistent_peers`
  remains the cold-start seed; the broker only supplies steady-state updates. A
  pre-integration node ignores the broker and keeps using its baked-in list, so it
  rejoins exactly as today.
- Thin `accumulated-bootstrap` / `accumulated-http` wrappers remain for one
  release so deploy scripts do not break (deleted after Phase 4, §8).

## 7. Risks and tradeoffs

- **Shared P2P host on validators.** DHT `ModeAutoServer` and bootstrap duties add
  peer churn and load. The binary **refuses** to start a `core-validator`
  combined with `gateway` or `bootstrap` in one process — fail-closed startup
  validation, not operational policy (H2 / §9). Co-locating `bootstrap` with a
  follower or gateway is permitted; a validator stays lean and single-purpose.
- **Bootstrap identity is still key-bound.** Integration does not remove the
  constraint that the bootstrap peer ID is compiled in. The key remains the
  network anchor; protect it (accman #17).
- **Schema regeneration.** `schema.yml` additions require the `go:generate` step;
  generated files must be committed together.
- **Discovery trust.** Mitigated by the identity/reachability split (§4.5), but the
  `network-peers` method, if added, should return data anchored to on-chain keys.

## 8. Open questions

None outstanding. The wrapper-lifetime question is settled — `accumulated-bootstrap`
and `accumulated-http` are deleted after Phase 4, per the §6 migration plan, not
kept long-term.

## 9. Security remediation plan

A security review of this design produced findings H1–H4, M1–M6, L1–L4. They are
remediated on the schedule below. Tiers are ordered by urgency; the migration
phase each item gates (§6) is given per row.

### Tier 0 — Live bug, fix immediately (independent of this design)

| Finding | Fix |
|---|---|
| H1 — `/connect` unauthenticated SSRF | Remove `/connect` from `accumulated-bootstrap`; discovery never needs an externally triggered dial. The hardening alternative (bearer token + loopback bind + private-range rejection + rate-limit) is **not** pursued: a `/dns4/…` multiaddr resolves to an address only at dial time, so a pre-dial target check cannot close DNS-rebinding SSRF. Tracked as accumulate #4026 (confidential). |

H1 is an exploitable bug in deployed code, not a design issue — it does not wait
for the integration.

### Tier 1 — Gate the integration (fail-closed architecture)

The integration must not ship without these.

| Finding | Fix | Gates |
|---|---|---|
| H2 (#4027) — role co-location | The binary **refuses** `core-validator` combined with `gateway`/`bootstrap` in one process. Startup config validation, fail-closed. | Phase 1 |
| M6 (#4035) — misconfiguration | `accumulated config verify` linter; startup validation rejects dangerous role combinations and unsafe public listeners. | Phase 1 |
| H4 (#4029, conf.) — identity binding | Resolved (§4.6): a `libp2p-peer-id` field on each validator entry in the on-chain network definition, set by a partition-operator-quorum governed transaction. Scope: the schema field; the governed update path; quorum-authenticated partition→peer resolution; extend `promote-validator` (env2) to set the field; a one-time backfill of the existing validator set landing atomically with the switch off the static map. | Phase 3 |
| H3 (#4028, conf.) — gateway trust | Gateways verify query responses against consensus-anchored state proofs/receipts, or trust only on-chain-anchored peers (depends on H4). | Phase 3 |

### Tier 2 — Harden the seed / network-config mechanism

| Finding | Fix | Gates |
|---|---|---|
| M2 (#4031, conf.) — unsigned seed list | Sign the seed list with an offline network key. Every source (flag, `peerdb.json`, GitLab, compiled-in) must pass signature verification; the compiled-in copy carries the verifying public key as the trust anchor. | Phase 3 |
| M1 (#4030) — no key rotation | Rotation path: a signed seed list can introduce a new bootstrap identity; evaluate multiple bootstrap keys so rotation never requires a fleet-wide rebuild. | Phase 3 |
| M3 (#4032, conf.) — `peerdb.json` poisoning | Treat `peerdb.json` as a cache: validate and cap entries on load, prefer on-chain-anchored peers, never a trust store. | Phase 1 |

### Tier 3 — Endpoint hardening (when components move into the framework, Phase 1–2)

| Finding | Fix |
|---|---|
| M4 (#4033, conf.) — debug endpoints | `/debug/dht` and pprof bind to loopback or a separate authenticated admin listener. Drop the blanket `net/http/pprof` import; pprof only via `--pprof` on a private address. |
| M5 (#4034, conf.) — topology disclosure | `/peers`, `/connections`, `/stats` disabled or authenticated on non-bootstrap roles; rate-limited on bootstrap nodes. |
| L1 (#4036) — CORS | Safe defaults; reject `*` with a startup warning. |
| L3 (#4038) — ACME key | Document the Let's Encrypt account key as a managed secret. |
| L4 (#4039) — rate limiting | Rate-limit all InfoServer endpoints, including `/connect` if retained. |

### Tier 4 — Documented residual risk

| Finding | Action |
|---|---|
| L2 (#4037) — gateway censorship | Document as an inherent gateway availability property; mitigate client-side (multi-gateway, consensus-proof verification — overlaps H3). |

### Cross-cutting

- Security regression tests: an SSRF test for `/connect`, config-validation tests
  for H2/M6, signature-verification tests for M2.
- No migration phase ships until its gating Tier 0/1 items are merged and tested.

## 10. Related

- accman #9 — `http-gateway` follower-type and `peers.json` management.
- accman #17 — bootstrap + gateway server transitions; bootstrap key custody.
- accumulate #4021 / #4024 — `accumulated-http` peer-map; removal of the hardcode.
- accumulate #4026 — `/connect` SSRF (confidential; Tier 0 above).
- accumulate #4043 — bootstrap server brokers CometBFT peer addresses (§4.7).
- `docs/plans/bootstrap-v2.md` — prior bootstrap work.
