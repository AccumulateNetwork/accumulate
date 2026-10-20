# Network parameters — Specification

What a network declares about itself, as opposed to what each node happens to
be configured with.

## 1. Architecture — what we are doing

A network has properties that are true of the *network*, not of a node: the
acceptance thresholds, the major block schedule, the fee schedule, the limits.
They are recorded in `NetworkGlobals` when the network is deployed, they are
readable from the network afterwards, and every node uses the recorded value.

**Block interval is one of them.** A network runs at one cadence. The
alternative — each node pacing itself from its own configuration — does not
produce a network property at all: both engines advance on a quorum, so the
cadence becomes whatever the quorum-th node produces. That has three
consequences, and all three are defects rather than untidiness:

- a misconfigured node cannot be detected, because there is no network value to
  compare it against;
- the cadence changes when the validator set changes, since who is in the
  quorum decides how fast the network runs;
- everything denominated in blocks — anchor emission, pending expiry, the
  execution-lag bound — rests on a number nothing guarantees.

### The invariants

1. **A deployed network declares its block interval.** A default is
   permitted; an *unrecorded* default is not. After deployment the value is
   read from the network, never inferred from a node's binary.
2. **A node paces from the network's value.** Not from its own.
3. **A node that disagrees does not run.** The divergence is refused and named
   at startup — not silently honoured, which would restore the emergent
   cadence, and not silently overridden, because an operator who set a value is
   entitled to be told their node is not running it.
4. **Catch-up is exempt.** A node behind the frontier ignores pacing entirely,
   and enforcing the network value must not change that: pacing a catch-up is
   an absorbing state, and it once pinned 4 of 12 validators behind for a whole
   run.

## 2. Specification — how it is implemented

**Recorded.** `NetworkGlobals.BlockInterval`, a duration, beside
`MajorBlockSchedule` — the timing parameter that already lives there. It is an
appended field, so a network deployed before it decodes with the field absent.
`NewGlobals` fills in `protocol.DefaultBlockInterval` when the operator states
nothing, so genesis always records a value. Zero means nothing was stated and
takes the default; a negative value is a wrong statement rather than an absent
one, so it survives to be refused by `genesis.Init`.

**Resolved at startup.** `resolveBlockInterval` takes what the network declares
and what the node was configured with:

| network declares | node states | result |
|---|---|---|
| a value | nothing | the network's |
| a value | the same | the network's |
| a value | something else | **refused, both values named** |
| nothing | a value | the node's, with a warning |
| nothing | nothing | the default, with a warning |

A network that declares nothing is one deployed before the field existed. Those
keep running on local configuration; they are not locked out.

**Applied.** Rounds pace at half the block interval, because Bullshark commits
a leader every other round. The resolved value sets
`NodeConfig.MinRoundInterval`, which is the only pacing input the primary has.
The catch-up exemption is upstream of it, in the primary's `behind` check, and
is untouched by resolution.

**The recorded default and the pacing default are the same number** in two
packages that cannot reference each other. A test asserts their equality: if
they drift, recording a default at deployment silently re-paces every node that
stated nothing.

### Not yet done

Changing the interval after deployment. The value is fixed when the network is
deployed and there is no ratification path: a change requires redeploying.
Making it changeable — a threshold of validators ratifying, activating at a
major block boundary so partitions switch together — is #4267's remaining
scope. Until then a deployed network's cadence is immutable, which is a
limitation, not a design.

## Where the validator set lives

A partition's validator set and its threshold are the **network definition**,
`dn.acme/network`, published as an account like every other protocol fact.
It is the set an anchor's signatures are counted against — by the executor
when an anchor arrives, and by a joining node validating the spine
(executor.md, "Sync" §1) — and it changes only by a write that the operators'
key book (`dn.acme/operators`) authorizes. The operators' page is therefore
the governance root and the definition is the authority read at run time;
nothing counts anchor signatures against the operators' page itself (#4301).

## How a new node gets its genesis

A partition's genesis is the snapshot its nodes load into an empty database
(`DAGBFTService.loadGenesisIfNeeded`). It is a property of the network, fixed
at deployment, so a node added later must start from the same bytes.

**Every node serves the genesis of each partition it runs**, on API v3, as the
`genesis` method (`api.GenesisService`). The request is `GenesisRequest`
(message type 0x0F) carrying `GenesisOptions{NodeID, Partition}`; the answer
is `GenesisResponse` (0x2F) whose `Value` is the snapshot file, byte for byte.
It is served on the partition's consensus service address
(`/acc-svc/consensus:{partition}`); a `NodeID` routes the request to that
node, and an empty one to any node serving the partition. Over JSON-RPC the
method is `genesis` and the bytes are base64.

