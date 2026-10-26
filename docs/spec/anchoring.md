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
  a **rotation spend** — the sweep from that list's addresses to the new
  list's, whose `OP_RETURN` carries the new list's hash, so it needs both of
  that list's key sets — or by activating the **backup list** published in
  full inside it, whose offline Bitcoin keys the same validators hold, when
  the list in use is compromised. A backup may override a rotation only within
  `backupWindow` Bitcoin blocks of it (§2.2).
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
   an address on the list in force at that height and confirmed before that
   list's successor was committed (invariant 4), *and* the list's
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
   Accumulate keys, or by activating the backup list published inside it,
   which needs the backup's offline Bitcoin keys and the Accumulate keys it
   names (§2.2). The
   operator book alone cannot change the list; stolen Bitcoin keys alone
   cannot change the list, make a record, or make an anchor. Once a list is
   replaced, its keys cannot add anchors: an anchor counts only if it
   confirms before its list's successor is committed in Bitcoin (§2.2,
   the Bitcoin-order cut). Recovery from a live compromise is activating the
   backup within its window; it needs no spend from the old addresses. A
   party holding both key sets of a list is the stated limit (§2.2 threat
   table).
5. **Each anchor is confirmed by a later one.** Anchor *i* has a confirming
   anchor *j* when a confirmed anchor whose height `H_j` is at or above the
   Directory height of the last record entry of *i* (its broadcast, or its
   last share) exists, and *j*'s own record verifies. The builder uses the
   first such *j*; *j* may itself lack one. Without *j*, *i* is
   *provisional*; the newest anchor always is. With *j*, a package is
   *final* — unless its list chain holds a rotation, which a backup may
   override within `backupWindow`: then it is *contestable* until Bitcoin
   block `X`, the latest such rotation's block plus the rotated list's
   `backupWindow`, and *final* after it (§2.6 step 5). A verifier reports
   which, and `X`.
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
   Bitcoin alone. Once final it does not expire, and no Accumulate node need
   exist for it to be checked.

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
- Finality takes the anchor interval plus Bitcoin confirmations (six), and, for
  a package whose list chain holds a rotation, until that rotation's
  `backupWindow` has passed (invariant 5). The anchor interval is a cost: at
  $0.12–$1 a transaction, an anchor every Bitcoin block (about ten minutes) is
  about $6,300–$52,600 a year, hourly $1,050–$8,800, daily $44–$365.
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
name a signer's account from an earlier list, and a backup names the same
accounts as its list (§2.2); a new signer's account is created when the list
naming it is prepared. Validators and the verifier ignore a signer's entries
in an account whose book does not hold that signer's `accKey` at threshold 1.
Its authority is its own book only. A signer that runs `UpdateAccountAuth` on
it locks only itself out; the others keep anchoring, and the next list names a
replacement. Each validator funds its own account's page with credits
(`AddCredits`); the credits-low alarm is per signer (§2.6).

