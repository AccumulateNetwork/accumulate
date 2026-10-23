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
  per slot of `intervalBlocks` Bitcoin blocks, a parameter of the list in
  force; the proposer chooses the Directory block.
- **The address list.** A data entry in the network's address-list account
  naming the Bitcoin keys the validators sign anchors with, which validator
  holds each, the threshold, and the Directory height the list takes effect
  at. The account is governed by the network's operator book. A list is valid
  only as a successor of the list before it: by a rotation that needs both of
  that list's key sets, or by activating the **backup list** it committed to
  in advance — keys generated and held offline — when the list in use is
  compromised (§2.2).
- **The coordination accounts.** One data account per signer, named in the
  list, that only that signer can append to. An anchor is proposed there, each
  validator records its signature share there, and the broadcast is recorded
  there. Every entry is signed by the validator's Accumulate key.
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
   account holds a proposal for (height, root) with signature shares from at
   least the list's `threshold` of the list's `signers`, each signed by that
   validator's Accumulate key. A transaction from a listed address with no
   record behind it is a theft of coins, not an anchor.
2. **The threshold follows the network's, by count.** "The validators" are
   the Directory's active validators in the network definition (its
   committee, consensus.md "Epochs"). The Bitcoin multisig requires the same
   fraction of them as `ValidatorAcceptThreshold`, counted in keys, because a
   Bitcoin multisig counts keys. Consensus thresholds are in stake
   (consensus.md "The thresholds"), so with unequal stakes a count threshold
   can be met by less than a stake quorum. The stored `threshold` is
   therefore a floor that operators may raise (§2.2), not a guarantee equal to
   the network's.
3. **A validator signs only its own root.** A validator contributes a share
   for (height, root) only if its own node holds `root` as the Directory's
   root chain anchor at `height`. The multisig is the check; no separate
   watcher compares anchors to the network.
4. **The list is authoritative; the keys are not.** Which addresses may anchor
   is decided on Accumulate: a list is valid when the operator book wrote it
   and it succeeds the list before it, either by a rotation signed by a
   threshold of that list's Accumulate keys and committed by an anchor from
   its addresses, or by activating the backup list it named in advance, which
   needs only the backup's own keys (§2.2). Stolen Bitcoin keys cannot change
   the list, cannot make a record, and so cannot make an anchor. Recovery
   from a live compromise is activating the backup; it needs no spend from the
   old addresses. A party holding both key sets of a past list is the stated
   limit (§2.2 threat table).
5. **Each anchor is confirmed by a later one.** Anchor *i* is *final* when the
   first confirmed anchor *j* whose height `H_j` is at or above the Directory
   height of the last record entry of *i* (its broadcast, or its last share)
   exists, and *j*'s own record verifies. *j* may itself be provisional. The
   newest anchor is *provisional*. A verifier reports which.
