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
  holds each, and the threshold. The account is governed by the network's
  operator book. A list is valid only as a successor of the list before it: by
  a **rotation spend** — a recorded spend from that list's addresses whose
  `OP_RETURN` names the new list, so it needs both of that list's key sets —
  or by activating the **backup list** it committed to in advance, in full,
  with keys generated and held offline, when the list in use is compromised
  (§2.2).
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
   an address on the list in force at that height, *and* the list's
   coordination accounts hold a proposal for (height, root) with signature
   shares from at least the list's `threshold` of the list's `signers`, each
   signed by that validator's Accumulate key. A transaction from a listed
   address with no record behind it is a theft of coins, not an anchor.
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
4. **The list is authoritative; no one key set is.** Which addresses may
   anchor is decided on Accumulate and in Bitcoin together: a list is valid
   when the operator book wrote it and it succeeds the list before it, either
   by a rotation spend from that list's addresses with a record signed by its
   Accumulate keys, or by activating the backup list it fixed in advance,
   which needs the backup's own Bitcoin and Accumulate keys (§2.2). The
   operator book alone cannot change the list; stolen Bitcoin keys alone
   cannot change the list, make a record, or make an anchor. Recovery from a
   live compromise is activating the backup within its window; it needs no
   spend from the old addresses. A party holding both key sets of a list is
   the stated limit (§2.2 threat table).
5. **Each anchor is confirmed by a later one.** Anchor *i* is *final* when a
   confirmed anchor *j* whose height `H_j` is at or above the Directory
   height of the last record entry of *i* (its broadcast, or its last share)
   exists, and *j*'s own record verifies. The builder uses the first such
   *j*. *j* may itself be provisional. The newest anchor is *provisional*. A
   verifier reports which.
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
  block (about ten minutes) is about $6,300–$52,600 a year, hourly
  $1,050–$8,800, daily $44–$365.
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

Each coordination account is a data account with its own key book holding
only that signer's `accKey` at threshold 1. Creating an account with a named
authority needs that authority's signature
(`internal/core/execute/v2/chain/create_utils.go:48-51`), so each signer
cosigns its own book's and account's creation with the operators. A list may
name a signer's account from an earlier list; a new signer's account is
created when the list, or the backup that names it, is prepared. Validators
and the verifier ignore a signer's entries in an account whose book does not
hold that signer's `accKey` at threshold 1. Its authority is its own book
only. A signer that runs `UpdateAccountAuth` on it locks only itself out; the
others keep anchoring, and the next list names a replacement. Each validator funds its own account's page
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
  "backupWindow": 1008,
  "next": "…",
  "nextAddresses": ["bc1q…"]
}
```

- **Height and order.** Let `h_L` be the Directory height of the block holding
  list `L`'s entry (the package carries it, §2.5). Once valid, `L` is in force
  for Directory heights strictly above `h_L`; the list in force at `h` is the
  latest list of the valid chain with `h_L < h`. Anchors at heights above
  `h_L` from the previous list's addresses are not anchors. Lists are read in
  main-chain order of `dn.acme/bitcoin/addresses`, so `h_L` only grows. A list
  takes no chosen start height: it starts where it was written, and is in
  force only once its succession (below) is complete.
- `signers` map each Bitcoin public key to the validator that holds it, by the
  validator's key hash on `dn.acme/operators/1`, and bind `accKey`, the hash of
  the Accumulate public key whose signatures count for that signer. The
  verifier checks records against `accKey`. A node-key rotation therefore
  takes effect for anchoring only through a new list. The signers are the
  Directory's active validators at publication. A validator holds exactly one
  key per list.
- `threshold` is by count: `ceil(len(signers) × ValidatorAcceptThreshold)` at
  publication, stored in the list, and equal to the descriptor's `k`; a list
  where they differ is invalid. The verifier uses the stored value. Validators
  refuse to sign shares under a list whose stored threshold is below the value
  derived at the time it was published; operators may store a higher one.
- `descriptor` is a BIP 380 output descriptor, written without its checksum.
  With 20 or fewer signers it is `wsh(sortedmulti(k, …))`; above 20 it is
  `tr(NUMS, multi_a(k, …))` (`OP_CHECKSIGADD`, BIP 342), where the internal key
  `NUMS` is the BIP 341 point
  `50929b74c1a04954b78b4b6035e97a5e078a5a0f28ec96d547bfee9ace803ac0`, so that
  no one can spend by the key path. `addresses` are derived from it and listed
  for readers that do not parse descriptors.
- `intervalBlocks` is in Bitcoin blocks, at least 1 (6 is about an hour). The
  slot of Bitcoin height `b` is `floor(b / intervalBlocks)` (§2.4).
- `maxLag` is the number of stored Directory blocks behind its own tip beyond
  which a validator declines a proposed `H` (§2.4).
- `backupWindow` is in Bitcoin blocks (1008 is about a week): how long after
  this list is succeeded by a rotation spend its backup may still be
  activated over that succession (fork rule).
- `next` is the `sha256` of the backup list, in full, in the canonical form of
  RFC 8785 (JSON Canonicalization Scheme), without its `version` field. The
  backup is therefore fixed when this list is published: its descriptor,
  threshold, signers with their `accKey`s and coordination accounts, its
  parameters and its own `next`. Activation writes exactly that list with a
  `version`; nothing is supplied then.
- `nextAddresses` are the backup's addresses, published so that a verifier
  with an address index can see whether the backup was ever activated
  (§2.6). They are fixed by `next` as well and must equal the backup's
  `addresses`.

**Fee entries.** `fee` and the smallest spendable input are not in the list.
They are operator-signed entries on `dn.acme/bitcoin/addresses`:

```json
{"type": "fee", "minSatPerVB": 2, "maxSatPerVB": 40, "maxFeeSat": 50000,
 "maxInputs": 12, "minInputSat": 10000}