A spend's records are read from every coordination account its list names,
ordered by Directory block height, then signer index, then the entry's index
on the account's main chain. The verifier and the builder take the account
URLs from the list, not from this spec.

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
  "backup": {
    "type": "address-list",
    "network": "mainnet",
    "threshold": 14,
    "signers": [
      {"validator": "acc://dn.acme/operators/1#…", "accKey": "…",
       "key": "03…", "coordination": "acc://dn.acme/bitcoin/c1"},
      …
    ],
    "descriptor": "wsh(sortedmulti(14,03…,03…,…))",
    "addresses": ["bc1q…"],
    "intervalBlocks": 6,
    "maxLag": 1000,
    "backupWindow": 1008,
    "next": "…"
  }
}
```

- `signers` map each Bitcoin public key to the validator that holds it, by the
  validator's key hash on `dn.acme/operators/1`, and bind `accKey`, the hash of
  the Accumulate public key whose signatures count for that signer. The
  verifier checks records against `accKey`. A node-key rotation therefore
  takes effect for anchoring only through a new list. A validator holds
  exactly one key per list.
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
  for readers that do not parse descriptors. Every list uses fresh Bitcoin
  keys: no key appears in more than one list or backup, so destroying a
  retired list's keys never touches a live one.
- `intervalBlocks` is in Bitcoin blocks, at least 1 (6 is about an hour). The
  slot of Bitcoin height `b` is `floor(b / intervalBlocks)` (§2.4).
- `maxLag` is the number of stored Directory blocks behind its own tip beyond
  which a validator declines a proposed `H` (§2.4).
- `backupWindow` is in Bitcoin blocks, default 1008 (about a week): how long
  after this list is succeeded by a rotation spend its backup may still be
  activated over that succession (fork rule). No minimum is imposed; a
  shorter window shortens the time in which a forged rotation can be
  recovered from, and the time in which a package stays contestable
  (invariant 5).
- `backup` is the whole backup list, published inside this one: its
  descriptor, threshold, signers, addresses and parameters, without `version`
  and with its own backup only as `next`. The backup's keys are held by the
  same validators, one offline Bitcoin key each: its signers name the same
  validators, `accKey`s and coordination accounts as this list, and differ
  only in `key`. Its addresses are therefore public, and a verifier with a
  full node can watch them (§2.6).
- `next` is the `sha256` of `backup` in the canonical form of RFC 8785 (JSON
  Canonicalization Scheme). A written list whose `next` does not match its
  `backup` is invalid. Inside a `backup`, `next` commits to the backup's own
  backup, which is written in full when the backup is activated.
- **Well formed.** A list entry is well formed when: its `type` and `network`
  are as above; its signers are exactly the Directory's committee at `h_L`
  (the height of the block holding it; for a backup, at the height of the
  list that published it), one each; the threshold rule holds; `backup` is
  present and matches `next`, and is itself well formed except that it
  carries only its `next`; its keys are fresh; and `intervalBlocks ≥ 1`.
  Validators and the verifier ignore a list entry that is not.
- **Size.** A list entry, backup included, must fit one 20 KB data entry
  (`protocol/data_entry.go:27`). At 20 signers it is about 15 KB; about 25
  signers is the most it holds. A larger committee needs the list split over
  several entries, which this spec does not define.

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
always signalled. Fee entries are written by the operator book, which is the
node keys; each Bitcoin signer also enforces a fee cap its own operator sets
locally (§2.8), so a fee entry alone cannot pay the coins away as fee.

**Commit, start and cut.** Every list `L` has a **commit**, a Bitcoin
transaction, and a **start**, a Directory height:

| How `L` came to be | Commit | Start `s_L` |
|---|---|---|
| The first list | the pinned first anchor | that anchor's `H` |
| Normal rotation | its rotation spend | `h_L + 1`, `h_L` from the rotation payload |
| Backup activation | its activating anchor | that anchor's `H` |

A transaction is *confirmed in* the Bitcoin block that includes it; counting
it valid also needs six confirmations. `L`'s **cut block** is the block of
the first confirmed commit of a successor of `L` — a rotation spend from
`L`'s addresses with its record, or an activating anchor of `L`'s backup —
whether or not that successor completes or wins the fork rule: a rotation
spend overridden by the backup still cuts. An anchor `(H, R)` sent from `L`'s
addresses counts only if `s_L ≤ H`, its block is at or after the block of
`L`'s commit, and — once `L` has a successor `L'` in the chain — `H < s_L'`
and its block is strictly before `L`'s cut block. The second half is the
**Bitcoin-order cut**: the keys of a replaced list cannot add anchors for
that list's heights later, because any anchor they make confirms after the
cut. Starts strictly increase along the chain; a chain where they do not is
invalid. Lists are read in main-chain order of
`dn.acme/bitcoin/addresses`.

**Succession.** The first list is fixed by the pinned first anchor. Every later
list is a successor of the list before it, N, by one of two routes. Both need
(a): the list entry is well formed, its transaction has principal
`dn.acme/bitcoin/addresses`, its hash is `L`, and its receipt proves it on
that account's main chain (only a successful transaction reaches a main
chain).

- **Normal rotation.** List N+1 is valid when (a) holds, its receipt ending at
  the root of the first anchor under N+1 (§2.4), which counts under the cut
  (payload, address, six confirmations); and its **rotation spend** from N's
  addresses has six confirmations. The rotation spend is the sweep from N's
  addresses to N+1's (§2.3): its `OP_RETURN` carries `ACR1`, the network tag,
  `h_L` (the height of the block holding the entry) and `L`, the hash of the
  new list's entry; its record is a proposal and shares from N's `threshold`
  signers, each signed by that signer's `accKey` in N, and its broadcast
  counts (§2.4). Its inputs are signed by N's Bitcoin keys. A rotation
  therefore needs a threshold of **both** of N's key sets — the same as a
  forged anchor under N — and the operator book alone cannot change the list.
  Each validator's Bitcoin signer signs a rotation spend only after its
  operator has approved `L`, and checks `h_L` against its own node (§2.8).
  The anchor the receipt ends at need not carry its record here: `L` is
  already signed by both of N's key sets.
- **Backup activation.** List N+1 is valid when: the entry, without `version`
  and `backup`, equals `N.backup` (its JCS hash is `N.next`); (a) holds, its
  receipt ending at the root of the **activating anchor**: a confirmed anchor
  from N+1's own addresses, at a height `H` at or above the entry's, whose
  record (proposal and `threshold` shares) is signed by N+1's signers'
  `accKey`s, which `N.backup` fixed; and the window holds (fork rule). No
  signature and no spend from N is needed, so it works with N's Bitcoin keys
  lost or stolen. Because the start is the activating anchor's own `H`, a
  backup cannot claim heights before it was activated.
- **Fork rule.** A backup activation of N is *within its window* if no
  rotation spend of N is confirmed in an earlier block, or the first one is
  confirmed fewer than `N.backupWindow` blocks earlier; otherwise it is void.
  Among the successors of N that satisfy the rules, a backup activation
  within its window beats every normal rotation; otherwise the one whose
  commit is confirmed first (block height, then position in the block) wins.
  The losing list, and every list after it, is not part of the chain. The
  verifier checks the chain the package presents, with the rotation an
  activation overrides when the package names it (§2.5), and applies the
  rest of the rule only when shown both or when it scans (§2.6 step 6).
- **Stated limit.** A party holding a threshold of both key sets of a list
  can present a verifier with an alternate history from that list onward:
  anchors confirmed after the honest successor's commit, a chain that leaves
  that successor out, and rotations to lists of its own. For the list in
  force, that is a threshold of the validators — the network's own trust —
  and the answer is the backup within its window. For a past list, the
  Bitcoin-order cut voids its later anchors and its forged rotation confirms
  after the honest one, so it loses whenever the verifier sees the honest
  successor: shown both, or by scanning the list's addresses (§2.6 step 6). A
  verifier that is shown only the thief's chain and does not scan accepts it.
  The operational backstop is key destruction: operators destroy both key
  sets of a retired list, and its backup's offline keys, once its
  `backupWindow` has passed.

**Funding.** The rotation spend moves N's funds: its outputs other than the
`OP_RETURN` go only to N+1's addresses, at least `ceil(7 / intervalBlocks) +
1` of them (N+1's `intervalBlocks`), each of at least `minInputSat`, so that
N+1 has an input for every slot while each waits its six confirmations. Within
N's `backupWindow` after the rotation spend, an **old-list sweep** moves what
is left: a recorded spend from N's addresses with no `OP_RETURN` and outputs
only to N+1's addresses, proposed and shared by N's signers in their
coordination accounts on N's slot schedule. Coins paid to a retired list's
addresses after its window are lost, as with any payment to a destroyed key.
When N's keys are lost or stolen the backup's addresses are funded by a plain
payment.

**Rotation.** Every change of the Directory's committee that adds or removes a
validator requires a new list, whose rotation spend should confirm within two
slots (`intervalBlocks` Bitcoin blocks each) of the change taking effect. This
is an operational requirement, not a consensus rule. Until the rotation spend
has six confirmations the old list is in force for coordination and its shares
count; from then, validators anchor under the new list (§2.4), and its first
anchor completes the succession. A new list is also published to rotate keys.
Each list's backup is prepared before the list is published: each validator
generates one fresh offline Bitcoin key for it. A committee change changes the
backup, and so needs a new list. A later list entry supersedes a pending one;
that is how a pending list is cancelled.

**Threat table.** What a party holding each key set can do, and what stops it.

| Holds | Can do | Stopped by |
|---|---|---|
| Current Bitcoin keys alone (threshold) | Spend list coins in a transaction with no record | It is not an anchor (invariant 1) nor a rotation; the unrecorded-spend alarm fires; the backup is activated |
| Current Accumulate keys alone (threshold; they also meet the operator book) | Write proposals, shares, list entries and fee entries | No spend without Bitcoin signatures; a list entry without a rotation spend is never in force; Bitcoin signers sign a rotation only on their operator's approval and check every output and a local fee cap (§2.8); a wrong root is refused (invariant 3). At worst a pending list halts anchoring (pending-list alarm) until operators write a correct one |
| A threshold of the node keys and of the backup's offline Bitcoin keys | Activate the backup | That is the recovery route; the same validators hold both (custody), so a validator threshold that collects its offline keys can do it, as it can forge anchors |
| Both current key sets | Forge anchors and rotate to a list of its own | Nothing; it is a threshold of the validators, the network's own trust. Answer: the backup within `backupWindow` (stated limit) |
| Both key sets of a past list | Present an alternate history from that list onward | The Bitcoin-order cut voids its later anchors; its rotation confirms after the honest one; a verifier that is shown the successor or scans rejects it; one shown only the thief's chain and not scanning accepts it (stated limit); key destruction |
| The backup's Bitcoin keys alone | Nothing | Activation needs a record signed by the backup's `accKey`s, fixed by `backup` |
| A past list's unused backup keys, after `backupWindow` | Present a chain that leaves the rotation out | A verifier that scans the list's addresses sees the rotation and the window rejects the backup; one that cannot scan accepts it (stated limit); key destruction |
| One retired validator's node key | Sign as that validator on lists that still name its `accKey` | Below threshold alone; the next list drops it (rotation); `accKey` binds a key per list |
| One faulty signer's coordination entries | Write proposals, shares and broadcasts in its own account | A share counts only if its partial signature validates; a broadcast counts only with `threshold` such shares for its `T`; a proposal counts only in its slot (§2.4); below threshold alone |
| A lagging or eclipsed Bitcoin node at a validator | Misjudge an input or the slot | That validator declines or shares late; a split costs one slot; it cannot make others sign (§2.4 margins) |
| An eclipsed verifier | Be shown a false Bitcoin chain | Six blocks of valid proof of work at the current difficulty; an SPV client is only as safe as that (§2.8) |
| A Bitcoin reorganization deeper than six blocks | Remove a confirmed anchor | The builder re-derives; a package built on the removed block no longer verifies; a party wanting more margin waits for more confirmations |
| A package server | Show only one chain of lists | Stated limit; the scan (§2.6 step 6) |
| One far-ahead leader header | Move Accumulate block time | No rule depends on block time (§2); bounding it is DIFFERENCES C23 |
| One locked-out signer | Lock only its own coordination account | Per-signer accounts (§2.1); the others keep anchoring; the next list replaces it |

### 2.3 The anchor transaction

Four kinds of **recorded spend** leave a list's addresses: the anchor, the
rotation spend (§2.2), the sweep and the old-list sweep (§2.2 "Funding"). Each
has a proposal, shares from `threshold` distinct signers and a broadcast
(§2.4). A spend from a list address that is none of these is unrecorded.

- Inputs: one or more outputs of the list's addresses, each of at least
  `minInputSat`. Any payment to a list address funds it. The proposer uses
  inputs with at least six confirmations in its own Bitcoin view; a validator
  shares for inputs with at least one. No input may be an input of an
  **outstanding** recorded spend: one whose broadcast counts (§2.4) and which
  is not yet confirmed. Its inputs are free again once it is conflicted, or
  once a later recorded spend of the same list confirms. A replacement (§2.4)
  spends exactly the inputs of the transaction it replaces.
- Outputs:
  - an anchor: one `OP_RETURN` carrying the anchor payload, and at most one
    change output to the list's own addresses;
  - a rotation spend: one `OP_RETURN` carrying the rotation payload, and
    outputs only to the new list's addresses, as many as §2.2 "Funding"
    requires;
  - a sweep: no `OP_RETURN`, and outputs only to the list's own addresses;
  - an old-list sweep: no `OP_RETURN`, and outputs only to the successor's
    addresses.

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
  coordination record. A stuck transaction therefore blocks nothing but its
  own inputs: it is replaced at a higher fee (§2.4) or abandoned, and the next
  anchor proceeds with other inputs.
- A sweep consolidates inputs. The unrecorded-spend alarm does not fire for
  any recorded spend.

### 2.4 Coordination

**Slots.** The clock is Bitcoin's. The slot of a Bitcoin height `b` under a
list is `s = floor(b / intervalBlocks)`. The designated proposer for slot `s`
is `signers[s mod len(signers)]`. Slots, proposers, shares and the
one-per-slot rule are per list: the list a spend is from.

**Proposals.** Each is an entry in its proposer's coordination account
carrying the unsigned transaction as a PSBT (BIP 174); `T` below is a
proposal's unsigned transaction ID, which for segregated-witness inputs is
also the signed transaction's ID.

- Anchor: `{"type":"proposal","slot":s,"height":H,"root":R,"psbt":"…"}`.
- Rotation spend: `{"type":"rotation","slot":s,"height":h_L,"list":L,"psbt":"…"}`.
- Sweep or old-list sweep: `{"type":"sweep","slot":s,"psbt":"…"}`.
- Replacement: `{"type":"replace","slot":s,"replaces":T,"psbt":"…"}`.

**Judging.** A validator judges each proposal once, when its node processes
the Directory block holding it, against its own Bitcoin view at that moment;
a proposal judged invalid is not judged again. It takes proposals in the
order of §2.1. A proposal counts for the validator only if its `slot` is the
validator's current slot or the one before. The designated proposer's
proposal counts at once; another signer's counts only once the validator's
Bitcoin tip is at least `s × intervalBlocks + ceil(intervalBlocks / 2)` — with
`intervalBlocks = 1` that is the next slot, which the one-behind rule admits.
A validator shares at most one anchor, rotation or sweep proposal per slot of
a list, the first that counts and passes its checks. These are liveness rules
for validators: the verifier checks only the payload, the address, the
confirmations and the record. A split costs the slot; the next slot has
another proposer.

**What a slot proposes.** A well-formed list entry (§2.2) on
`dn.acme/bitcoin/addresses` after the list in force, present at `h_L` on the
validator's own node, is **pending** until its rotation spend confirms; only
the latest such entry is pending, so a later entry supersedes, and so
cancels, an earlier one.

- While a list is pending, no anchor is proposed or shared under the list in
  force; once no recorded spend of that list is outstanding, the slot's
  proposal is the rotation spend. Anchoring is halted while a list is
  pending; that is a stated limit, bounded by the pending-list alarm (§2.6).
- Once the rotation spend has six confirmations, validators propose and share
  under the new list: its signers, slot schedule, coordination accounts and
  parameters, with `H ≥ h_L + 1`. The first such anchor that confirms
  completes the succession (§2.2).
- Within the old list's `backupWindow` after its rotation spend, its signers
  may propose old-list sweeps under the old list's schedule.
- Otherwise the slot's proposal is an anchor, or a sweep when the list's
  inputs need consolidating.

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
     addresses; its Bitcoin signer has its operator's approval of `L`
     (§2.8);
   - the PSBT has exactly the outputs of §2.3 for its kind, with the payload
     for the proposal;
   - every input meets §2.3, is an output of a list address of at least
     `minInputSat`, and its prevout (txid, vout, scriptPubKey, amount) matches
     the validator's **own** Bitcoin node, not the PSBT's `witness_utxo`
     (CVE-2020-14199);
   - the sighash type is as §2.3, and the fee rate, absolute fee and input
     count are within the latest fee entry's bounds and its signer's local
     cap (§2.8).

   If all hold it writes `{"type":"share","txid":T,"sig":"…"}`, `sig` being
   its BIP 174 partial signature, hex (`PSBT_IN_PARTIAL_SIG` for `wsh`,
   `PSBT_IN_TAP_SCRIPT_SIG` for `tr`). If any check fails it writes nothing.
   A share **counts** only if its partial signature validates, for every
   input, against the reading validator's own prevouts; at most one share per
   signer per `T` counts.
3. **Broadcast.** When counted shares from `threshold` distinct signers for
   `T` exist, the proposer finalizes and broadcasts, and writes
   `{"type":"broadcast","txid":T}`. A broadcast **counts** only if the record
   holds a proposal whose unsigned transaction ID is `T` and counted shares
   for `T` from `threshold` distinct signers of the list. A broadcast that
   does not count is ignored: for the `H` rule, for outstanding inputs and
   for the alarms. One faulty signer's entries alone therefore move nothing.
   Every validator finalizes and broadcasts any `T` with `threshold` counted
   shares that it does not see in its mempool or chain, so a proposer that
   writes `broadcast` but does not send the transaction delays nothing.
4. **Replacement.** If a counted broadcast's transaction is unconfirmed after
   `intervalBlocks` Bitcoin blocks and the fee bounds allow a higher rate, the
   designated proposer of the current slot writes a `replace` proposal for
   it. A replacement has the same payload and output destinations, spends
   exactly the same inputs and has a strictly higher fee rate; its absolute
   fee rises by at least the incremental relay fee times its size (BIP 125).
   It is exempt from the `H` rule and from the one-per-slot rule; a validator
   shares at most one replacement of a given `T` per slot. It is shared and
   broadcast as in steps 2 and 3, and may itself be replaced, forming a
   chain. The record keeps all; the one that confirms is the spend. A
   replacement without a fee increase is not proposed, because Bitcoin nodes
   reject it: when the caps block a bump, operators raise them (a fee entry
   and the signers' local caps), or the transaction expires from mempools
   and its inputs stay outstanding until a later recorded spend of the list
   confirms. That is a stated limit; the rotation spend's several outputs
   leave other inputs to anchor with.

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
4. When the first **confirmed** anchor with `H ≥ D` that counts under the
   Bitcoin-order cut (§2.2) has six confirmations, re-prove the Directory
   root-chain entry at `D` to the root-chain anchor at `H`. This replaces the
   final leg with a receipt to a different height, the case database.md says
   searches; the builder does it once per `(D, H)` and every package under
   that anchor shares the leg. Record the Bitcoin transaction, its block and
   height only after six confirmations. The builder tracks every `T` proposed
   or replaced for `H`; on a reorganization it re-derives. Before then
   `directory` is `D`; after, it is `H`: the field changes meaning, and the
   package says which by the presence of `anchor`.
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
      "replace": [{"E": E, "receipt": { … }}],
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
     "record": {"proposal": E, "shares": [E]}, "overrides": "…"}
  ]}
}
```

