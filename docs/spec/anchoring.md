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

The keys that sign anchors are held the way the staking operators' keys are
(staking `docs/design/staking-signer.md`): each operator has one key, created
in its own encrypted vault and never imported or exported; the vault unlocks
for one signature and locks again; no key acts alone, because the keys sit on
one key page whose threshold is what authorizes; and the set changes by
changing that page, with no keys made in advance.

### The objects

- **The anchor.** A Bitcoin transaction with one `OP_RETURN` output carrying
  the network's identity, a Directory block height and that block's root chain
  anchor, sent from an address on the list in force at that height. One anchor
  per slot of `intervalBlocks` Bitcoin blocks, a parameter of the list in
  force; the proposer chooses the Directory block.
- **The anchoring page.** The key page `dn.acme/bitcoin/book/1`: one entry per
  anchoring operator, the hash of that operator's one key, at threshold `k`.
  It governs itself, the address-list account and `dn.acme/bitcoin`; the node
  keys' operator book governs none of them. Its membership and threshold
  change only by `k` of its keys; one key may replace only itself.
- **The vault.** Each operator's anchoring signer, a process separate from the
  node, creates the operator's secp256k1 key inside an encrypted vault. The
  key is never imported or exported. The vault unlocks for one signature and
  locks again. The one key signs the Bitcoin inputs, the coordination entries
  and the page's own transactions.
- **The address list.** A data entry in the network's address-list account,
  written by the page, that equals the page at the height of the block holding
  it — the same keys and threshold — and adds the Bitcoin descriptor, the
  addresses, each signer's coordination account and the parameters. A list is
  valid only as a successor of the list before it, by a **rotation spend**:
  the sweep from that list's addresses to the new list's, whose `OP_RETURN`
  carries the new list's hash, signed by `k` of the old list's keys. There is
  no backup list and no key made in advance.
- **The coordination accounts.** One data account per signer, named in the
  list, that only that signer can append to. An anchor is proposed there, each
  signer records its signature share there, and the broadcast is recorded
  there. Every entry is signed by the signer's vault key.
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
   entry signed by that signer's key. A transaction from a listed address with
   no record behind it is a theft of coins, not an anchor.
2. **The threshold is the page's.** The signers are the entries of
   `dn.acme/bitcoin/book/1` and the threshold is the page's `k`, both read at
   the height of the block holding the list. Neither is derived from the
   Directory's committee or from `ValidatorAcceptThreshold`: consensus
   thresholds are in stake (consensus.md "The thresholds"), a Bitcoin multisig
   counts keys, and the page is what authorizes, as staking's roster page is.

   > **Open for Paul (Q1, Q2).** Proposed: the page's members are the
   > Directory validator operators, one entry each, and `n` and `k` are
   > parameters of the page — `k` is the page's threshold at `h_L`, and no
   > number is fixed here, nor a floor such as `ceil(n × 2/3)`.
3. **A signer signs only its own root.** A signer contributes a share for
   (height, root) only if its operator's own Directory node holds `root` as the
   Directory's root chain anchor at `height`. The multisig is the check; no
   separate watcher compares anchors to the network. Every anchoring operator
   therefore runs a Directory node (Q1).
4. **The page is authoritative; no node key is.** Which addresses may anchor
   is decided on Accumulate and in Bitcoin together: a list is valid when the
   page wrote it, it equals the page at its height, and it succeeds the list
   before it by a rotation spend from that list's addresses signed by `k` of
   that list's keys (§2.2). The node keys and the operator book write no list,
   no fee entry and no coordination entry; fewer than `k` vault keys cannot
   change the list, make a record that counts, or make an anchor. Once a list
   is replaced, its keys cannot add anchors: an anchor counts only if it
   confirms before its list's successor is committed in Bitcoin (§2.2, the
   Bitcoin-order cut). A party holding `k` of a list's vault keys is the stated
   limit (§2.2 threat table).
5. **Each anchor is confirmed by a later one.** Anchor *i* has a confirming
   anchor *j* when a confirmed anchor whose height `H_j` is at or above the
   Directory height of the last record entry of *i* (its broadcast, or its
   last share) exists, and *j*'s own record verifies. The builder uses the
   first such *j*; *j* may itself lack one. Without *j*, *i* is
   *provisional*; the newest anchor always is. With *j*, a package is
   *final*. A verifier reports which.