```

applied from the block holding the latest such entry. Validators read the
latest; verifiers never need it. The rate must lie in `[minSatPerVB,
maxSatPerVB]`, the absolute fee must not exceed `maxFeeSat`, and the input
count must not exceed `maxInputs`. `maxInputs` is derived from the 20 KB
transaction limit (`protocol/data_entry.go:27`), which bounds the proposal
entry and so the PSBT, in hex: a `wsh` 14-of-20 input carries a 683-byte
witness script, about 1.5 KB in hex, so about 12 inputs fit. Replace-by-fee is
always signalled.

**Succession.** The first list is fixed by the pinned first anchor. Every later
list is a successor of the list before it, N, by one of two routes. Both need
(a): the list entry's transaction has principal `dn.acme/bitcoin/addresses`,
its hash is `L`, and its receipt proves it on that account's main chain (only
a successful transaction reaches a main chain) at `h_L`.

- **Normal rotation.** List N+1 is valid when (a) holds, its receipt ending at
  the root of a confirmed anchor from N+1's addresses at a height above `h_L`
  (payload, address, six confirmations; the builder uses the first); and a
  **rotation spend** for it from N's addresses has six confirmations. The rotation spend is a recorded spend
  (§2.3): its `OP_RETURN` carries `ACR1`, the network tag, `h_L` and `L`; its
  record is a proposal and shares from N's `threshold` signers, each signed by
  that signer's `accKey` in N, and its broadcast counts (§2.4). Its inputs are
  signed by N's Bitcoin keys. A rotation therefore costs a threshold of
  **both** of N's key sets — the same as a forged anchor under N — and the
  operator book alone cannot change the list. Each validator's Bitcoin signer
  signs a rotation spend only after its operator has confirmed `L` out of band
  (§2.8). The receipt's anchor need not carry its record: `L` is already
  signed by both of N's key sets.
- **Backup activation.** List N+1 is valid when: the JCS hash of the entry
  without `version` equals `N.next`; (a) holds, its receipt ending at the root
  of the **activating anchor**: a confirmed anchor from N+1's own addresses at
  a height above `h_L`, whose record (proposal and `threshold` shares) is
  signed by N+1's signers' `accKey`s, which `N.next` fixed; and the window
  holds: N is in force when the activating anchor confirms, or N's successor
  by rotation spend confirmed fewer than `N.backupWindow` Bitcoin blocks
  before it. No signature and no spend from N is needed, so it works with N's
  keys lost or stolen; it needs both of the backup's key sets.
- **Fork rule.** If two lists both satisfy the rules as successors of N: a
  backup activation within its window beats a normal rotation; between two
  activations of the same `N.next`, or two normal rotations, the one whose
  activating anchor or rotation spend confirmed first in Bitcoin (block
  height, then position in the block) wins. The losing list, and every list
  after it, is not part of the chain. A backup whose window has passed is
  void, and its keys are destroyed with N's. The verifier checks the chain the
  package presents and applies the rule only when shown both (stated limit).
- **Stated limit.** A party holding a threshold of both key sets of a list can
  present a verifier with an alternate history from that list onward: anchors,
  and rotations to lists of its own. For the list in force, that is a
  threshold of the validators — the network's own trust — and the answer is
  the backup within its window. For a past list, its forged rotation confirms
  after the honest one and loses when both are shown. A verifier shown *only*
  the thief's chain accepts it: the backup protects packages made after the
  activation, and verifiers that look (`nextAddresses`, §2.6). The promise
  is: no list change, and no anchor, with fewer than both key sets of one
  list; live theft is answered by the backup. Operators destroy both key sets
  of a retired list, and its backup's, once its `backupWindow` has passed.

**Funding.** The rotation spend also moves N's funds: its outputs other than
the `OP_RETURN` go only to N+1's addresses. When N's keys are lost or stolen
the backup's addresses are funded by a plain payment. A sweep (§2.3)
consolidates inputs.

**Rotation.** Every change of the Directory's committee that adds or removes a
validator requires a new list, whose rotation spend should confirm within two
slots (`intervalBlocks` Bitcoin blocks each) of the change taking effect. This
is an operational requirement, not a consensus rule. Until the new list is in
force the old list is, and its shares count. A new list is also published to
rotate keys. Each list's backup is prepared before the list is published —
keys generated offline, coordination accounts created — with one offline key
per signer; backups are two deep (the backup's own `next`). A committee change
changes the backup, and so needs a new list. Who holds the offline keys is
custody, an operational matter.

**Threat table.** What a party holding each key set can do, and what stops it.

| Holds | Can do | Stopped by |
|---|---|---|
| Current Bitcoin keys alone (threshold) | Spend list coins in a transaction with no record | It is not an anchor (invariant 1) nor a rotation; the unrecorded-spend alarm fires; the backup is activated |
| Current Accumulate keys alone (threshold; they also meet the operator book) | Write proposals, shares and list entries | No spend without Bitcoin signatures; a list entry without a rotation spend is never in force; Bitcoin signers sign a rotation only on their operator's word (§2.8); a wrong root is refused (invariant 3). At worst a pending list stalls anchoring (overdue alarm) until operators write a correct one |
| Both current key sets | Forge anchors and rotate to a list of its own | Nothing; it is a threshold of the validators, the network's own trust. Answer: the backup within `backupWindow` (stated limit) |
| Both key sets of a past list | Present an alternate history from that list onward | Its rotation confirms after the honest one and loses when both are shown; a verifier shown only it accepts it (stated limit); operators destroy retired keys |
| The backup's Bitcoin keys alone | Nothing | Activation needs a record signed by the backup's `accKey`s, fixed by `next` |
| Both of the current backup's key sets | Activate the backup | That is the recovery route; custody of the offline keys is an operational decision |
| A past list's unused backup keys | Nothing, once `backupWindow` after that list's rotation has passed | The window (fork rule); destroyed with the list's keys |
| One retired validator's node key | Sign as that validator on lists that still name its `accKey` | Below threshold alone; the next list drops it (rotation); `accKey` binds a key per list |
| One faulty signer's coordination entries | Write proposals, shares and broadcasts in its own account | A broadcast counts only with `threshold` shares for its `T`; a proposal counts only in its slot (§2.4); below threshold alone |
| A lagging or eclipsed Bitcoin node at a validator | Misjudge an input or the slot | That validator declines or shares late; a split costs one slot; it cannot make others sign (§2.4 margins) |
| An eclipsed verifier | Be shown a false Bitcoin chain | Six blocks of valid proof of work at the current difficulty; an SPV client is only as safe as that (§2.8) |
| A Bitcoin reorganization deeper than six blocks | Remove a confirmed anchor | The builder re-derives; a package built on the removed block no longer verifies; a party wanting more margin waits for more confirmations |
| A package server | Show only one chain of lists | Stated limit; the `nextAddresses` check (§2.6) |
| One far-ahead leader header | Move Accumulate block time | No rule depends on block time (§2); C23 is Paul's to bound |
| One locked-out signer | Lock only its own coordination account | Per-signer accounts (§2.1); the others keep anchoring; the next list replaces it |

### 2.3 The anchor transaction

Three kinds of **recorded spend** leave a list's addresses: the anchor, the
rotation spend (§2.2) and the sweep. Each has a proposal, shares from
`threshold` distinct signers and a broadcast (§2.4). A spend from a list
address that is none of these is unrecorded.

- Inputs: one or more outputs of the list's addresses, each of at least
  `minInputSat`. Any payment to a list address funds it. The proposer uses
  inputs with at least six confirmations in its own Bitcoin view; a validator
  shares for inputs with at least one. No input may be an input of a
  **pending** recorded spend: one whose broadcast counts (§2.4) and which has
  not confirmed. Its inputs are free again once it is conflicted, or once a
  later recorded spend of the same list confirms. A replacement (§2.4) spends
  exactly the inputs of the transaction it replaces.
- Outputs:
  - an anchor: one `OP_RETURN` carrying the anchor payload, and at most one
    change output to a list address;
  - a rotation spend: one `OP_RETURN` carrying the rotation payload, and
    outputs only to the new list's addresses;
  - a sweep: no `OP_RETURN`, and outputs only to the list's own addresses.

  A change output is omitted when it would be below the dust limit. Nothing
  else.
- Signatures: `SIGHASH_ALL` for `wsh` inputs and `SIGHASH_DEFAULT` for `tr`
  inputs. A share with any other sighash type is invalid.
- The payloads, 52 bytes each:

  | bytes | anchor | rotation spend |
  |---|---|---|
  | 0–3 | `ACC1` — magic and version | `ACR1` — magic and version |
  | 4–11 | the first 8 bytes of SHA-256 of the network ID | the same |
  | 12–19 | the Directory block height `H`, big-endian | `h_L`, the height of the block holding the new list, big-endian |
  | 20–51 | `R`, the Directory's `RootChainAnchor` at `H` | `L`, the new list entry's transaction hash |

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
  replaced at a higher fee (§2.4) or abandoned, and the next anchor proceeds.
- A sweep consolidates inputs. The unrecorded-spend alarm does not fire for
  any recorded spend.

### 2.4 Coordination

**Slots.** The clock is Bitcoin's. The slot of a Bitcoin height `b` is
`s = floor(b / intervalBlocks)`. The designated proposer for slot `s` is
`signers[s mod len(signers)]`.

**Proposals.** Each is an entry in its proposer's coordination account
carrying the unsigned transaction as a PSBT (BIP 174); `T` below is a
proposal's unsigned transaction ID, which for segregated-witness inputs is
also the signed transaction's ID.

- Anchor: `{"type":"proposal","slot":s,"height":H,"root":R,"psbt":"…"}`.
- Rotation spend: `{"type":"rotation","slot":s,"height":h_L,"list":L,"psbt":"…"}`.
- Sweep: `{"type":"sweep","slot":s,"psbt":"…"}`.
- Replacement: `{"type":"replace","slot":s,"replaces":T,"psbt":"…"}`.

**Judging.** A validator judges each proposal once, when its node processes
the Directory block holding it, against its own Bitcoin view at that moment;
a proposal judged invalid is not judged again. It takes proposals in the
order of §2.1 (Directory height, then signer index). A proposal counts for the
validator only if its `slot` is the validator's current slot or the one
before. The designated proposer's proposal counts at once; another signer's
counts only once the validator's Bitcoin tip is at least
`s × intervalBlocks + ceil(intervalBlocks / 2)` — with `intervalBlocks = 1`
that is the next slot, which the one-behind rule admits. A validator shares at
most one anchor, rotation or sweep proposal per slot, the first that counts
and passes its checks. These are liveness rules for validators: the verifier
checks only the payload, the address, the confirmations and the record. A
split costs the slot; the next slot has another proposer.

**What a slot proposes.** A list entry on `dn.acme/bitcoin/addresses` after
the list in force, well formed, and present at `h_L` on the validator's own
node, is **pending** until its rotation spend confirms; only the latest such
entry is pending. While a list is pending, the slot's proposal is its rotation
spend and no anchor is proposed or shared under the list in force. Otherwise
it is an anchor, or a sweep when the list's inputs need consolidating.

At each slot, in order:

1. **Proposal.** For an anchor, the designated proposer chooses `H`: a stored
   Directory block at least 10 blocks below its own tip, and strictly above
   the `H` of the latest counted anchor broadcast; `R` is that block's
   `RootChainAnchor`. The margins of height and of input confirmations absorb
   the lag of the other validators' nodes.
2. **Shares.** Every validator checks the proposal:
   - for an anchor: its own node holds `H` with root `R`, and `H` is above the
     `H` of the latest counted anchor broadcast; as a local liveness check it
     may decline an `H` more than `maxLag` stored blocks behind its own tip;
   - for a rotation spend: the list is the pending one, its own node holds
     its entry with hash `L` at `h_L`, and the outputs go to that list's
     addresses; its Bitcoin signer has its operator's confirmation of `L`
     (§2.8);
   - the PSBT has exactly the outputs of §2.3 with the payload for the
     proposal;
   - every input meets §2.3, is an output of a list address of at least
     `minInputSat`, and its prevout (txid, vout, scriptPubKey, amount) matches
     the validator's **own** Bitcoin node, not the PSBT's `witness_utxo`
     (CVE-2020-14199);
   - the sighash type is as §2.3, and the fee rate, absolute fee and input
     count are within the latest fee entry's bounds.

   If all hold it writes `{"type":"share","txid":T,"sig":"…"}`, `sig` being
   its BIP 174 partial signature, hex (`PSBT_IN_PARTIAL_SIG` for `wsh`,
   `PSBT_IN_TAP_SCRIPT_SIG` for `tr`). If any check fails it writes nothing.
   At most one share per signer per `T` counts.
3. **Broadcast.** When shares from `threshold` distinct signers for `T`
   exist, the proposer — or, after `intervalBlocks` Bitcoin blocks, anyone —
   finalizes and broadcasts, and writes `{"type":"broadcast","txid":T}`. A
   broadcast **counts** only if the record holds a proposal whose unsigned
   transaction ID is `T` and shares for `T` from `threshold` distinct signers
   of the list in force. A broadcast that does not count is ignored: for the
   `H` rule, for pending inputs and for the alarms. One faulty signer's
   entries alone therefore move nothing.
4. **Replacement.** If a counted broadcast's transaction is unconfirmed after
   `intervalBlocks` Bitcoin blocks and the fee bounds allow a higher rate, the
   designated proposer of the current slot writes a `replace` proposal for
   it. A replacement has the same payload and outputs' destinations, spends
   exactly the same inputs and has a strictly higher fee rate; its absolute
   fee rises by at least the incremental relay fee times its size (BIP 125).
   It is exempt from the `H` rule and from the one-per-slot rule; a validator
   shares at most one replacement of a given `T` per slot. It is shared and
   broadcast as in steps 2 and 3. The record keeps both; the one that
   confirms is the anchor (or rotation spend, or sweep).

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
   tracks every `T` proposed or replaced for `H`; on a reorganization it
   re-derives.
   Before then `directory` is `D`; after, it is `H`: the field changes meaning,
   and the package says which by the presence of `anchor`.
5. Serve the package by account and entry, or by transaction hash.

It also keeps, for every list and coordination entry, the receipt into the next
anchor, since every package carries the anchor's record, and for every list
its rotation spend's or activating anchor's record and Bitcoin data.

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
  "bitcoin": {
    "<txid>": {"header": "…", "merkleProof": "…", "prevTxs": ["…"]}
  },
  "list": {"chain": [
    {"entry": E, "directoryHeight": 41190000, "receipt": { … }, "anchorTxid": "…"},
    {"entry": E, "directoryHeight": 41202000, "receipt": { … }, "anchorTxid": "…",
     "rotation": {"txid": "…", "record": {"proposal": E, "shares": [E]}}},
    {"entry": E, "directoryHeight": 41203100, "receipt": { … }, "anchorTxid": "…",
     "record": {"proposal": E, "shares": [E]}}
  ]}
}
```

