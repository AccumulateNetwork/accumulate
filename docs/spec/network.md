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

## A validator's network identity

A validator is two keys. Its **validator key** signs what the protocol
counts: anchors, and under DAG-BFT headers, votes and certificates. Its
**node key** is its libp2p identity (`p2p.key`): the peer ID every connection
to it is authenticated by, and the key every gossip message it publishes is
signed with (the router's default `StrictSign` policy; a message's `from` is
the peer ID whose key signed it). The two are separate keys, and until now
the only thing that tied them together was the relay challenge
(`internal/node/dagbft/relay_challenge.go`, #4366): a proof made on demand,
for one request. Nothing a node could read said which peer a validator is,
so the consensus topics could not be limited to the committee's peers
(#4596), and committee members had no way to find one another but the
deployment's bootstrap lists (#4587).

**The binding is on chain, in the network definition.** Each `ValidatorInfo`
of `dn.acme/network` carries, beside the validator's key, its node's
identity (`protocol/general.yml`):

```yaml
ValidatorInfo:
  fields:
    # ... PublicKey, PublicKeyHash, Operator, Partitions, then:
    - name: Peer
      description: is the libp2p identity of the node that runs this validator, attested by the validator key
      type: ValidatorPeer
      marshal-as: reference
      pointer: true
      optional: true

ValidatorPeer:
  fields:
    - name: ID
      description: is the node's libp2p peer ID, the bytes of its multihash
      type: bytes
    - name: Signature
      description: is the validator key's signature over the peer-ID attestation
      type: bytes
```

The record is the peer ID itself, not the node's public key: the peer ID is
what the router compares and what a connection is authenticated by, and it
is defined for every key type libp2p has, while nothing requires a node key
to be ed25519 (every key the node writes is). An ed25519 peer ID is 38 bytes,
an identity multihash with the key inline. In JSON it is hex, as every
`bytes` field is; tooling prints the base58 form (`12D3KooW…`) beside it. A
node is one libp2p identity whatever partitions it runs (the Directory and a
BVN on one host, "Converting a mainnet node"), and a validator key is one
node's, so the record is per validator, not per partition.

**What is signed, and by which key.** `Signature` is the validator key's
ed25519 signature over the attestation

```
"acc-validator-peer-id-v1" ‖ network name ‖ SHA-256(validator public key) ‖ peer ID bytes
```

The first element is a fixed ASCII tag, in the same role as the relay
challenge's: the validator key also signs headers, votes, certificates,
anchors and challenge answers, and the tag is what keeps an attestation from
being read as any of those, and them as an attestation. The network name is
in it so an attestation made for a devnet is not one for mainnet. The key
hash is in it so the record binds *this* validator, and the peer ID so it
binds *this* node. An attestation holds for the key that made it: a
validator whose key is rotated (`UpdateValidatorKey`) attests again under
the new key, in the same write.

Who may say what:

- **Only the validator can bind a peer to its key.** The signature is the
  validator key's, which no operator and no other validator holds. A
  definition carrying an entry whose signature does not verify against the
  entry's own key is refused.
- **A node proves it is the peer by connecting.** The libp2p handshake
  authenticates the peer ID with the node key, and every gossip message is
  signed with it. So the record needs no second signature by the node key:
  a validator that records a peer it does not run has bound its committee
  seat to an identity it cannot speak from, and has shut out itself, nobody
  else.
- **One peer, one validator.** Two entries naming the same peer ID are
  refused.
- **Every active validator has one.** From v2-kourou an entry active on any
  partition without a `Peer` that verifies is refused. The DAG-BFT
  integration is unreleased, so there is no network with unbound validators
  to carry: genesis writes every validator bound.

These are checked where the definition is parsed, `GlobalValues.ParseNetwork`
(`pkg/types/network/globals.go`), which the executor runs on every write to
`dn.acme/network` (`network_accounts.go`, beside the check that the version
increases) and genesis runs on the definition it writes, so the check is one
and a definition genesis would write is one the executor would accept.
Twelve validators is twelve ed25519 verifies, once per definition write.

**Registering and rotating.** There is no new transaction. The definition
changes as it always has: a `WriteData` to `dn.acme/network`, writing to
state, that the operators' key book `dn.acme/operators` authorizes; the
write carries the whole definition with its version increased ("Where the
validator set lives"). To register a peer ID — a new validator's, or a
validator's new one — the operators write the definition with that
validator's entry carrying the new attestation. The validator makes the
attestation, since only it can: the node computes it at start from the two
keys it holds and serves it in its consensus status (`ConsensusStatus.Peer`),
and `accumulated` prints it on request; an operator collects it and writes
it. Genesis (`init network`, `BuildGenesisDocs`) holds both keys of every
node and writes every validator bound; the simulator's network does the
same. A node converted from CometBFT has both keys in `migrate-legacy`'s
hands too; what it writes to the definition belongs to the database
conversion ("Converting a mainnet node").

A validator **rotates** its peer ID — a new `p2p.key` — by having the new
binding written before the node restarts under it. The old peer stays
admitted while the committee its definition named is still scheduled
(consensus.md, "Which peers may send them"), so the node restarts under the
new key at any time between the write and then. Restarting under a new key
before it is written is the one order that does not work: the node comes up
as a peer no committee admits, and must be written in before it can speak.

**What reads it.** The committee built from the definition
(`types.Committee`) carries each member's peer ID beside its key
(`types.ValidatorInfo.PeerID`), through the executor bridge and
`onValidatorSetChange` as the keys travel today. Consensus uses it for two
things, both in consensus.md, "Which peers may send them": admitting
consensus messages only from committee peers, and keeping committee members
connected to one another. The relay (`internal/node/dagbft/relay.go`) still
challenges a candidate before handing it a submission; with the peer bound on
chain, a candidate whose peer ID is a member's *is* that member, and the
challenge is redundant for it — retiring it is a follow-up, not part of this
change. The record carries no addresses: a peer's addresses come from the
peerstore, the DHT or the bootstrap lists, as every peer's do.

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