6. **Trust begins at one pin and proceeds by induction.** The verifier is
   given the Bitcoin transaction ID of the first anchor, out of band. That
   anchor fixes the first list; each later list is a successor of the one
   before it by rotation. A re-pin (§2.7) starts a new chain from a newly
   published first anchor. No genesis, no snapshot, no node is trusted — and
   the walk crosses a network reorganization, which the major-block spine
   (`ProofService`, #4058) cannot.
7. **No consensus rule depends on anchoring.** No executor, no block, no
   `hashChains` input depends on whether or when an anchor was made. The
   coordination writes are ordinary transactions, and the executor accepts
   every `WriteData` as it does today. Every "invalid" or "ignored" below is a
   read-side rule applied by the anchoring software and by the verifier. A
   network with anchoring stalled runs as before.
8. **A proof is a package.** Once built, an evidence package is verified with
   Bitcoin alone. Once final it does not expire, and no Accumulate node need
   exist for it to be checked.

### What it proves, and what it does not

- It proves that the record existed by the Bitcoin block's time, that `k` of
  the anchoring operators attested to the root it sits under, and — through
  the account's main chain — that the transaction executed, which on
  Accumulate means its authorities' thresholds were met.
- It does not prove the decision was right. It is a proof of record.
- It does not prove anything before the first anchor through Bitcoin. Before
  that the existing spine applies, and the spine does not cross the 13 July
  2025 reorganization (`docs/incidents/2025-07-13-reorg-history-loss.md`).
  Promises of preserved proof begin at the first anchor.
- Finality takes the anchor interval plus Bitcoin confirmations (six). The
  anchor interval is a cost: at $0.12–$1 a transaction, an anchor every
  Bitcoin block (about ten minutes) is about $6,300–$52,600 a year, hourly
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
the anchoring software and by the verifier. The executor accepts every
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
| `dn.acme/bitcoin` | `dn.acme/bitcoin/book` | the accounts below |
| `dn.acme/bitcoin/book` | itself; page 1 at threshold `k` | one key per anchoring operator |
| `dn.acme/bitcoin/addresses` | `dn.acme/bitcoin/book` | address lists and fee entries (§2.2) |
| one coordination account per signer, named in the list (§2.2) | that signer's own key book only | that signer's coordination entries (§2.4) |

A page entry is the key's `BTCSignature` hash, `BTCHash` of the compressed
secp256k1 public key (`protocol/signature.go:128`), so the vault key signs
Accumulate transactions as a `BTCSignature`. The page changes by
`UpdateKeyPage` at threshold `k`, which adds or removes an operator or sets
the threshold, or by `UpdateKey`, which the key being replaced initiates alone
(`internal/core/execute/v2/chain/update_key.go:48-108`): one key can replace
only itself.

> **Open for Paul (Q1).** Proposed: no outer authority on
> `dn.acme/bitcoin/book`, so nothing but its own threshold changes it. An
> outer authority (`dn.acme/operators`, or governance) would let a page below
> threshold be repaired in place, and would let that authority change the
> page.

On an existing network the ADI, the books and the accounts are created by
operator-signed `CreateIdentity`, `CreateKeyBook` and `CreateDataAccount`
transactions, not by an executor change. Each operator's signer creates its key
in its vault (§2.8) and gives its `BTCHash`; page 1 is given every operator's
entry and set to threshold `k`; then `UpdateAccountAuth` removes
`dn.acme/operators` from `dn.acme/bitcoin` and `dn.acme/bitcoin/addresses`,
leaving the book as their only authority. That is done before the first list
is written; from then the node keys govern nothing under `dn.acme/bitcoin`.

Each coordination account is a data account with its own key book holding
only that signer's page key at threshold 1. Creating an account with a named
authority needs that authority's signature
(`internal/core/execute/v2/chain/create_utils.go:48-51`), so each signer
cosigns its own book's and account's creation with the page. A list may name a
signer's account from an earlier list; a new signer's account is created when
the list naming it is prepared. The anchoring software and the verifier ignore
a signer's entries in an account whose book does not hold that signer's key at
threshold 1. Its authority is its own book only. A signer that runs
`UpdateAccountAuth` on it locks only itself out; the others keep anchoring, and
the page removes it. Each operator funds its own account's page with credits
(`AddCredits`), and the operators together fund page 1; the credits-low alarm
is per signer (§2.6).

A spend's records are read from every coordination account its list names,
ordered by Directory block height, then signer index, then the entry's index
on the account's main chain. The verifier and the builder take the account
URLs from the list, not from this spec.

All accounts are ordinary data accounts. Every entry is a
`DoubleHashDataEntry` with exactly one `Data` element, the JSON of the record
below. The JSON is parsed, not canonical; the entry hash is over the bytes as
written. The entry's transaction executes under the account's authority as any
`WriteData` does; the signer of a coordination entry is read from its
signatures.

### 2.2 The address list

```json
{
  "type": "address-list",
  "version": 4,
  "network": "mainnet",
  "threshold": 5,
  "signers": [
    {"key": "02…", "coordination": "acc://dn.acme/bitcoin/c1"},
    …
  ],
  "descriptor": "tr(50929b74…803ac0,multi_a(5,…,…,…))",
  "addresses": ["bc1p…"],
  "intervalBlocks": 6,
  "maxLag": 1000
}
```

The numbers are examples; no `n` or `k` is fixed (Q2).

- `signers` are the page at `h_L`, the height of the block holding the list:
  each `key` is a compressed secp256k1 public key whose `BTCHash` is an entry
  of `dn.acme/bitcoin/book/1` at `h_L`, one signer per entry and one entry
  per signer, in the page's order (by that hash). `coordination` names the
  signer's account. The verifier checks records against these keys. A key
  rotation takes effect for anchoring only through a new list.

  > **Open for Paul (Q4).** Proposed: one key per operator, for both the
  > Bitcoin inputs and the coordination entries. The record is then not a
  > second key set; it stays as the coordination channel and the proof that
  > the signers judged (height, root).
- `threshold` equals the page's threshold at `h_L` and the descriptor's `k`; a
  list where they differ is invalid.
- `descriptor` is a BIP 380 output descriptor, written without its checksum:
  `tr(NUMS, multi_a(k, …))` (BIP 386; `OP_CHECKSIGADD`, BIP 342) for every
  `n`, the keys x-only in `signers` order. The internal key `NUMS` is the BIP
  341 point
  `50929b74c1a04954b78b4b6035e97a5e078a5a0f28ec96d547bfee9ace803ac0`, so that
  no one can spend by the key path. `addresses` are derived from it and listed
  for readers that do not parse descriptors. A key stays in successive lists
  while its operator stays on the page.

  > **Open for Paul (Q3).** Proposed: Tapscript `multi_a` for every `n`,
  > replacing `wsh(sortedmulti)` up to 20. At 4-of-8 an input is about 185 vB,
  > as with `wsh(sortedmulti)`, and each key adds about 8.5 vB; `n` is not
  > capped at 20; each signature fills its own slot, so shares arrive in any
  > order and none is redone. Not proposed: MuSig2 is `n`-of-`n` and keeps
  > nonce state between two rounds; FROST needs a distributed key generation,
  > so no key is created in one vault, and a new one at every page change.
- `intervalBlocks` is in Bitcoin blocks, at least 1 (6 is about an hour). The
  slot of Bitcoin height `b` is `floor(b / intervalBlocks)` (§2.4).
- `maxLag` is the number of stored Directory blocks behind its own tip beyond
  which a signer declines a proposed `H` (§2.4).
- **Well formed.** A list entry is well formed when: its `type` and `network`
  are as above; its signers and threshold equal the page at `h_L`; its keys or
  its threshold differ from the list in force, so that its addresses do; and
  `intervalBlocks ≥ 1`. Parameters therefore change with a page change. The
  anchoring software and the verifier ignore a list entry that is not well
  formed.
- **Size.** A list entry must fit one 20 KB data entry
  (`protocol/data_entry.go:27`). A signer takes about 190 bytes (its entry and
  its key in the descriptor), so about 100 signers fit, and the page holds no
  more than the network's `PageEntries`. A larger set needs the list split over
  several entries, which this spec does not define.

**Fee entries.** `fee` and the smallest spendable input are not in the list.
They are page-signed entries on `dn.acme/bitcoin/addresses`:

```json
{"type": "fee", "minSatPerVB": 2, "maxSatPerVB": 40, "maxFeeSat": 50000,
 "maxInputs": 24, "minInputSat": 10000}
```

applied from the block holding the latest such entry. Signers read the latest;
verifiers never need it. The rate must lie in `[minSatPerVB, maxSatPerVB]`,
the absolute fee must not exceed `maxFeeSat`, and the input count must not
exceed `maxInputs`. `maxInputs` is derived from the 20 KB transaction limit
(`protocol/data_entry.go:27`), which bounds the proposal entry and so the PSBT,
in hex: a `multi_a` input carries its leaf script (about `34n` bytes), the
control block and its `witness_utxo`, about `34n + 130` bytes, so about
`20,000 / (2 × (34n + 130))` inputs fit — 24 at `n = 8`, 8 at `n = 30`.
Replace-by-fee is always signalled. Each signer also enforces a fee cap its own
operator sets locally (§2.8).

**Commit, start and cut.** Every list `L` has a **commit**, a Bitcoin
transaction, and a **start**, a Directory height:

| How `L` came to be | Commit | Start `s_L` |
|---|---|---|
| The first list | the pinned first anchor | that anchor's `H` |
| Rotation | its rotation spend | `h_L + 1`, `h_L` from the rotation payload |

A transaction is *confirmed in* the Bitcoin block that includes it; counting
it valid also needs six confirmations. `L`'s **cut block** is the block of
the first confirmed commit of a successor of `L` — a rotation spend from
`L`'s addresses with its record — whether or not that successor completes. An
anchor `(H, R)` sent from `L`'s addresses counts only if `s_L ≤ H`, its block
is at or after the block of `L`'s commit, and — once `L` has a successor `L'`
in the chain — `H < s_L'` and its block is strictly before `L`'s cut block.
The second half is the **Bitcoin-order cut**: the keys of a replaced list
cannot add anchors for that list's heights later, because any anchor they make
confirms after the cut. Starts strictly increase along the chain; a chain
where they do not is invalid. Lists are read in main-chain order of
`dn.acme/bitcoin/addresses`.

**Succession.** The first list is fixed by the pinned first anchor. Every later
list N+1 is a successor of the list before it, N, when:

- (a) the list entry is well formed, its transaction has principal
  `dn.acme/bitcoin/addresses`, its hash is `L`, and its receipt proves it on
  that account's main chain (only a successful transaction reaches a main
  chain, so `k` of the page signed it), its receipt ending at the root of the
  first anchor under N+1 (§2.4), which counts under the cut (payload, address,
  six confirmations); and