- `entry.hash` is the chain entry, which for a data account's main chain is the
  **transaction hash**. `transaction` and `signatures` let the verifier link
  the entry to its data and check who signed it. Any data carried for the
  reader must equal `transaction.body.entry.data`.
- `receipt` runs from `entry.hash` to `directory.root`.
- `anchor.record` holds one receipt per `E`, each into the root of anchor *j*
  (`anchor.final`). After a replacement the record also carries
  `"replace": {"E": E, "receipt": { … }}`, the `replace` proposal whose `T`
  matches `anchor.txid`; it must repeat the payload of the `proposal` it
  replaces, and the shares checked are those for its `T`. `anchor.final` is
  *j*; its record is checked by signatures only, with no receipt, which ends
  the recursion. It is absent when the anchor is provisional.
- `bitcoin` maps each Bitcoin transaction the package names — the first
  anchor, the anchor, *j*, and every `anchorTxid` and rotation `txid` in
  `list.chain` — to its `gettxoutproof` form: the block `header`, the
  `merkleProof`, and `prevTxs`, the funding transactions, so that partial
  signatures can be checked against the prevout amounts and scripts and the
  sending address read. A full node with `txindex` may skip it.
- `list.chain` is the sequence from the first list to the one in force at
  `H_j` (at `directory.height` when the anchor is provisional). Each item is
  the list entry, `h_L`, its receipt into the root of the anchor `anchorTxid`
  (for the first list, the first anchor), and either `rotation` — the
  rotation spend's txid and record, for a normal rotation — or `record` — the
  activating anchor's record, `anchorTxid` being that anchor, for a backup
  activation. These records are checked by signatures only.