- `entry.hash` is the chain entry, which for a data account's main chain is the
  **transaction hash**. For a main-chain entry, `transaction` and `signatures`
  let the verifier link the entry to its data and check who signed it, and any
  data carried for the reader must equal `transaction.body.entry.data`. For an
  entry of another chain (a signature chain, which the builder also proves)
  they are absent.
- `receipt` runs from `entry.hash` to `directory.root`.
- `anchor.record` holds one receipt per `E`, each into the root of anchor *j*
  (`anchor.final`). After replacements the record also carries `replace`, the
  chain of `replace` proposals in order, each replacing the one before (the
  first replaces `proposal`), the last one's `T` matching `anchor.txid`; each
  must repeat the payload of the one it replaces, and the shares checked are
  those for the last `T`. Without replacement `replace` is absent.
  `anchor.final` is *j*; its record is checked by signatures only, with no
  receipt, which ends the recursion. It is absent when the anchor is
  provisional.
- `bitcoin` maps each Bitcoin transaction the package names — the first
  anchor, the anchor, *j*, and every `anchorTxid`, rotation `txid` and
  `overrides` in `list.chain` — to its `gettxoutproof` form: the block
  `header`, the `merkleProof`, and `prevTxs`, the funding transactions, so
  that partial signatures can be checked against the prevout amounts and
  scripts and the sending address read. A full node with `txindex` may skip it.