`accumulated init node <peer>` and `accumulated init dual <peer>` use it.
They ask the peer's node info (at the peer's base port, where its one API is)
for its libp2p identity and partitions, fetch each partition's genesis from
that node, and write the layout `init network` writes for a node: one
`accumulate.toml` with a core-validator (or, with `--follow`, follower)
configuration — `DN`, `BVN` or `Dual` — its node and
validator keys inline, `join-running-network` set, and the genesis snapshots
beside it (`directory-genesis.snap`, `{bvn}-genesis.snap`). The peer is the
bootstrap peer, at `/ip4|dns/{host}/tcp/{base+2}/p2p/{id}`. `--genesis-doc`
(`--dn-genesis-doc`, `--bvn-genesis-doc`) supplies a snapshot from a file
instead. `init` never overwrites an existing `accumulate.toml`; `--reset`
removes it first.

**Which partitions.** A DAG-BFT node has one API, so the peer's port no longer
says which partition is meant (on main it did: the DN and BVN had separate
base ports). `init node --partition <id>` names it — `Directory` or a BVN's
ID, in any case. Without it the node runs **every partition the peer runs**:
the consensus services its node info lists, the Directory and its BVN, which
is what `init dual` writes; against a peer that runs only a BVN, a `BVN` node.
Each partition is confirmed by asking the peer's `consensus-status` with that
partition set, and its type and canonical ID come from the answer; a
partition the peer does not run is refused at `init`, naming it. `init dual`
is the default case that insists on both. With `--seed`, the seed proxy's
`<partition>.<network>` names the partition unless `--partition` does (#4489).

**The node key is the node's identity.** A node's libp2p peer ID is its
node key (`p2p.key`); `init` writes it inline, so it is the same at every
start. A configuration that names no node key does not silently start under a
new one (#4490):

- a **core validator refuses to start**, naming the file: its peer ID is bound
  in its committee and named in its peers' bootstrap lists, so a new identity
  would strand both, and a generated key would hide the operator's mistake
  rather than fix it;
- a **follower is given one**: the node generates a key, appends it to its
  TOML configuration as an inline raw key (the operator's text is kept as it
  was), and logs that it did, so every later start has that identity. Nothing
  is bound to a follower's identity until it first runs, so making it is safe;
  a follower with no TOML file to write to refuses, as a validator does;
- any other configuration (a gateway, a devnet) keeps the transient key it
  always had, which warns when it is made. An explicit `type = "transient"`
  key is the operator's choice and is honoured.

A node initialized for CometBFT is converted by the standalone
`tools/cmd/migrate-legacy` (see "Converting a mainnet node"). A node never
reads that layout.

**A genesis snapshot is state only**: the header, the records and the BPT.
The validator set it starts from is the network definition among those
records. Until #3910 genesis also wrote a *consensus* section, a CometBFT
genesis document (chain ID, consensus parameters, validator addresses). No
node ever read it. Genesis no longer writes it; a file that has one still
loads, and the section is ignored (section type 10 stays assigned for that).

**A fetched genesis is a copy, not a trust root.** Nothing in the answer proves
it is the network's genesis; the operator trusts the peer they named, which is
why `init` warns. The proof service takes the same position: a walk of the
spine starts at a genesis pinned out of band.

Message types 0x0F and 0x2F are this line's. Main assigns up to 0x0E and 0x2E,
so they are free there today, and must not be assigned to anything else there.

## Adding a BVN

A BVN is added to a running network, without a restart, by one write to
`dn.acme/network` that names the new partition and its validators, authorized
by `dn.acme/operators` like every other change to the definition. The routing
table gives the new partition no buckets and no overrides, so it holds no
accounts and receives no user transactions until an identity is moved to it
(below).

**Genesis.** The Directory builds the new partition's genesis at the block that
executes the write, from the network definition and globals as of that block.
It holds the partition's system accounts and nothing else. Nodes serve it as
the partition's genesis ("How a new node gets its genesis"), so the first
validators and every later node start from the same bytes.

**Activation.** The partition is active from the first Directory block after
the write. From then the Directory anchors to it and counts its anchors
against its validators in the definition. The other BVNs learn of it from the
Directory anchor that carries the update (`processNetworkAccountUpdates`) and
create its synthetic and anchor ledger entries on first use
(`SyntheticLedger.Partition`, `AnchorLedger.Anchor`), so no existing ledger is
migrated. Until its validators produce a first block it is reported as a
partition with no blocks, not as a stalled one.

**Validators.** A validator of the new partition runs
`accumulated init node <peer> --partition <id>` against a node that runs the
Directory, and joins as any validator joins a running network.

## Moving an identity between partitions

Routing is by identity: the routing tree looks an account up by
`IdentityAccountID32`, overrides first, then the bucket of `u.Routing()`
(`RouteTree.Route`). An identity — a root ADI or a lite identity, with every
account under it — is therefore the unit that moves. It moves from partition
A to partition B in five steps, each an executed fact at a named block.

1. **Stage.** The operators request the move of identity I from A to B. B's
   nodes copy I's records from A into their sidecar (database.md, "The
   sidecar") as of a block of A, and keep the copy current until the lock.
   Staging changes no state on either partition and may be abandoned.
2. **Lock.** A lock on I, authorized by `dn.acme/operators`, executes at block
   L of A. From L, A refuses every new transaction whose principal is in I,
   and every signature from one of I's key books, as moving (executor.md, "A
   locked identity"). Synthetic messages addressed to I are held.
3. **Move.** B takes I's records as of L. Each is checked against A's BPT root
   at L, anchored in the Directory, before B writes it into its live database
   and BPT. A record that fails the check stops the move and releases the lock
   on A.
4. **Transfer.** The Directory writes an override routing I to B in
   `dn.acme/routing`. Every partition routes I to B from the block that
   executes the Directory anchor carrying it. At that block A removes I from
   its BPT and forwards to B the synthetic messages it held for I. A keeps I's
   chain records, so history written on A stays readable there.
5. **Unlock.** B accepts transactions for I from the block that executes the
   transfer anchor.

A synthetic message produced against the old route that reaches A after the
transfer is forwarded to B, never dropped. A receipt for an entry made on A
before the move anchors into A's root chain, which the entry's index names
(#4567). Pending transactions on I move with I and keep their signatures.

**A bucket** moves by moving every identity in it, then reassigning the bucket
in `Routes` and removing the overrides the moves wrote.

None of this section or the one above is built yet (DIFFERENCES, E31).

## Converting a mainnet node

A mainnet node was initialized for CometBFT. Moving it to DAG-BFT is a
conversion, done once, offline, by `tools/cmd/migrate-legacy`; the node itself
reads only its own layout.

**Configuration and keys (done).** The tool accepts either shape a mainnet
node can be in:

- the CometBFT layout — `dnn/` and `bvnn/`, each with `config/tendermint.toml`
  and `config/accumulate.toml` — which it merges into one `accumulate.toml`
  with a core-validator configuration;
- an `accumulate.toml` that main's `accumulated migrate` already wrote, whose
  keys still name CometBFT key files.

Either way every key is written **inline**, as a raw private key — the form
`accumulated init network` writes. The node and validator keys are read from
CometBFT's JSON key files (`node_key.json`, `priv_validator_key.json`, type
`tendermint/PrivKeyEd25519`): wherever `p2p.key` or a configuration's
`validator-key` names one (`cometNodeKeyFile`, `cometPrivValFile`), and, for
a core validator or follower with no `p2p.key`, from
`{dnn|bvnn}/config/node_key.json`, which main's node defaulted to. The node
knows neither key type (their numbers, 4 and 5, stay unassigned): a
configuration that still names one fails to load with a message pointing at
the tool.

A node is one libp2p identity, so `dnn/` and `bvnn/` must hold the same node
key (main's `init dual` wrote one); the tool **refuses keys that differ**,
for node keys as for validator keys, rather than keep one and drop the other.
**Nothing it replaces is lost**: each partition's `accumulate.toml`, and any
`<work>/accumulate.toml` it is about to write over, is copied into
`<work>/backup/` first; a backup is never overwritten (a later one is named
`accumulate.toml.1`, `.2`, …) (#4490).

**Bootstrap peers are never made up.** CometBFT's `persistent_peers` name
each peer by its CometBFT node ID, which is not a libp2p identity; main's
`migrate` zero-padded it into a peer ID that no node has, so every such peer
failed its identity check on dial. The tool carries none of them over, and
drops any already written that way from a configuration main's `migrate`
converted. A node converted from the CometBFT layout gets the partition
bootstrap peers the operator names (a configuration main's `migrate` wrote
keeps the real ones it has): `--bootstrap <multiaddr>` (with its `/p2p/` peer ID), or
`--peer <url>`, a running node whose API v3 node info gives its libp2p
identity, as `init node` takes it. When there are none, the tool says so;
the node then finds peers only through `[p2p] bootstrap-peers`, which the
CometBFT-era `accumulate.toml` already held as libp2p addresses and which are
kept (#4490).

**The database (future work).** Converting a mainnet node's database to
what a DAG-BFT node holds is not built. When it is, it belongs here and in
the same tool: the key conversion is its first step, not a separate one.