- (b) its **rotation spend** from N's addresses has six confirmations. The
  rotation spend is the sweep from N's addresses to N+1's (§2.3): its
  `OP_RETURN` carries `ACR1`, the network tag, `h_L` and `L`; its record is a
  proposal and shares from N's `threshold` signers, each entry signed by that
  signer's key in N, and its broadcast counts (§2.4). Its inputs are signed by
  `k` of N's keys.

A rotation therefore needs `k` of N's vault keys — the same as a forged anchor
under N — and the node keys cannot change the list. Each signer signs a
rotation spend only on its operator's approval, and checks `h_L`, `L` and the
page at `h_L` against its own node (§2.8). The anchor the receipt ends at need
not carry its record here: `L` is already signed by `k` of N's keys.

- **Competing successors.** If more than one rotation spend from N's addresses
  confirms, the one confirmed first (block height, then position in the
  block) commits N's successor; the other lists, and every list after them,
  are not part of the chain. The verifier checks the chain the package
  presents, and applies this rule only when shown both or when it scans (§2.6
  step 6).
- **Stated limit.** A party holding `k` of a list's vault keys can present a
  verifier with an alternate history from that list onward: anchors confirmed
  after the honest successor's commit, a chain that leaves that successor out,
  and rotations to lists of its own. For the list in force, that is `k` of the
  anchoring operators — the trust anchoring rests on — and nothing in band
  answers it: the response is a re-pin (§2.7). For a past list, the
  Bitcoin-order cut voids its later anchors and its forged rotation confirms
  after the honest one, so it loses whenever the verifier sees the honest
  successor: shown both, or by scanning the list's addresses (§2.6 step 6). A
  verifier that is shown only the thief's chain and does not scan accepts it.
  An operator destroys its vault once its key has left the page (§2.8), as
  `asp install` deletes the old vault on a staking rotation; keys that stay on
  the page stay live, so for a past list the cut, not key destruction, is what
  stops its keys adding anchors.

