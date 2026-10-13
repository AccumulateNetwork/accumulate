# Bitcoin anchoring — Specification

How the network commits its state to Bitcoin, so that a third party can verify
what was recorded on Accumulate, and when, with Bitcoin and one published
starting point — and nothing else from us: no node, no database, no word.

## 1. Architecture — what we are doing

### The case

A customer holds a document — an eligibility decision, a policy, an approval —
and must show a counterparty that it was recorded, that the parties who had to
approve it did, and that all of this happened by a certain time. The parties
share no database. The counterparty may distrust every one of them, and us.

Accumulate records the document's hash as a data entry, and enforces who must
sign before a change to an account takes effect. That gives the record and the
approvals. What it does not give, on its own, is a root of trust that a
stranger will accept: an Accumulate receipt ends at a Directory root, and the
stranger must trust whoever told them that root.

Bitcoin anchoring supplies that root. The Directory's root is committed, at
intervals, in a Bitcoin transaction sent from an address the network has
published. The stranger checks the receipt up to the root, the root in Bitcoin,
and the address against the published list. Bitcoin's proof of work fixes the
time; its history fixes the record.

### The objects

- **The anchor.** A Bitcoin transaction with one `OP_RETURN` output carrying
  the network's identity, a Directory block height and that block's root chain
  anchor, sent from an address on the list in force at that height. One anchor
  per interval; the interval is a parameter of the list in force.
- **The address list.** A data entry in the network's address-list account
  naming the Bitcoin keys the validators sign anchors with, which validator
  holds each, the threshold, and the Directory height the list takes effect
  at. The account is governed by the network's operator book at the network's
  threshold, so a list changes only when the validators agree on Accumulate.
  **Backup lists** are prepared — keys generated and held offline — and
  published when the list in use is compromised. Publishing one is the
  recovery.
- **The coordination account.** A data account any validator can append to.
  An anchor is proposed there, each validator records its signature share
  there, and the broadcast is recorded there. Every entry is signed by the
  validator's Accumulate key.
- **The proof builder.** A service that reads the block ledgers and keeps, for
  every entry of a configured set of accounts, the receipt from that entry to
  the next anchored Directory root, and the anchor that covers it. It produces
  the **evidence package**.
- **The verifier.** The program a stranger runs. It takes an evidence package,
  the pinned first anchor, and a Bitcoin node or light client.

### The invariants

1. **An anchor is a record, not a transaction.** A Bitcoin transaction is an
   anchor of (height, root) only if its `OP_RETURN` says so, it was sent from
   an address on the list in force at that height, *and* the coordination
   account holds a proposal for (height, root) with signature shares from a
   threshold of the validators of that height, each signed by that
   validator's Accumulate key. A transaction from a listed address with no
   record behind it is a theft of coins, not an anchor.
2. **The threshold is the network's.** The Bitcoin multisig requires the same
   fraction of validators as `ValidatorAcceptThreshold`. Fewer validators can
   never forge an anchor than can break the network.
3. **A validator signs only its own root.** A validator contributes a share
   for (height, root) only if its own node holds `root` as the Directory's
   root chain anchor at `height`. The multisig is the check; no separate
   watcher compares anchors to the network.
4. **The list is authoritative; the keys are not.** Which addresses may anchor
   is decided on Accumulate, by the operator book. Stolen Bitcoin keys cannot
   change the list, cannot make a record in the coordination account, and so
   cannot make an anchor. Recovery is publishing the next list; nothing has to
   be done in Bitcoin.
5. **Each anchor is confirmed by the next.** The coordination records of
   anchor *i* are entries under roots that anchor *i+1* commits. An anchor is
   *final* once the next anchor is confirmed; the newest anchor is
   *provisional*. A verifier reports which.