- `list.chain` is the sequence from the first list to the list in force at
  the Bitcoin block of *j* (of the anchor, when it is provisional): every
  successor whose commit is confirmed at or before that block is in it, so
  that the verifier can apply the Bitcoin-order cut (§2.2). Each item is the
  list entry, the Directory height of the block holding it, its receipt into
  the root of the anchor `anchorTxid` (for the first list, the first anchor),
  and either `rotation` — the rotation spend's txid and record, for a normal
  rotation — or `record` — the activating anchor's record, `anchorTxid` being
  that anchor, for a backup activation, with `overrides`, the txid of the
  rotation spend of the previous list it overrides, if there is one. These
  records are checked by signatures only.
- Each share's `sig` is the BIP 174 partial signature, hex (§2.4).

### 2.6 The verifier

Given a package, the first anchor's transaction ID, and Bitcoin:

1. Fetch the first anchor; decode its payload; the address it was sent from is
   on the first list, and the first list's receipt ends at that payload's root.
   Walk `list.chain`: each next list is well formed and a successor of the one
   before it by normal rotation or backup activation (§2.2), every Bitcoin
   transaction checked as in step 2 and every record by its signers'
   `accKey`s. For a rotation, the carried height equals the payload's `h_L`.
   For an activation with `overrides`, that transaction is an `ACR1` spend
   from the previous list's addresses, the activation is within its window,
   and the previous list's cut block is the override's. Each list's start
   (§2.2) is strictly above the one before. This yields each list's commit
   block, cut block and start.
