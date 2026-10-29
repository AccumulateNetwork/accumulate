# Design questions: a validator's peer ID and the consensus topics (#4596 part 2, #4587)

The decisions taken in network.md, "A validator's network identity", and
consensus.md, "Which peers may send them", with the alternatives that were
considered and not taken. DIFFERENCES C17 lists what remains to build.

## Where the record lives

Decided: a `ValidatorPeer` sub-record on `ValidatorInfo` of `dn.acme/network`,
per validator, not per partition (a node is one libp2p identity). Not taken: two
flat appended fields; the node's public key in place of the peer ID.

## What the entry is matched on

Decided: the peer ID is matched; `NodePublicKey` is carried beside it only to
verify the node signature, and the verifier derives the peer ID from it and
checks it equals `ID`. Not taken: **recovering the key from the peer ID**, which
restricts the node key to types whose ID carries the key inline (ed25519,
secp256k1) and excludes ECDSA and RSA.

## How an entry is matched to its predecessor

Decided: the network keeps, per peer ID, the highest `Sequence` it has ever
accepted (the tombstone), kept across `RemoveValidator` and
`UpdateValidatorKey`, checked only for an entry that differs from the active one,
in the executor's write path and not in `ParseNetwork`; genesis seeds it for every
peer ID it binds, and the activating version's block seeds it from the active
definition for every bound peer ID that has none (the active entry's
`Sequence` is the floor); the spine verifier skips it, held to the executor by an
agreement test. Not taken: **matching by validator key** (a removed validator has
no predecessor, a rotated key has a new hash); a definition-wide counter (cannot
tell which attestation is stale); checking every entry (refuses every unchanged
entry); giving the spine the executor state (a larger change to the verifier).

## What the origin gate covers

Decided: batches and certificate-sync responses only; an unadmitted origin is
ignored (`ValidationIgnore`), not rejected; dormant until the activating version
is active and every scheduled committee member has a verified peer. Authored
messages admit on their author checks. Not taken: the origin gate on every
consensus topic (a validator under an unrecorded peer ID would go silent, and
the repair is a definition write the Directory must execute).

## Where the gate sits on the certificate-sync topic

Decided: requests and responses share one topic and one validator, so the gate is
the first check after the type byte is read: a request (`0x01`) passes, a
response (`0x02`) is gated on `msg.GetFrom()`. Not taken: **two certificate-sync
topics**, one for requests and one for responses (more topics and a wire change
for what one byte read does).

## Who signs the attestation

Decided: both keys, over a length-prefixed struct with its own tag each; the
node-key signature makes "one peer, one validator" a rule about ownership.
Not taken: the validator key alone (a validator could claim another's new peer ID
and have the owner's entry refused as a duplicate).

## Direct peering, followers, the filter

Decided: the gossip layer keeps its own connections between committee members;
followers keep the same topics and cannot originate batches or responses. Not
taken: `pubsub.WithDirectPeers` (fixed at router construction, before any
committee is known); a separate follower feed; `PeerFilter` (it gates control
messages, and would cut followers off from relays).

## The executor version

Decided: no name or number here. The gate is the first executor version
activated after v2-kourou on this line; the owner decides which (#4139, !1199,
!1277), and no code for it is written until then.