6. **Trust begins at one pin and proceeds by induction.** The verifier is
   given the Bitcoin transaction ID of the first anchor, out of band. That
   anchor fixes the first list, the first list fixes the first validators, and
   each later list is signed by the validators of the list before it, with a
   receipt into a root the earlier list anchored. No genesis, no snapshot, no
   node is trusted — and the walk crosses a network restart, which the
   major-block spine (`ProofService`, #4058) cannot.
7. **Anchoring never touches consensus.** Anchors are read-side: no executor,
   no block, no `hashChains` input depends on whether or when an anchor was
   made. A network with anchoring stalled runs as before.
8. **A proof is a package.** Once built, an evidence package is verified with
   Bitcoin alone. It does not expire, and no Accumulate node need exist for it
   to be checked.

### What it proves, and what it does not

- It proves that the record existed by the Bitcoin block's time, that the
  network's validators attested to the root it sits under, and — through the
  account's main chain — that the transaction executed, which on Accumulate
  means its authorities' thresholds were met.
- It does not prove the decision was right. It is a proof of record.
- It does not prove anything before the first anchor through Bitcoin. Before
  that the existing spine applies, and the spine does not cross the 13 July
  2025 restart. Promises of preserved proof begin at the first anchor.
- Finality takes the anchor interval plus Bitcoin confirmations (six). The
  interval is a cost: at $0.12–$1 a transaction, every ten minutes is about
  $6,300–$52,600 a year, hourly $1,050–$8,800, daily $44–$365.
- A verifier that wishes to re-derive, rather than trust the quorum for, *who*
  was entitled to approve must also hold the key pages' history at the signing
  height. The proof builder can be configured to keep those accounts too; the
  default package trusts the quorum's attestation that the threshold was met.
- The customer keeps the document. The package carries its hash; a lost
  document leaves a hash that proves nothing. A short, guessable document
  should be hashed with a random salt kept beside it, or its hash is a
  dictionary away from its contents.
- Past anchors survive a break of secp256k1: proof of work protects the blocks
  that hold them. Only future anchors would need new keys.

## 2. Specification — how it is implemented

### 2.1 Accounts

Under the Directory's identity:

| Account | Governed by | Holds |
|---|---|---|
| `dn.acme/bitcoin/addresses` | `dn.acme/operators` at the network threshold | address lists (§2.2) |
| `dn.acme/bitcoin/anchors` | a key page of the current validators' keys, threshold 1 | coordination entries (§2.4) |

Both are ordinary data accounts. Every entry is a `DoubleHashDataEntry` whose
first `Data` is the JSON of the record below and whose remaining `Data` are
empty. The entry's transaction executes under the account's authority as any
`WriteData` does; the signer of a coordination entry is read from its
signatures.

### 2.2 The address list

```json
{
  "type": "address-list",
  "version": 3,
  "effective": 41203000,
  "network": "MainNet",
  "threshold": 14,
  "signers": [
    {"validator": "acc://dn.acme/operators/1#…", "key": "02…"},
    …
  ],
  "descriptor": "wsh(sortedmulti(14,02…,02…,…))",
  "addresses": ["bc1q…"],
  "interval": 60,
  "fee": {"policy": "economy", "maxSatPerVB": 40}
}
```

- `effective` is the Directory height from which anchors must come from this
  list's addresses. Anchors at or above `effective` from the previous list's
  addresses are not anchors.
- `signers` map each Bitcoin public key to the validator that holds it, by the
  validator's key hash on the operator page. A validator holds exactly one key
  per list.
- `threshold` is `ceil(len(signers) × ValidatorAcceptThreshold)`.
- `descriptor` is a BIP 380 output descriptor. With 20 or fewer signers it is
  `wsh(sortedmulti(k, …))`; above 20 it is `tr(…, multi_a(k, …))`
  (`OP_CHECKSIGADD`, BIP 342). `addresses` are derived from it and listed for
  readers that do not parse descriptors.
- `interval` is in Directory blocks. An anchor is due at every height that is
  a multiple of it.
- `fee` names the priority and caps the rate. Replace-by-fee is always
  signalled.

A new list is published to rotate keys, to follow a validator-set change, or
to recover from a compromise. The difference between the three is operational,
not in the record. Backup lists are prepared ahead: their keys exist and are
held offline, and the list is published, with an `effective` height, only when
it is needed.

### 2.3 The anchor transaction

- Inputs: one or more outputs of the list's addresses.
- Outputs: one `OP_RETURN` carrying the payload, and one change output to a
  list address. Nothing else.
- The payload, 52 bytes:

  | bytes | field |
  |---|---|
  | 0–3 | `ACC1` — magic and version |
  | 4–11 | the first 8 bytes of SHA-256 of the lower-cased network name |
  | 12–19 | the Directory block height, big-endian |
  | 20–51 | the Directory's `RootChainAnchor` at that height |

- The root is the Directory's root chain anchor, because that is what a chain
  receipt terminates at (database.md, "Proofs are read, not searched").
  Partition roots reach it through the Directory's anchor chain; a package for
  a BVN entry carries that leg too.
- Anchors are independent transactions. Nothing requires one to spend the
  previous one's output; order comes from the height in the payload and the
  coordination record. A stuck transaction therefore blocks nothing: it is
  fee-bumped (RBF) or abandoned, and the next anchor proceeds.

### 2.4 Coordination

At every due height, in order:

1. **Proposal.** The validator whose index in `signers` equals
   `height / interval mod len(signers)` writes
   `{"type":"proposal","height":H,"root":R,"psbt":"…"}`: the unsigned
   transaction as a PSBT (BIP 174). If no proposal for `H` appears within one
   interval, any validator may propose; the first proposal for `H` to execute
   is the one.
2. **Shares.** Every validator reads the proposal, and checks:
   - its own node's root chain anchor at `H` is `R`;
   - the PSBT has exactly the outputs of §2.3 with the payload for `(H, R)`;
   - every input is an output of a list address, and the fee is within the
     list's policy.

   If all hold it writes `{"type":"share","height":H,"txid":T,"sig":"…"}`, its
   partial signature over the PSBT, `T` being the unsigned transaction's ID.
   If any fails it writes nothing. A validator writes at most one share per
   height.
3. **Broadcast.** When shares from `threshold` distinct validators exist, the
   proposer — or, after one interval, anyone — finalizes and broadcasts, and
   writes `{"type":"broadcast","height":H,"txid":"…"}`.
4. **Fee bump.** If the transaction is unconfirmed after one interval and the
   policy allows a higher rate, steps 1–3 repeat for the same `H` with a
   replacement PSBT; the record keeps both, and the one that confirms is the
   anchor.

Coordination entries are read by every validator, so the process needs no
channel but the network itself, and leaves its whole history on it.

### 2.5 The proof builder

A service beside a follower's API, configured with the accounts it keeps proofs
for. It processes block ledgers as they are written:

1. For each block `B` of each partition, read `ledger/<B>` and take every
   `BlockEntry` whose `Account` is configured. Each names a chain and an index.
2. For each, build the receipt from the entry to the partition's root chain
   anchor at `B`, by reads (database.md: one read per level, no search).
3. When the Directory block `D` that commits the partition's anchor for `B` is
   known, extend the receipt through the Directory's anchor chain to the
   Directory's root chain anchor at `D`.
4. When the anchor at the first due height `H ≥ D` confirms, extend to `H`
   (a receipt to a later root of the same chain is one more leg) and record
   the Bitcoin transaction.
5. Serve the package by account and entry, or by transaction hash.

It also keeps, for every list and coordination entry, the receipt into the next
anchor, since every package carries the anchor's record.

**The evidence package:**

```json
{
  "network": "MainNet",
  "account": "acc://clearedcounterparty.acme/decisions",
  "entry": {"txid": "…", "hash": "…", "data": ["…"]},
  "receipt": { … },
  "directory": {"height": 41203060, "root": "…"},
  "anchor": {
    "txid": "…", "block": "…", "blockHeight": 920344,
    "record": {"proposal": {…}, "shares": [{…}], "receipt": { … }}
  },
  "list": {"entry": {…}, "receipt": { … }, "chain": [ … ]}
}
```

`receipt` runs from the entry to `directory.root`; `anchor.record.receipt`
proves the coordination entries into the root of the *next* anchor;
`list.chain` is the sequence of list entries from the first to the one in
force at `directory.height`, each with its receipt into a root the list before
it anchored.

### 2.6 The verifier

Given a package, the first anchor's transaction ID, and Bitcoin:

1. Fetch the first anchor; decode its payload; the address it was sent from is
   on the first list, and the first list's receipt ends at that payload's root.
   Walk `list.chain`: each entry executed under the operator book at the
   threshold, signed by validators of the previous list, with its receipt into
   a root that an anchor from the previous list committed. This yields the list
   in force at `directory.height`.
2. Fetch `anchor.txid`. Its payload is `(directory.height, directory.root)`;
   its input is an output of a list address; it has `n ≥ 6` confirmations.
3. Check `receipt` from `entry.hash` to `directory.root`.
4. Check the anchor's record: a proposal for `(height, root)`, shares from
   `threshold` distinct validators of the list, each entry signed by its
   validator's key, each share a valid partial signature over the proposal's
   PSBT; and `anchor.record.receipt` into the next anchor's root, that next
   anchor itself passing step 2. If there is no next anchor yet, the result is
   *provisional*.
5. Report: the entry, the height, the Bitcoin block and its time, and
   *final* or *provisional*.

The verifier is the only observer the scheme needs. Run continuously against a
node it also raises two alarms, both liveness, not safety:

- **Overdue:** no confirmed anchor within two intervals of a due height.
- **Unrecorded spend:** an output of a list address was spent by a transaction
  with no broadcast entry. That is a key compromise; the response is the next
  list.

### 2.7 What is pinned out of band

One value: the first anchor's Bitcoin transaction ID. It is published in the
documentation and on the network's website, and nothing else is. Every other
fact the verifier uses is derived from it through Bitcoin and the receipts in
the package.

### 2.8 Layout

| Component | Where |
|---|---|
| Payload, list and record types | `pkg/anchoring` |
| Validator coordination (propose, share, broadcast, bump) | `internal/anchoring`, started by the Directory's validator service when a list names its key |
| Proof builder and package service | `internal/anchoring/proofs`, started by a follower's API when configured |
| Verifier and alarms | `tools/cmd/anchor-verify` |

Bitcoin access is through a configured node's RPC or an Electrum-style light
client; the verifier accepts either.