**Funding.** The rotation spend moves N's funds: its outputs other than the
`OP_RETURN` go only to N+1's addresses, at least `ceil(7 / intervalBlocks) +
1` of them (N+1's `intervalBlocks`), each of at least `minInputSat`, so that
N+1 has an input for every slot while each waits its six confirmations. After
the rotation spend, an **old-list sweep** moves what is left: a recorded spend
from N's addresses with no `OP_RETURN` and outputs only to N+1's addresses,
proposed and shared by N's signers in their coordination accounts on N's slot
schedule. It needs `k` of N's keys, and stays possible until enough of them
have left the page and their vaults are destroyed; coins paid to a list's
addresses after that are lost, as with any payment to a destroyed key. The
float — what the list's addresses hold — is kept small, because below
threshold it is lost (Q5).

**Rotation.** Every change of the page — an operator added or removed, the
threshold changed, or an operator replacing its own key — requires a new list:
the page changes at Directory height `h_P`, and the page writes a list entry
at `h_L ≥ h_P` that equals the page at `h_L`. Its rotation spend should confirm
within two slots (`intervalBlocks` Bitcoin blocks each) of the page change.
This is an operational requirement, not a consensus rule. Until the rotation
spend has six confirmations the old list is in force for coordination and its
shares count; from then, signers anchor under the new list (§2.4), and its
first anchor completes the succession. A later list entry supersedes a pending
one; that is how a pending list is cancelled. Proposed (Q1): the page follows
the Directory's committee — an operator joins when its validator does and
leaves when it does — as an operational rule, not a check.

**Lost operators.** Up to `n − k` operators may lose their keys and the list
still signs. The remaining `k` sign the page change that removes or replaces
each lost key, the new list, and its rotation spend under the old script. There
is no backup and no key made in advance. The signer-margin alarm (§2.6) fires
while fewer than `k + 1` signers are sharing, before one more loss stops the
list.

**Below threshold.** With fewer than `k` usable keys the page cannot sign its
own change, and the old script's coins cannot move, so no rotation spend can
link a new list. The recovery is a re-pin (§2.7), and the float is lost. A
timelocked recovery leaf in the script would need its keys fixed in advance,
which this design removes, so there is none.

> **Open for Paul (Q5).** Proposed: below threshold, re-pin and lose the
> float. With no outer authority (Q1) the page and `dn.acme/bitcoin/addresses`
> cannot be repaired either, so a re-pin creates the accounts of §2.1 afresh
> under a new ADI.

**Threat table.** What a party holding each key or set can do, and what stops
it.

| Holds | Can do | Stopped by |
|---|---|---|
| `k` of the current vault keys | Forge anchors and records, rotate to a list of its own | Nothing in band: it is `k` of the anchoring operators, the trust anchoring rests on. The unrecorded-spend alarm, or a forged record seen by the honest signers; the response is a re-pin (stated limit) |
| Fewer than `k` vault keys | Write those signers' coordination entries; replace those keys on the page (`UpdateKey`) | No spend, no list, no fee entry and no record that counts; a replaced key changes the page, which the operators see, and the page removes it |
| A vault file, copied | Nothing without its passphrase | The vault's encryption (§2.8) |
| One signer host while its signer runs | Sign as that signer, within its checks | One key; the signer's checks (§2.8); below threshold alone; the page removes it |
| The node keys (a threshold; they meet the operator book) | Nothing under `dn.acme/bitcoin` once set up | The operators' authority is removed (§2.1); a node proposes but holds no anchoring key (§2.8) |
| `k` of a past list's vault keys | Present an alternate history from that list onward | The Bitcoin-order cut voids its later anchors; its rotation confirms after the honest one; a verifier that is shown the successor or scans rejects it; one shown only the thief's chain and not scanning accepts it (stated limit) |
| Losses beyond `n − k` | Stop anchoring under the list | The signer-margin alarm before it; a re-pin after it (Q5) |
| One faulty signer's coordination entries | Write proposals, shares and broadcasts in its own account | A share counts only if its partial signature validates; a broadcast counts only with `threshold` such shares for its `T`; a proposal counts only in its slot (§2.4); below threshold alone |
| A lagging or eclipsed Bitcoin node at a signer | Misjudge an input or the slot | That signer declines or shares late; a split costs one slot; it cannot make others sign (§2.4 margins) |
| An eclipsed verifier | Be shown a false Bitcoin chain | Six blocks of valid proof of work at the current difficulty; an SPV client is only as safe as that (§2.8) |
| A Bitcoin reorganization deeper than six blocks | Remove a confirmed anchor | The builder re-derives; a package built on the removed block no longer verifies; a party wanting more margin waits for more confirmations |
| A package server | Show only one chain of lists | Stated limit; the scan (§2.6 step 6) |
| One far-ahead leader header | Move Accumulate block time | No rule depends on block time (§2); bounding it is DIFFERENCES C23 |
| One locked-out signer | Lock only its own coordination account | Per-signer accounts (§2.1); the others keep anchoring; the page removes it |

### 2.3 The anchor transaction

Four kinds of **recorded spend** leave a list's addresses: the anchor, the
rotation spend (§2.2), the sweep and the old-list sweep (§2.2 "Funding"). Each
has a proposal, shares from `threshold` distinct signers and a broadcast
(§2.4). A spend from a list address that is none of these is unrecorded.

- Inputs: one or more outputs of the list's addresses, each of at least
  `minInputSat`. Any payment to a list address funds it. The proposer uses
  inputs with at least six confirmations in its own Bitcoin view; a signer
  shares for inputs with at least one. No input may be an input of an
  **outstanding** recorded spend: one whose broadcast counts (§2.4) and which
  is not yet confirmed. Its inputs are free again once it is conflicted, or
  once a later recorded spend of the same list confirms. A replacement (§2.4)
  spends exactly the inputs of the transaction it replaces; a rotation spend
  may instead spend exactly the inputs of an outstanding spend, replacing it
  (§2.4 "What a slot proposes").
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
- Signatures: Tapscript script-path signatures (BIP 342) with
  `SIGHASH_DEFAULT`. A share with any other sighash type is invalid.
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
carrying the unsigned transaction as a PSBT (BIP 174, with the BIP 371 Taproot
fields); `T` below is a proposal's unsigned transaction ID, which for
segregated-witness inputs is also the signed transaction's ID.

- Anchor: `{"type":"proposal","slot":s,"height":H,"root":R,"psbt":"…"}`.
- Rotation spend: `{"type":"rotation","slot":s,"height":h_L,"list":L,"psbt":"…"}`.
- Sweep or old-list sweep: `{"type":"sweep","slot":s,"psbt":"…"}`.
- Replacement: `{"type":"replace","slot":s,"replaces":T,"psbt":"…"}`.

**Judging.** A signer judges each proposal once, when its operator's node
processes the Directory block holding it, against its own Bitcoin view at that
moment; a proposal judged invalid is not judged again. It takes proposals in
the order of §2.1. A proposal counts for the signer only if its `slot` is the
signer's current slot or the one before. The designated proposer's proposal
counts at once; another signer's counts only once the signer's Bitcoin tip is
at least `s × intervalBlocks + ceil(intervalBlocks / 2)` — with
`intervalBlocks = 1` that is the next slot, which the one-behind rule admits.
A signer shares at most one anchor, rotation or sweep proposal per slot of a
list, the first that counts and passes its checks. These are liveness rules
for signers: the verifier checks only the payload, the address, the
confirmations and the record. A split costs the slot; the next slot has
another proposer.

**What a slot proposes.** A well-formed list entry (§2.2) on
`dn.acme/bitcoin/addresses` after the list in force, present at `h_L` on the
signer's operator's own node, is **pending** until its rotation spend
confirms; only the latest such entry is pending, so a later entry supersedes,
and so cancels, an earlier one. Only the page writes list entries, so only `k`
of the page's keys can make a list pending.

- While a list is pending, no anchor or sweep is proposed or shared under the
  list in force, and replacements of its outstanding spends continue (step 4).
  The slot's proposal is the rotation spend, at once: from free inputs, or
  spending exactly the inputs of an outstanding spend at a higher fee rate, so
  that it replaces that spend (BIP 125), whose payload is then dropped and
  anchored again under the new list. An outstanding spend that cannot be
  bumped therefore does not hold the rotation back. Anchoring is halted while
  a list is pending; that is a stated limit, bounded by the pending-list alarm
  (§2.6).
- Once the rotation spend has six confirmations, signers propose and share
  under the new list: its signers, slot schedule, coordination accounts and
  parameters, with `H ≥ h_L + 1`. The first such anchor that confirms
  completes the succession (§2.2).
- After its rotation spend, the old list's signers may propose old-list sweeps
  under the old list's schedule (§2.2 "Funding").
- Otherwise the slot's proposal is an anchor, or a sweep when the list's
  inputs need consolidating.

At each slot, in order:

1. **Proposal.** For an anchor, the designated proposer chooses `H`: a stored
   Directory block at least 10 blocks below its own tip, and strictly above
   the `H` of the latest counted anchor broadcast; `R` is that block's
   `RootChainAnchor`. The margins of height and of input confirmations absorb
   the lag of the other signers' nodes.
2. **Shares.** Every signer checks the proposal:
   - for an anchor: its operator's node holds `H` with root `R`, and `H` is
     above the `H` of the latest counted anchor broadcast; as a local liveness
     check it may decline an `H` more than `maxLag` stored blocks behind its
     own tip;
   - for a rotation spend: the list is the pending one, its node holds its
     entry with hash `L` at `h_L`, the entry equals the page at `h_L`, and the
     outputs go to that list's addresses; its operator has approved `L`
     (§2.8);
   - the PSBT has exactly the outputs of §2.3 for its kind, with the payload
     for the proposal;
   - every input meets §2.3, is an output of a list address of at least
     `minInputSat`, and its prevout (txid, vout, scriptPubKey, amount) matches
     the signer's **own** Bitcoin node, not the PSBT's `witness_utxo`
     (CVE-2020-14199);
   - the sighash type is as §2.3, and the fee rate, absolute fee and input
     count are within the latest fee entry's bounds and its local cap (§2.8).

   If all hold it writes `{"type":"share","txid":T,"sig":"…"}`, `sig` being
   its BIP 174 partial signatures, hex (`PSBT_IN_TAP_SCRIPT_SIG`, one per
   input). The vault unlocks once for the share: it signs every input and the
   share entry's transaction, and locks (§2.8). If any check fails it writes
   nothing. A share **counts** only if its partial signature validates, for
   every input, against the reading signer's own prevouts; at most one share
   per signer per `T` counts.
3. **Broadcast.** When counted shares from `threshold` distinct signers for
   `T` exist, the proposer finalizes and broadcasts, and writes
   `{"type":"broadcast","txid":T}`. A broadcast **counts** only if the record
   holds a proposal whose unsigned transaction ID is `T` and counted shares
   for `T` from `threshold` distinct signers of the list. A broadcast that
   does not count is ignored: for the `H` rule, for outstanding inputs and
   for the alarms. One faulty signer's entries alone therefore move nothing.
   Every signer finalizes and broadcasts any `T` with `threshold` counted
   shares that it does not see in its mempool or chain, so a proposer that
   writes `broadcast` but does not send the transaction delays nothing.
   Finalizing needs no key: the shares carry the signatures.
4. **Replacement.** If a counted broadcast's transaction is unconfirmed after
   `intervalBlocks` Bitcoin blocks and the fee bounds allow a higher rate, the
   designated proposer of the current slot writes a `replace` proposal for
   it. A replacement has the same payload and output destinations, spends
   exactly the same inputs and has a strictly higher fee rate; its absolute
   fee rises by at least the incremental relay fee times its size (BIP 125).
   It is exempt from the `H` rule and from the one-per-slot rule; a signer
   shares at most one replacement of a given `T` per slot. It is shared and
   broadcast as in steps 2 and 3, and may itself be replaced, forming a
   chain. The record keeps all; the one that confirms is the spend. A
   replacement without a fee increase is not proposed, because Bitcoin nodes
   reject it: when the caps block a bump, operators raise them (a fee entry
   and the signers' local caps), or the transaction expires from mempools
   and its inputs stay outstanding until a later recorded spend of the list
   confirms. That is a stated limit; the rotation spend's several outputs
   leave other inputs to anchor with, and a pending list's rotation spend may
   replace the stuck spend (above).

Coordination entries are read by every signer, so the process needs no
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
its rotation spend's record and Bitcoin data.

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
     "rotation": {"txid": "…", "record": {"proposal": E, "shares": [E]}}}
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
  anchor, the anchor, *j*, and every `anchorTxid` and rotation `txid` in
  `list.chain` — to its `gettxoutproof` form: the block `header`, the
  `merkleProof`, and `prevTxs`, the funding transactions, so that partial
  signatures can be checked against the prevout amounts and scripts and the
  sending address read. A full node with `txindex` may skip it.
- `list.chain` is the sequence from the first list to the list in force at
  the Bitcoin block of *j* (of the anchor, when it is provisional): every
  successor whose commit is confirmed at or before that block is in it, so
  that the verifier can apply the Bitcoin-order cut (§2.2). Each item is the
  list entry, the Directory height of the block holding it, its receipt into
  the root of the anchor `anchorTxid` (for the first list, the first anchor),
  and, after the first, `rotation`: the rotation spend's txid and record,
  checked by signatures only.
- Each share's `sig` is the BIP 174 partial signatures, hex (§2.4).

### 2.6 The verifier

Given a package, the pinned first anchors in order (§2.7), and Bitcoin:

1. Fetch the package's first anchor; it is one of the pins, `p`. Decode its
   payload; the address it was sent from is on the first list, and the first
   list's receipt ends at that payload's root. Walk `list.chain`: each next
   list is well formed and a successor of the one before it by rotation
   (§2.2), every Bitcoin transaction checked as in step 2 and every record by
   its signers' keys. The carried height equals the payload's `h_L`. Each
   list's start (§2.2) is strictly above the one before. This yields each
   list's commit block, cut block and start.
2. Check `bitcoin` against `anchor.txid`: the header and Merkle proof (SPV, or
   a full node's own view). The payload is `(directory.height,
   directory.root)`; every input is an output of an address of one list `L` of
   the chain; the anchor counts under `L` by the Bitcoin-order cut (§2.2):
   `s_L ≤ H`, confirmed at or after `L`'s commit, and, when the chain has a
   list after `L`, below that list's start and before `L`'s cut block; it has
   `n ≥ 6` confirmations; and, when a pin after `p` exists, it is confirmed
   before that pin's block (§2.7). Ignore the package's `block` if it
   disagrees.
3. Check the receipt from `entry.hash` to `directory.root`. For an entry of a
   data account's main chain, also check `hash(transaction) == entry.hash`,
   that the body is `WriteData` with a `DoubleHashDataEntry` of exactly one
   `Data` element, and the entry's signatures. An entry of another chain is
   proven by its receipt only.
4. Check the anchor's record: a proposal for `(height, root)`, any `replace`
   chain, shares for the final `T` from `threshold` distinct signers of `L`
   (the stored count), each entry signed by its signer's key, each share a
   valid partial signature over that PSBT, with each `E` receipted into the
   root of *j*. *j* is a confirmed anchor whose `H_j` is at or above the
   Directory height of the last record entry of this anchor; check *j* as in
   step 2, under the cut, and its own record by signatures only. *j* may
   itself be provisional. If the package has no *j*, the result is
   *provisional*.
5. Report: the entry, the height, the Bitcoin block and its time, and the
   state — *final* with a *j*, *provisional* without. The entry existed before
   the confirming block was mined. The header time is approximate, in either
   direction, by about two hours. A conservative upper bound is the median
   time past of the block eleven blocks after the confirming block, on the
   assumption that most hash power stamps honestly; the report says so.
6. Optionally, with a full node that can scan addresses (`scanblocks` with
   `blockfilterindex`, or an indexer it trusts), look for what the chain
   leaves out: for each list in the chain, a confirmed `ACR1` spend from its
   addresses at or before the block of *j* that is confirmed before the
   chain's next rotation, or, for the last list, any such spend. That means
   the package shows a chain the network superseded: the report says
   *contested*. A verifier without such a
   node cannot do this check; for it, the one-chain limit of §2.2 stands.

A package is checked with Bitcoin alone. The alarms are a separate watcher
that needs a Directory node to read the broadcast entries; run continuously
against one it raises these alarms, all liveness, not safety:

- **Overdue:** no confirmed anchor in three slots.
- **Unrecorded spend:** an output of a list address was spent by a
  transaction that is not a recorded spend — no proposal with that `T` and
  counted shares from `threshold` signers in the network's own coordination
  accounts. That is a compromise of `k` vault keys; the response is a re-pin
  (§2.7).
- **Signer margin:** fewer than `k + 1` distinct signers of the list in force
  have written a counted share in its last six slots: one more loss stops
  anchoring under the list (§2.2 "Lost operators").
- **Pending list:** a list has been pending for more than two slots;
  anchoring is halted until it is rotated to or superseded.
- **Credits low:** a signer's coordination account, or page 1, is short of
  credits for the writes of the coming slots.

### 2.7 What is pinned out of band

One value: the first anchor's Bitcoin transaction ID. It is published in the
documentation and on the network's website, and nothing else is. Every other
fact the verifier uses is derived from it through Bitcoin and the receipts in
the package.

**Re-pin.** When the page falls below threshold (§2.2) or `k` of its keys are
compromised, the operators create the accounts of §2.1 afresh under a new ADI,
each with a fresh vault key, and publish the first anchor of the new chain the
same way. The published pins are ordered. A package descends from one pin; when
a later pin exists, its anchor counts only if it confirmed before the later
pin's first anchor (§2.6 step 2), so a chain kept going under lost or stolen
keys after the re-pin proves nothing. Packages that end before the re-pin stay
valid. The coins on the old chain's addresses are lost unless `k` of its keys
still sign (Q5).

### 2.8 Layout

| Component | Where |
|---|---|
| Payload, list and record types | `pkg/anchoring` |
| Coordination (propose, judge, broadcast, replace, rotate, sweep) | `internal/anchoring`, started by the Directory's validator service when a list names its operator's key |
| Anchoring signer: the vault, install, and signing after its checks | `internal/anchoring/signer`, run as its own process beside the node |
| Proof builder and package service | `internal/anchoring/proofs`, started by a follower's API when configured |
| Verifier and alarms | `tools/cmd/anchor-verify` |

**The vault signer (a requirement of B2).** Each operator's key is held by an
anchoring signer separate from the node process, as staking's `asp` is
separate from the wallet:

- **Created in the vault.** Install creates an encrypted vault and generates
  one secp256k1 key inside it, and prints only the public key and its
  `BTCHash` for the page. There is no import and no export: moving a signer to
  another host copies the encrypted vault, as `asp move` does, so the key
  never leaves in plaintext.
- **Unlock, sign, lock.** Every signature is one unlock: the vault opens, signs
  and locks again. A share is one unlock that signs every input and the share
  entry's transaction (§2.4 step 2). Tapscript signatures keep no state
  between calls (no nonces), so nothing stays unlocked between rounds. A
  signer run as a service holds its passphrase in its process environment for
  its whole life, as staking's services do, so locking per signature protects
  the key at rest and between signatures, not against a compromise of the
  running host.
- **One key, two schemes.** The key signs Accumulate transactions as a
  `BTCSignature` (ECDSA) and Bitcoin inputs as BIP 340 Schnorr in Tapscript.
  It signs nothing else.
- **On the operator's approval.** Page changes, list entries, fee entries and
  rotation spends are signed only on its operator's command, as staking's page
  changes are, never on the coordination path. Anchors, sweeps, old-list
  sweeps, replacements and coordination entries are signed by the coordination
  path, after the checks below.
- **Destroyed when it leaves.** When the operator's key leaves the page and
  every list naming it has been rotated away from with six confirmations, and
  its old-list sweeps are done, the operator destroys the vault and its copies.

The signer checks before it unlocks, against its operator's own Directory node
and its own Bitcoin node, never on the coordinating node's word:

- every input's prevout, from its own Bitcoin node;
- for an anchor: `R` is its node's root at `H`, and the only other output is
  change to the list's own addresses;
- for a rotation spend: `h_L` is the height of the block holding `L` on its
  node, `L` equals the page at `h_L` and is the entry its operator has
  approved, and every output but the `OP_RETURN` goes to the addresses of the
  list `L` names, in the number §2.2 requires;
- for a sweep: every output goes to the list's own addresses; for an
  old-list sweep, to its successor's;
- for a replacement: the same inputs and the same output destinations as the
  transaction it replaces;
- for a coordination entry: it is a `WriteData` to its own coordination
  account carrying the entry it was asked to sign;
- the fee is below a cap its operator sets locally, whatever the fee entries
  say.

The node process holds no anchoring key: a compromised node host can propose,
but not sign. A compromised signer host signs as one key, within its checks,
and below threshold alone.

Bitcoin access is through a configured node's RPC or a light client; the
verifier accepts either. A light client must validate block headers and proof
of work (SPV); an Electrum-style server without that check is a trusted party.