2. Check `bitcoin` against `anchor.txid`: the header and Merkle proof (SPV, or
   a full node's own view). The payload is `(directory.height,
   directory.root)`; every input is an output of an address of one list `L` of
   the chain; the anchor counts under `L` by the Bitcoin-order cut (§2.2):
   `s_L ≤ H`, confirmed at or after `L`'s commit, and, when the chain has a
   list after `L`, below that list's start and before `L`'s cut block; it has
   `n ≥ 6` confirmations. Ignore the package's `block` if it disagrees.
3. Check the receipt from `entry.hash` to `directory.root`. For an entry of a
   data account's main chain, also check `hash(transaction) == entry.hash`,
   that the body is `WriteData` with a `DoubleHashDataEntry` of exactly one
   `Data` element, and the entry's signatures. An entry of another chain is
   proven by its receipt only.
4. Check the anchor's record: a proposal for `(height, root)`, any `replace`
   chain, shares for the final `T` from `threshold` distinct signers of `L`
   (the stored count), each entry signed by its signer's `accKey`, each share
   a valid partial signature over that PSBT, with each `E` receipted into the
   root of *j*. *j* is a confirmed anchor whose `H_j` is at or above the
   Directory height of the last record entry of this anchor; check *j* as in
   step 2, under the cut, and its own record by signatures only. *j* may
   itself be provisional. If the package has no *j*, the result is
   *provisional*.
5. Report: the entry, the height, the Bitcoin block and its time, and the
   state. Let `X` be the latest, over the normal rotations in `list.chain`, of
   the rotation spend's block plus the rotated list's `backupWindow`. With a
   *j*, the package is *contestable* until Bitcoin block `X` — a backup
   activated within a window could still supersede a list the package relies
   on — and *final* after `X`, or at once if the chain has no rotation; the
   report gives `X`. The entry existed before the confirming block was mined.
   The header time is approximate, in either direction, by about two hours. A
   conservative upper bound is the median time past of the block eleven
   blocks after the confirming block, on the assumption that most hash power
   stamps honestly; the report says so.
6. Optionally, with a full node that can scan addresses (`scanblocks` with
   `blockfilterindex`, or an indexer it trusts), look for what the chain
   leaves out:
   - for each list in the chain, a confirmed `ACR1` spend from its addresses
     at or before the block of *j* that is neither the chain's next rotation
     nor the rotation the chain's next activation overrides; and
   - for each list in the chain, a confirmed spend from its backup's
     addresses (`backup.addresses`) at or before the block of *j*, or within
     the list's `backupWindow` after its rotation spend, that is not the
     chain's next list.

   Either means the package shows a chain the network superseded: the report
   says *contested*. A spend from a backup's addresses after its window is
   void and is not reported. A verifier without such a node cannot do this
   check; for it, the one-chain limit of §2.2 stands.

A package is checked with Bitcoin alone. The alarms are a separate watcher
that needs a Directory node to read the broadcast entries; run continuously
against one it raises these alarms, all liveness, not safety:

- **Overdue:** no confirmed anchor in three slots.
- **Unrecorded spend:** an output of a list address was spent by a
  transaction that is not a recorded spend — no proposal with that `T` and
  counted shares from `threshold` signers in the network's own coordination
  accounts. That is a key compromise; the response is the backup.
- **Backup spend:** an output of any list's backup addresses was spent while
  that backup was not activated on the network's own chain.
- **Pending list:** a list has been pending for more than two slots;
  anchoring is halted until it is rotated to or superseded.
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
| Validator coordination (propose, share, broadcast, replace, rotate, sweep) and the separate Bitcoin signer | `internal/anchoring`, started by the Directory's validator service when a list names its key |
| Proof builder and package service | `internal/anchoring/proofs`, started by a follower's API when configured |
| Verifier and alarms | `tools/cmd/anchor-verify` |

**Key placement (a requirement of B2).** The Bitcoin key is held by a signer
separate from the node process, its own process or an HSM. The node's key
signs the coordination record; the signer signs the Bitcoin transaction only
after checking it itself, against its own Directory node and its own Bitcoin
node, never on the coordinating node's word:

- every input's prevout, from its own Bitcoin node;
- for an anchor: `R` is its node's root at `H`, and the only other output is
  change to the list's own addresses;
- for a rotation spend: `h_L` is the height of the block holding `L` on its
  node, `L` is the entry its operator has approved out of band, and every
  output but the `OP_RETURN` goes to the addresses of the list `L` names, in
  the number §2.2 requires;
- for a sweep: every output goes to the list's own addresses; for an
  old-list sweep, to its successor's;
- for a replacement: the same inputs and the same output destinations as the
  transaction it replaces;
- the fee is below a cap its operator sets locally, whatever the fee entries
  say.

Without this, a forged anchor, or a transaction that pays the coins away as
change or fee, needs only one compromised host per validator. With it, the
node keys alone — which also write the fee entries and meet the operator
book — move no coins outside the lists.

Bitcoin access is through a configured node's RPC or a light client; the
verifier accepts either. A light client must validate block headers and proof
of work (SPV); an Electrum-style server without that check is a trusted party.