6. **Trust begins at one pin and proceeds by induction.** The verifier is
   given the Bitcoin transaction ID of the first anchor, out of band. That
   anchor fixes the first list; each later list is a successor of the one
   before it by rotation or backup activation. No genesis, no snapshot, no
   node is trusted — and the walk crosses a network reorganization, which the
   major-block spine (`ProofService`, #4058) cannot.
7. **No consensus rule depends on anchoring.** No executor, no block, no
   `hashChains` input depends on whether or when an anchor was made. The
   coordination writes are ordinary transactions, and the executor accepts
   every `WriteData` as it does today. Every "invalid" or "ignored" below is a
   read-side rule applied by validators' anchoring software and by the
   verifier. A network with anchoring stalled runs as before.
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
  2025 reorganization (`docs/incidents/2025-07-13-reorg-history-loss.md`).
  Promises of preserved proof begin at the first anchor.
- Finality takes the anchor interval plus Bitcoin confirmations (six). The
  interval is a cost: at $0.12–$1 a transaction, an anchor every Bitcoin
  block (about ten minutes) is about $6,300–$52,600 a year, hourly $1,050–$8,800, daily $44–$365.
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

Throughout, "invalid", "ignored" and "refuse" are read-side rules applied by
validators' anchoring software and by the verifier. The executor accepts every
`WriteData` to these accounts as it does today (invariant 7). Every receipt in
a record or package is the `merkle.Receipt` JSON the API serves: `start`,
`entries` of `{hash, right}`, and `anchor`. All other bytes in a record or
package — hashes, roots, transactions, signatures, keys — are hex strings.

No rule below depends on Accumulate block time. A Directory leader's header
timestamp is not checked by any voter (DIFFERENCES C23), so a rule on it would
stall on one bad header. The clock is Bitcoin's block height, which no single
validator controls. The verifier may report Accumulate block time as
informational only; the Bitcoin block is the time bound.

### 2.1 Accounts

`dn.acme/bitcoin` is an ADI (`CreateIdentity`) under the Directory's identity.
Under it:

| Account | Governed by | Holds |
|---|---|---|
| `dn.acme/bitcoin/addresses` | `dn.acme/operators` | address lists and fee entries (§2.2) |
| one coordination account per signer, named in the list (§2.2) | that signer's own key book only | that signer's coordination entries (§2.4) |

Each coordination account is a data account created by the operators at list
creation, with its own key book holding only that signer's `accKey` at
threshold 1. Its authority is its own book only. A signer that runs
`UpdateAccountAuth` on it locks only itself out; the others keep anchoring, and
the next list names a replacement. Each validator funds its own account's page
with credits (`AddCredits`); the credits-low alarm is per signer (§2.6).

Records are read from every coordination account the list in force names,
ordered by Directory block height, ties by signer index. The verifier and the
builder take the account URLs from the list, not from this spec.

All accounts are ordinary data accounts. On an existing network the ADI, the
books and the accounts are created by operator-signed `CreateIdentity`,
`CreateKeyBook` and `CreateDataAccount` transactions, not by an executor
change. Every entry is a `DoubleHashDataEntry` with exactly one `Data` element,
the JSON of the record below. The JSON is parsed, not canonical; the entry hash
is over the bytes as written. The entry's transaction executes under the
account's authority as any `WriteData` does; the signer of a coordination entry
is read from its signatures.

### 2.2 The address list

```json
{
  "type": "address-list",
  "version": 3,
  "effective": 41203000,
  "network": "mainnet",
  "threshold": 14,
  "signers": [
    {"validator": "acc://dn.acme/operators/1#…", "accKey": "…",
     "key": "02…", "coordination": "acc://dn.acme/bitcoin/c1"},
    …
  ],
  "descriptor": "wsh(sortedmulti(14,02…,02…,…))",
  "addresses": ["bc1q…"],
  "intervalBlocks": 6,
  "maxLag": 1000,
  "next": "…"
}
```

- `effective` is a Directory block height from which anchors must come from
  this list's addresses. It must be strictly greater than the Directory height
  of the block holding the list entry (the package carries that height, §2.5),
  and strictly greater than the previous valid list's `effective`. Lists are
  applied in main-chain order of `dn.acme/bitcoin/addresses`. A list that
  breaks either rule is ignored by validators and verifiers. The list in force
  at height `h` is the latest valid list with `effective ≤ h`. Anchors at or
  above `effective` from the previous list's addresses are not anchors.
  Operators should set `effective` at least one slot after publication.
- `signers` map each Bitcoin public key to the validator that holds it, by the
  validator's key hash on `dn.acme/operators/1`, and bind `accKey`, the hash of
  the Accumulate public key whose signatures count for that signer. The
  verifier checks records and list signatures against `accKey`. A node-key
  rotation therefore takes effect for anchoring only through a new list. The
  signers are the Directory's active validators at publication. A validator
  holds exactly one key per list.
- `threshold` is by count: `ceil(len(signers) × ValidatorAcceptThreshold)` at
  publication, stored in the list, and equal to the descriptor's `k`; a list
  where they differ is invalid. The verifier uses the stored value. Validators
  refuse to sign shares under a list whose stored threshold is below the value
  derived at the time it was published; operators may store a higher one.
- `descriptor` is a BIP 380 output descriptor. With 20 or fewer signers it is
  `wsh(sortedmulti(k, …))`; above 20 it is `tr(NUMS, multi_a(k, …))`
  (`OP_CHECKSIGADD`, BIP 342), where the internal key `NUMS` is the BIP 341
  point `50929b74c1a04954b78b4b6035e97a5e078a5a0f28ec96d547bfee9ace803ac0`, so
  that no one can spend by the key path. `addresses` are derived from it and
  listed for readers that do not parse descriptors.
- `intervalBlocks` is in Bitcoin blocks (6 is about an hour). The slot of
  Bitcoin height `b` is `floor(b / intervalBlocks)` (§2.4).
- `maxLag` is the number of stored Directory blocks behind its own tip beyond
  which a validator declines a proposed `H` (§2.4).
- `next` is the `sha256` of the descriptor string of the list's backup list.
  The descriptor fixes the keys, threshold and addresses; `effective`, the
  `signers` mapping and the rest are supplied at activation.

**Fee entries.** `fee` and the smallest spendable input are not in the list.
They are operator-signed entries on `dn.acme/bitcoin/addresses`:

```json
{"type": "fee", "minSatPerVB": 2, "maxSatPerVB": 40, "maxFeeSat": 50000,
 "maxInputs": 50, "minInputSat": 10000}
```

applied from the block holding the latest such entry. Validators read the
latest; verifiers never need it. The rate must lie in `[minSatPerVB,
maxSatPerVB]`, the absolute fee must not exceed `maxFeeSat`, and the input
count must not exceed `maxInputs`. The 20 KB transaction limit
(`protocol/data_entry.go:27`) bounds the PSBT. Replace-by-fee is always
signalled.

**Succession.** The first list is fixed by the pinned first anchor. Every later
list is a successor of the list before it by one of two routes.

- **Normal rotation.** List N+1 is valid when: (a) its transaction's principal
  is `dn.acme/bitcoin/addresses` and the receipt proves it on that account's
  main chain (only a successful transaction reaches a main chain); (b) its
  transaction carries signatures from at least N's `threshold` of N's signers'
  `accKey`s; (c) its receipt ends at a root committed by a **confirmed anchor
  from N's addresses** (payload, address, six confirmations; its record is not
  required here, because (b) already needs N's Accumulate keys); (d) the
  `effective` rules above hold. A forged rotation costs a threshold of **both**
  of N's key sets, the same as a forged anchor under N (invariant 1).
- **Backup activation.** List N+1 is valid when: `sha256(descriptor) ==
  N.next`; (a) and (d) hold; and its receipt ends at a root committed by a
  confirmed anchor from **N+1's own addresses**, at or above its `effective`,
  whose record (proposal and `threshold` shares) is signed by N+1's signers'
  `accKey`s. No signature and no spend from N is needed, so it works with N's
  keys lost or stolen.
- **Fork rule.** If two lists both satisfy the rules as successors of N, a
  backup activation beats a normal rotation; between two normal rotations the
  one whose committing anchor confirmed first in Bitcoin wins. The verifier
  checks the chain the package presents and applies the rule only when shown
  both. Detecting a competing chain is the network's job: every spend from a
  list address with no broadcast record is the unrecorded-spend alarm, and the
  answer is to activate the backup, which supersedes the attacker's branch by
  the fork rule.
- **Stated limit.** A party holding a threshold of both key sets of a past
  list can present a verifier with an alternate history from that list onward.
  That is the same party that could forge anchors under that list. The promise
  is: no forgery with fewer than both key sets of one list, and live theft is
  answered by the backup. Operators destroy both key sets of a retired list.

While the old list is live its next anchor also commits the new list entry; a
verifier may use that as additional evidence and must not require it. The new
addresses are funded by a `sweep` (§2.3) when the old keys are intact,
otherwise by a plain payment to a new-list address.

**Rotation.** Every change of the Directory's committee that adds or removes a
validator requires a new list, published so that its `effective` falls within
two intervals (`intervalBlocks` Bitcoin blocks each) of the change taking
effect. This is an operational requirement, not a consensus rule. Until
`effective` the old list is in force and its shares count. A new list is also
published to rotate keys. Backup lists are prepared at every list change with
one offline key per signer, are two deep (the backup's own `next`), and are
re-prepared when the committee changes. Custody of the offline keys is an
operational matter.

**Threat table.** What a party holding each key set can do, and what stops it.

| Holds | Can do | Stopped by |
|---|---|---|
| Current Bitcoin keys alone (threshold) | Spend list coins in a transaction with no record | The transaction is not an anchor (invariant 1); the unrecorded-spend alarm fires; the backup is activated |
| Current Accumulate keys alone (threshold) | Write proposals and shares | Shares without Bitcoin signatures spend nothing; a wrong root is refused by honest validators (invariant 3) |
| Both current key sets | Forge anchors and a normal rotation | Nothing; it is a threshold of the validators, the same as the network's trust. Answer: the backup (limit above) |
| Both key sets of a past list | Present an alternate history from that list onward | Stated limit; operators destroy retired keys |
| Backup offline keys alone | Nothing | The backup is valid only with its own anchor and record, which need its Bitcoin and Accumulate keys; the key sets are separate |
| One retired validator's node key | Sign as that validator on lists that still name its `accKey` | Below threshold alone; the next list drops it (rotation); `accKey` binds a key per list |
| One far-ahead leader header | Move Accumulate block time | No rule depends on block time (§2); C23 is Paul's to bound |
| One locked-out signer | Lock only its own coordination account | Per-signer accounts (§2.1); the others keep anchoring; the next list replaces it |

### 2.3 The anchor transaction

- Inputs: one or more outputs of the list's addresses, each of at least
  `minInputSat`. Any payment to a list address funds it. Each input has at
  least one confirmation in the validator's own Bitcoin view and is not an
  input of an unconfirmed recorded anchor or sweep, except that a replacement
  (§2.4 step 4) spends exactly the inputs of the transaction it replaces.
- Outputs: one `OP_RETURN` carrying the payload, and at most one change output
  to a list address; the change output is omitted when it would be below the
  dust limit. Nothing else.
- Signatures: `SIGHASH_ALL` for `wsh` inputs and `SIGHASH_DEFAULT` for `tr`
  inputs. A share with any other sighash type is invalid.
- The payload, 52 bytes:

  | bytes | field |
  |---|---|
  | 0–3 | `ACC1` — magic and version |
  | 4–11 | the first 8 bytes of SHA-256 of the network ID |
  | 12–19 | the Directory block height `H`, big-endian |
  | 20–51 | `R`, the Directory's `RootChainAnchor` at `H` |

  The network ID is `GenesisConfig.NetworkID` lower-case, e.g. `"mainnet"`;
  the package's `network` field is the same string. Heights never restart
  within a network ID; a re-genesis that restarts heights is a new network ID.
- The root is the Directory's root chain anchor, because that is what a chain
  receipt terminates at (database.md, "Proofs are read, not searched").
  Partition roots reach it through the Directory's anchor chain; a package for
  a BVN entry carries that leg too.
- Anchors are independent transactions. Nothing requires one to spend the
  previous one's output; order comes from the height in the payload and the
  coordination record. A stuck transaction therefore blocks nothing: it is
  fee-bumped (RBF) or abandoned, and the next anchor proceeds.
- A **sweep** is a recorded spend with a proposal, shares and a broadcast as
  for an anchor, but its transaction has no `OP_RETURN` and its outputs go only
  to addresses of the list in force or of a **pending** list: one on
  `dn.acme/bitcoin/addresses` that meets (a), (b) and (d) of its succession
  route and awaits its committing anchor. Its proposal is
  `{"type":"sweep","slot":s,"to":<list version>,"psbt":"…"}`. The
  unrecorded-spend alarm does not fire for it. It moves funds at a list change
  and consolidates inputs.

### 2.4 Coordination

**Slots.** The clock is Bitcoin's. The slot of a Bitcoin height `b` is
`s = floor(b / intervalBlocks)`. The designated proposer for slot `s` is
`signers[s mod len(signers)]`.

At each slot, in order:

1. **Proposal.** The designated proposer writes
   `{"type":"proposal","slot":s,"height":H,"root":R,"psbt":"…"}`: the unsigned
   transaction as a PSBT (BIP 174). The proposer chooses `H`: a stored
   Directory block its node holds, strictly above the `H` of the latest
   recorded broadcast; `R` is that block's `RootChainAnchor`. Several
   proposals for a slot may exist. A validator shares at most one proposal per
   slot, the first valid one in Directory main-chain order. A proposal from a
   non-designated signer counts for the validator only once its own Bitcoin
   tip is at least `s × intervalBlocks + ceil(intervalBlocks / 2)`. These
   designated-proposer and timeout rules are liveness rules for validators: the
   verifier checks only `(H, R)`, the address, the confirmations and the
   record. A split costs the slot; the next slot has another proposer.
2. **Shares.** Every validator reads the proposal and checks:
   - its own node holds `H` with root `R`, and `H` is above the `H` of the
     latest recorded broadcast; as a local liveness check it may decline an `H`
     more than `maxLag` stored blocks behind its own tip;
   - the PSBT has exactly the outputs of §2.3 with the payload for `(H, R)`;
   - every input meets §2.3, is an output of a list address of at least
     `minInputSat`, and its prevout (txid, vout, scriptPubKey, amount) matches
     the validator's **own** Bitcoin node, not the PSBT's `witness_utxo`
     (CVE-2020-14199);
   - the sighash type is as §2.3, and the fee rate, absolute fee and input
     count are within the latest fee entry's bounds.

   If all hold it writes `{"type":"share","height":H,"txid":T,"sig":"…"}`, its
   partial signature over the PSBT, `T` being the unsigned transaction's ID and
   `sig` the BIP 174 partial signature hex (`PSBT_IN_PARTIAL_SIG` for `wsh`,
   `PSBT_IN_TAP_SCRIPT_SIG` for `tr`). If any check fails it writes nothing.
   At most one share per validator per `(H, T)` counts.
3. **Broadcast.** When shares from `threshold` distinct signers exist, the
   proposer — or, after `intervalBlocks` Bitcoin blocks, anyone — finalizes and
   broadcasts, and writes `{"type":"broadcast","height":H,"txid":"…"}`.
4. **Fee bump.** If the transaction is unconfirmed after `intervalBlocks`
   Bitcoin blocks and the fee bounds allow a higher rate, steps 1–3 repeat for
   the same `H` with a replacement PSBT. A replacement spends exactly the same
   inputs and has a strictly higher fee rate; its absolute fee rises by at
   least the incremental relay fee times its size (BIP 125). There is at most
   one replacement per `intervalBlocks` blocks. The record keeps both, and the
   one that confirms is the anchor.

Coordination entries are read by every validator, so the process needs no
channel but the network itself, and leaves its whole history on it.

### 2.5 The proof builder

A service beside a follower's API, configured with the accounts it keeps proofs
for. It processes block ledgers as they are written:

1. For each block `B` of each partition, read the `BlockLedger(B)` record on
   the partition's system ledger account and take every `BlockEntry` whose
   `Account` is configured. Each names a chain and an index.
2. For each, build the receipt from the entry to the partition's root chain
   anchor at `B`, by reads (database.md: one read per level, no search).
3. When the Directory block `D` that commits the partition's anchor for `B` is
   known, extend the receipt through the Directory's anchor chain to the
   Directory's root chain anchor at `D`.
4. When the first **confirmed** anchor with `H ≥ D` has six confirmations,
   re-prove the Directory root-chain entry at `D` to the root-chain anchor at
   `H`. This replaces the final leg with a receipt to a different height, the
   case database.md says searches; the builder does it once per `(D, H)` and
   every package under that anchor shares the leg. Record the Bitcoin
   transaction, its block and height only after six confirmations. The builder
   tracks every txid proposed for `H`; on a reorganization it re-derives.
   Before then `directory` is `D`; after, it is `H`: the field changes meaning,
   and the package says which by the presence of `anchor`.
5. Serve the package by account and entry, or by transaction hash.

It also keeps, for every list and coordination entry, the receipt into the next
anchor, since every package carries the anchor's record.

**The evidence package.** `E` below is `{"transaction": "…", "signatures":
["…"]}`: the full transaction (header and body) in Accumulate's binary
encoding, and its signature objects in binary encoding, all hex.

```json
{
  "network": "mainnet",
  "account": "acc://clearedcounterparty.acme/decisions",
  "entry": {
    "chain": "main", "index": 7, "hash": "…",
    "transaction": "…", "signatures": ["…"]
  },
  "receipt": { … },
  "directory": {"height": 41203060, "root": "…"},
  "anchor": {
    "txid": "…", "block": "…", "blockHeight": 920344,
    "record": {
      "proposal": {"E": E, "receipt": { … }},
      "shares": [{"E": E, "receipt": { … }}],
      "broadcast": {"E": E, "receipt": { … }}
    },
    "final": {
      "txid": "…", "block": "…", "blockHeight": 920351,
      "record": {"proposal": E, "shares": [E]}
    }
  },
  "bitcoin": {"header": "…", "merkleProof": "…", "prevTxs": ["…"]},
  "list": {"chain": [
    {"entry": E, "directoryHeight": 41202000, "receipt": { … }, "anchorTxid": "…"}
  ]}
}
```

- `entry.hash` is the chain entry, which for a data account's main chain is the
  **transaction hash**. `transaction` and `signatures` let the verifier link
  the entry to its data and check who signed it. Any data carried for the
  reader must equal `transaction.body.entry.data`.
- `receipt` runs from `entry.hash` to `directory.root`.
- `anchor.record` holds one receipt per `E`, each into the root of anchor *j*
  (`anchor.final`). After a fee bump the proposal checked is the one whose
  unsigned txid matches `anchor.txid`. `anchor.final` is *j*; its record is
  checked by signatures only, with no receipt, which ends the recursion. It is
  absent when the anchor is provisional.
- `bitcoin` is, for the anchor and for *j*, the `gettxoutproof` form: the block
  `header`, the `merkleProof`, and `prevTxs`, the funding transactions, so that
  partial signatures can be checked against the prevout amounts and scripts. A
  full node with `txindex` may skip it.
- `list.chain` is the sequence from the first list to the one in force at
  `directory.height`; each item is the list entry, the Directory height of the
  block holding it, its receipt into the root of its committing anchor, and
  that anchor's Bitcoin txid.
- Each share's `sig` is the BIP 174 partial signature, hex (§2.4).

### 2.6 The verifier

Given a package, the first anchor's transaction ID, and Bitcoin:

1. Fetch the first anchor; decode its payload; the address it was sent from is
   on the first list, and the first list's receipt ends at that payload's root.
   Walk `list.chain`: each next list is a successor of the one before it by
   normal rotation or backup activation (§2.2), with `effective` checked
   against the carried `directoryHeight`. This yields the list in force at
   `directory.height`.
2. Check `bitcoin` against `anchor.txid`: the header and Merkle proof (SPV, or
   a full node's own view). The payload is `(directory.height,
   directory.root)`; the input is an output of a list address; it has `n ≥ 6`
   confirmations. Ignore the package's `block` if it disagrees.
3. Check `hash(transaction) == entry.hash`; the body is `WriteData` with a
   `DoubleHashDataEntry` of exactly one `Data` element; and the receipt from
   `entry.hash` to `directory.root`. Check the entry's signatures.
4. Check the anchor's record: a proposal for `(height, root)`, shares from
   `threshold` distinct signers of the list in force (the stored count), each
   entry signed by its signer's `accKey`, each share a valid partial signature
   over the proposal's PSBT, with each `E` receipted into the root of *j*. *j*
   is the first confirmed anchor whose `H_j` is at or above the Directory
   height of the last record entry of this anchor; check *j* (step 2) and its
   own record by signatures only. *j* may itself be provisional. If there is no
   such *j* yet, the result is *provisional*.
5. Report: the entry, the height, the Bitcoin block and its time, and
   *final* or *provisional*. The entry existed before the confirming block was
   mined. The header time is approximate, in either direction, by about two
   hours. A conservative upper bound is the median time past of the block
   eleven blocks after the confirming block, on the assumption that most hash
   power stamps honestly; the report says so.

A package is checked with Bitcoin alone. The alarms are a separate watcher
that needs a Directory node to read the broadcast entries; run continuously
against one it raises three alarms, all liveness, not safety:

- **Overdue:** no confirmed anchor in three slots.
- **Unrecorded spend:** an output of a list address was spent by a transaction
  with no broadcast entry, other than a recorded sweep. That is a key
  compromise; the response is the backup.
- **Credits low:** a signer's coordination account is short of credits for the
  writes of the coming slots.

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

**Key placement (a requirement of B2).** The Bitcoin key is held by a signer
separate from the node process, its own process or an HSM. It receives
`(H, R, PSBT)` and checks `R` against its own node before signing. The node's
key signs the coordination record. Without this, a forged anchor needs only one
compromised host per validator.

Bitcoin access is through a configured node's RPC or a light client; the
verifier accepts either. A light client must validate block headers and proof
of work (SPV); an Electrum-style server without that check is a trusted party.