- Each share's `sig` is the BIP 174 partial signature, hex (§2.4).

### 2.6 The verifier

Given a package, the first anchor's transaction ID, and Bitcoin:

1. Fetch the first anchor; decode its payload; the address it was sent from is
   on the first list, and the first list's receipt ends at that payload's root.
   Walk `list.chain`: each next list is a successor of the one before it by
   normal rotation or backup activation (§2.2), every Bitcoin transaction
   checked as in step 2 and every record by its signers' `accKey`s. This
   yields the list in force at any height up to `H_j`.
2. Check `bitcoin` against `anchor.txid`: the header and Merkle proof (SPV, or
   a full node's own view). The payload is `(directory.height,
   directory.root)`; the input is an output of an address of the list in force
   at `directory.height`; it has `n ≥ 6` confirmations. Ignore the package's
   `block` if it disagrees.
3. Check `hash(transaction) == entry.hash`; the body is `WriteData` with a
   `DoubleHashDataEntry` of exactly one `Data` element; and the receipt from
   `entry.hash` to `directory.root`. Check the entry's signatures.
4. Check the anchor's record: a proposal for `(height, root)`, shares for its
   `T` from `threshold` distinct signers of the list in force (the stored
   count), each entry signed by its signer's `accKey`, each share a valid
   partial signature over the proposal's PSBT, with each `E` receipted into
   the root of *j*. *j* is a confirmed anchor whose `H_j` is at or above the
   Directory height of the last record entry of this anchor; check *j* (step
   2, under the list in force at `H_j`) and its own record by signatures only.
   *j* may itself be provisional. If the package has no *j*, the result is
   *provisional*.
5. Report: the entry, the height, the Bitcoin block and its time, and
   *final* or *provisional*. The entry existed before the confirming block was
   mined. The header time is approximate, in either direction, by about two
   hours. A conservative upper bound is the median time past of the block
   eleven blocks after the confirming block, on the assumption that most hash
   power stamps honestly; the report says so.
6. Optionally, with an address index: for each list in the chain, look for a
   confirmed spend from its `nextAddresses`. One that the chain does not
   account for means the backup was activated on a chain the package does not
   show; the report says *contested* instead of *final*.

A package is checked with Bitcoin alone. The alarms are a separate watcher
that needs a Directory node to read the broadcast entries; run continuously
against one it raises these alarms, all liveness, not safety:

- **Overdue:** no confirmed anchor in three slots.
- **Unrecorded spend:** an output of a list address was spent by a
  transaction that is not a recorded spend — no proposal with that `T` and
  shares from `threshold` signers in the network's own coordination accounts.
  That is a key compromise; the response is the backup.
- **Backup spend:** an output of any list's `nextAddresses` was spent while
  that backup was not activated on the network's own chain.
- **Pending list:** a list has been pending for more than two slots.
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
| Validator coordination (propose, share, broadcast, replace, rotate) | `internal/anchoring`, started by the Directory's validator service when a list names its key |
| Proof builder and package service | `internal/anchoring/proofs`, started by a follower's API when configured |
| Verifier and alarms | `tools/cmd/anchor-verify` |

**Key placement (a requirement of B2).** The Bitcoin key is held by a signer
separate from the node process, its own process or an HSM. For an anchor it
receives `(H, R, PSBT)` and checks `R` against its own node before signing.
For a rotation spend it receives `(h_L, L, PSBT)` and signs only after its
operator has confirmed `L`, the new list, out of band; the node's say-so is
not enough, or the node keys alone could rotate the list. The node's
key signs the coordination record. Without this, a forged anchor needs only one
compromised host per validator.

Bitcoin access is through a configured node's RPC or a light client; the
verifier accepts either. A light client must validate block headers and proof
of work (SPV); an Electrum-style server without that check is a trusted party.
