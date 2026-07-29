# Receipt Soundness in Accumulate

**Status:** analysis, current `main` (`d6ddea514`)
**Scope:** `pkg/database/merkle`, `protocol`, `internal/core/execute/v2`, with `v1` noted where it diverges.

## 0. What is claimed

Let *R* be a `merkle.Receipt` accepted by `Receipt.Validate`, whose `Anchor` is a value
this network has registered as an anchor, and whose `Start` equals `T = Transaction.GetHash()`
for a transaction *X* that the claimant exhibits.

**Claim 1 (No external construction).** An attacker cannot produce such an *R* by choosing
receipt entries. Forging *R* reduces to a SHA-256 preimage or a cross-family collision.

**Claim 2 (No induced entry).** An attacker cannot cause the protocol to *emit* such an *R*
for a *T* that is not genuinely a node of an anchored chain — in particular, cannot obtain a
receipt for a fabricated transaction that has executed nowhere.

Both claims hold on current `main`, and Claim 2 holds **because of two deliberate controls**
(`DoubleHashDataEntry` and the 64-byte body rejection), not as an emergent property. §7 states
the residuals precisely. **Claim 2 does not hold for history anchored before
`ExecutorVersionV1DoubleHashEntries`** (§7.3).

The claims are about receipt *soundness*. Neither claim establishes that *X* **executed**.

> **Principal negative result (§7.1).** This is a **design constraint, not a defect.** No
> deployed code consumes a BPT-rooted receipt as evidence of execution, so nothing is
> exploitable and consensus is unaffected (§7.1.3). But if such a proof were built, it would not
> work: `hashPending` commits every *pending* transaction's hash into the account's BPT leaf via
> a raw `AddTxID` insertion, so a merely-pending transaction — never executed, possibly never
> executable — has an arithmetically valid receipt to a registered `StateTreeAnchor`. The BPT
> route is the only route available for proving execution to a foreign verifier, and **it cannot
> distinguish executed from pending.** `hashPending` is behaving correctly; the constraint falls
> out of the inheritance property (§3.1), not out of a bug.

---

## 1. The construction

### 1.1 Node combination

`pkg/database/merkle/hash.go:26-31`:

```go
func combineHashes(a, b []byte) []byte {
	h := sha256.New()
	h.Write(a)
	h.Write(b)
	return h.Sum(nil)
}
```

Bare `sha256(a || b)`. No leaf/internal tag, no length prefix, no domain separator.

### 1.2 Leaves are not hashed

`pkg/database/merkle/state.go:171-185`:

```go
func (m *State) AddEntry(hash_ []byte) {
	hash := copyHash(hash_)
	m.HashList = append(m.HashList, hash)
	m.Count++
	m.pad()
	for i, v := range m.Pending {
		if v == nil {
			m.Pending[i] = hash   // <-- raw entry stored as leaf
			return
		}
		hash = combineHashes(v, hash)
		m.Pending[i] = nil
	}
}
```

The entry value *is* the leaf. There is no `H(leaf)` step. Consequently **a leaf and an
internal node are indistinguishable by shape**: both are 32 bytes, and an internal node's
value is a syntactically valid leaf value.

All chain entries are exactly 32 bytes, enforced at `internal/database/chain.go:139-158`
(`>32` panics; `<32` is zero-padded with a fake field byte).

### 1.3 The verifier

`pkg/database/merkle/receipt.go:15-22, 47-58`:

```go
func (n *ReceiptEntry) apply(hash []byte) []byte {
	if n.Right {
		return combineHashes(hash, n.Hash)
	}
	return combineHashes(n.Hash, hash)
}

func (r *Receipt) Validate(opts *ValidateOptions) bool {
	MDRoot := r.Start
	for _, node := range r.Entries {
		if !opts.allowEntry(node) {
			return false
		}
		MDRoot = node.apply(MDRoot)
	}
	return bytes.Equal(MDRoot, r.Anchor)
}
```

`Validate` is pure arithmetic. It does **not** check that `Start` is a leaf, does not know
which chain it came from, and does not consult indices. `Combine` (`receipt.go:95-108`) only
checks `r.Anchor == s.Start`.

`allowEntry` (`receipt.go:28-42`) requires 32-byte entries unless `Relaxed`. **`Relaxed` is
never set outside tests** (verified by grep: the only non-test references are the field
declaration and its own read). Every production call site passes `nil` opts.

---

## 2. Claim 1 — external construction is infeasible

Work the fold backwards. Let *A* be a registered anchor. *A* is a real node, so
`A = combineHashes(L, R)` for real children *L*, *R*.

The final `apply` in a validating receipt must produce *A*. By §1.1 that means
`sha256(e || v) = sha256(L || R)` (or `sha256(v || e)`), where *v* is the running value and
*e* the supplied entry. Absent a SHA-256 collision, `e || v = L || R`. Since `|e| = |v| = 32`
(§1.3), this forces `v = R` (or `v = L`).

By induction, the running value at every step must equal an existing node of the real tree.
At step 0 the running value is `r.Start`. Therefore:

> **A receipt validates to a registered anchor only if `Start` is already a node — leaf or
> internal — of the real anchored structure.**

Two remarks on why this is not circumventable:

- **`Relaxed` does not help.** With variable-length entries an attacker could try
  `sha256(e || v) = sha256(L||R)` with `|e| ≠ 32`. But that still requires
  `e || v = L || R` as *byte strings*; the split is forced by `|v| = 32` because `Start` is
  bound to a 32-byte transaction hash at every trust site. Different lengths change the
  concatenation, they do not create a second valid parse of the same 64 bytes.
- **The attacker cannot assert `Start`.** Every trust site recomputes it from an exhibited
  object — `msg_synthetic.go:110-112` (`bytes.Equal(h[:], syn.Proof.Receipt.Start)`),
  `create_token_account.go:116`, v1 `signature.go:213-215`, `protocol/signature.go:829`.

So Claim 1 reduces to: *make my transaction hash equal an existing node.* Since `T` is a
SHA-256 output over an object the verifier re-hashes, this requires a preimage, or a
birthday collision across two structured families (~2^128 even grinding both sides).

**Claim 1 holds.**

---

## 3. The inheritance property — where the real risk lives

Claim 1 says `Start` must be a node. It does **not** say `Start` must be a *chain entry*.
Internal nodes qualify. This matters more than it first appears, because Accumulate computes
several protocol values with the *same* construction as `combineHashes`.

### 3.1 The transaction hash is a node-shaped value

`protocol/transaction_hash.go:43-72`:

```go
// Hash calculates the hash of the transaction as H(H(header) + H(body)).
func (t *Transaction) calcHash() {
	...
	header, err := t.Header.MarshalBinary()
	headerHash := sha256.Sum256(header)
	t.header64bytes = len(header) == 64

	bodyHash, is64 := t.getBodyHash()
	t.body64bytes = is64

	sha := sha256.New()
	sha.Write(headerHash[:])
	sha.Write(bodyHash)
	t.hash = sha.Sum(nil)
}
```

That is `T = sha256(headerHash || bodyHash)` — **byte-identical to `combineHashes(headerHash, bodyHash)`**.

**Lemma (inheritance).** If a transaction *X* with hash *T* is anchored, then `bodyHash(X)`
has a valid receipt to the same anchor, namely:

```
Start   = bodyHash
Entries = [ {Hash: headerHash, Right: false} ] ++ <real receipt for T>
```

The first `apply` yields `combineHashes(headerHash, bodyHash) = T`; the rest is *X*'s genuine
path. Every entry is 32 bytes, so `allowEntry` passes. Symmetrically `headerHash` inherits via
`{Hash: bodyHash, Right: true}`.

`bodyHash` was never written to any chain. It does not need to be. **Any value that is a
child of a node is receipt-reachable.**

### 3.2 Inheritance recurses through `hashWriteData`

`protocol/transaction_hash.go:91-108`:

```go
func hashWriteData(withoutEntry TransactionBody, entry DataEntry) []byte {
	data, err := withoutEntry.MarshalBinary()
	hasher := new(hash.Hasher)
	hasher.AddBytes(data)
	if entry == nil {
		var zero [32]byte
		hasher.AddHash(&zero)
	} else {
		hasher.AddHash((*[32]byte)(entry.Hash()))   // raw, un-rehashed
	}
	return hasher.MerkleHash()
}
```

`internal/core/hash` is a type alias for `merkle.Hasher` (`internal/core/hash/compat.go`), and:

- `AddBytes(v)` → `append(v)` → stores `sha256(v)` (`hasher.go:62-64, 23-26`)
- `AddHash(v)` → stores `v` **raw** (`hasher.go:28-32`)
- `MerkleHash()` builds a `State` and returns `Anchor()` (`hasher.go:117-132`)

For exactly two elements, `MerkleHash()` = `combineHashes(h0, h1)`. Therefore for
`WriteData`/`WriteDataTo`/`SyntheticWriteData`/`SystemWriteData`
(`transaction_hash.go:110-132`):

```
bodyHash = combineHashes( sha256(bodyWithoutEntry), entry.Hash() )
```

So `entry.Hash()` **inherits a receipt to the anchor**, via `bodyHash`, via `T`:

```
entry.Hash()  --{sha256(bodyWithoutEntry), left}-->  bodyHash
              --{headerHash, left}-------------->    T
              --<genuine receipt>--------------->    anchor
```

> **The rule.** `AddBytes` re-hashes and is safe. `AddHash` / `AddHash2` / `AddValue` insert
> raw and confer inheritance on their argument. Any attacker-influenced value reaching an
> anchored structure through a raw insertion is receipt-reachable.

Raw-insertion inventory (`AddHash`, `AddHash2`, `AddValue`, `AddTxID`) reaching anchored
structures — each is an inheritance edge and must be re-checked whenever one is added:

| Site | Raw value inserted | Consequence |
|---|---|---|
| `protocol/transaction_hash.go:104` | `entry.Hash()` | §3.3 — the historical vulnerability |
| `pkg/database/merkle/hasher.go:99-107` (`AddTxID`) | `combine(txHash, accountHash)` | **§7.1 — pending transactions inherit** |
| `internal/core/execute/internal/bpt_prod.go:102` | `AddTxID(txid)` for each pending txid | §7.1 |
| `internal/core/execute/internal/bpt_prod.go:124` | `sig.Hash()` (validator sigs) | signature hashes inherit |
| `internal/core/execute/internal/bpt_prod.go:129` | `AddHash2(hash)` (credit payments) | payment hashes inherit |
| `internal/core/execute/internal/bpt_prod.go:166` (`hashValue`→`AddValue`) | sub-hasher `MerkleHash()` | composes the leaf; §7.1 |
| `pkg/types/messaging/types.go:186` | `m.Proof.Receipt.Anchor` | deprecated, §7.5 |

Note `AddTxID` (`hasher.go:99-107`) is itself a 64-byte-preimage construction:

```go
func (h *Hasher) AddTxID(v *url.TxID) {
	...
	u := v.Hash()
	x := combine(u[:], v.Account().Hash())
	h.AddHash((*[32]byte)(x))
}
```

`x = sha256(txHash || accountHash)` = `combineHashes(txHash, accountHash)`. By the §3.1 lemma,
**`txHash` inherits a receipt from `x`** via `{Hash: accountHash, Right: true}`; `accountHash`
is `url.URL.Hash()`, 32 bytes, so `allowEntry` passes.

### 3.3 The attack this enables, and why the fix is what it is

`entry.Hash()` is receipt-reachable and attacker-influenced. If an attacker can make
`entry.Hash()` equal `T'` for a **fabricated** transaction *X'*, they obtain a valid receipt
to a real network anchor for a transaction that executed nowhere — purchased with one
legitimate, successful `WriteData`. They can exhibit *X'* (they built it), so the
`Start == X'.GetHash()` binding at every trust site is satisfied. This is a **construction**,
not a search.

`AccumulateDataEntry.Hash()` (`protocol/data_entry.go:29-35`):

```go
func (e *AccumulateDataEntry) Hash() []byte {
	h := make(hash.Hasher, 0, len(e.Data))
	for _, data := range e.Data {
		h.AddBytes(data)
	}
	return h.MerkleHash()
}
```

Two concrete instantiations:

- **Two parts.** `MerkleHash()` over two elements = `sha256(sha256(d0) || sha256(d1))`.
  Set `d0 = marshal(header')`, `d1 = marshal(body')`. Then
  `entry.Hash() = sha256(headerHash' || bodyHash') = T'` **exactly**.
- **One part.** `MerkleHash()` over one element returns it unchanged (`State.Anchor()` with
  `Count == 1` returns `Pending[0]`), i.e. `sha256(d0)` with `d0` arbitrary and
  arbitrary-length. Set `d0 = headerHash' || bodyHash'` (64 bytes). Then `entry.Hash() = T'`.

Both work because the *preimage* under the outer SHA-256 is attacker-shaped.

`DoubleHashDataEntry.Hash()` (`data_entry.go:41-50`):

```go
func (e *DoubleHashDataEntry) Hash() []byte {
	h := make(hash.Hasher, 0, len(e.Data))
	for _, data := range e.Data {
		h.AddBytes(data)
	}
	// Double hash the Merkle root
	hh := sha256.Sum256(h.MerkleHash())
	return hh[:]
}
```

The outer preimage is now **exactly 32 bytes** — a Merkle root — and can never equal a
64-byte `headerHash' || bodyHash'`. This is a precise, targeted fix, gated on a consensus
version (`ExecutorVersionV1DoubleHashEntries = 3`, `protocol/enums_gen.go:125-126`;
`DoubleHashEntriesEnabled()`, `protocol/version.go:31-34`).

---

## 4. Claim 2 — the "64 is forbidden" invariant

Generalising §3.3:

> **Invariant.** For every attacker-influenced value *V* that is receipt-reachable and of the
> form `V = sha256(P)`, the preimage *P* must never be exactly 64 bytes with attacker-chosen
> content.

`T' = sha256(headerHash' || bodyHash')` has a 64-byte preimage by construction. So if no
reachable *V* admits a 64-byte attacker-chosen preimage, no *V* can equal any `T'`
(absent a real collision). Enumerating every reachable *V*:

| Value | Preimage *P* | \|P\| | Verdict |
|---|---|---|---|
| `DoubleHashDataEntry.Hash()` | Merkle root | exactly **32** | Safe — `data_entry.go:48` |
| `FactomDataEntry.Hash()` | `sha512(d) \|\| d` | **≥ 99** | Safe — see §4.1 |
| `AccumulateDataEntry.Hash()` | arbitrary | **any** | **Rejected** — §4.2 |
| `bodyHash` (generic body) | `marshal(body)` | any, **64 banned** | Guarded — §4.3 |
| `headerHash` | `marshal(header)` | any, 64 unreachable | Safe — §4.4 |
| Signature hash | `marshal(signature)` | tagged/structured | Safe — §4.5 |

### 4.1 Factom entries (still accepted in v2)

`entryIsAccepted` (`internal/core/execute/v2/chain/write_data.go:175-191`) accepts
`FactomDataEntryWrapper`, so its hash is load-bearing.

`ComputeFactomEntryHash` (`protocol/factom_data_entry.go:47-54`):

```go
func ComputeFactomEntryHash(data []byte) []byte {
	sum := sha512.Sum512(data)
	saltedSum := make([]byte, len(sum)+len(data))
	i := copy(saltedSum, sum[:])
	copy(saltedSum[i:], data)
	h := sha256.Sum256(saltedSum)
	return h[:]
}
```

`P = sha512(d) || d`, so `|P| = 64 + |d|`. And `FactomDataEntry.MarshalBinary`
(`factom_data_entry.go:72-101`) forces a version byte, a 32-byte `AccountId` (rejecting
zero at `:76-78`), and a 2-byte length — so `|d| ≥ 35` and `|P| ≥ 99`. **`|P| = 64` requires
`|d| = 0`, which is unreachable.** Factom entry hashes cannot equal a transaction hash.

### 4.2 `AccumulateDataEntry` is rejected

v2 (`write_data.go:181-183`) — unconditional:

```go
case *protocol.AccumulateDataEntry:
	// Accumulate entries are not accepted after v1-doubleHashEntries
	return false
```

v1 (`internal/core/execute/v1/chain/write_data.go:157-163`) — version-gated:

```go
case *protocol.AccumulateDataEntry:
	return !st.Globals.ExecutorVersion.DoubleHashEntriesEnabled()
case *protocol.DoubleHashDataEntry:
	return st.Globals.ExecutorVersion.DoubleHashEntriesEnabled()
```

Enforced via `validateDataEntry` → `entryIsAccepted`, `errors.BadRequest` otherwise.

### 4.3 The 64-byte body ban

`getBodyHash` (`transaction_hash.go:74-89`) returns `is64 = len(data) == 64`. A body whose
marshaling is 64 bytes gives `bodyHash = sha256(64 attacker-influenced bytes)` — the §3.3
primitive on the body leg. Bodies are **polymorphic and open-ended**: each payload type has
its own marshaling, and new ones keep arriving, so a per-type argument does not compose.
Hence a blanket length ban, enforced at three independent layers:

- `internal/core/execute/v2/block/msg_transaction.go:112-114` — `errors.BadRequest`
- `internal/node/abci/accumulator.go:592-600` — **fails the entire batch** (`resp.Code = 1`)
- `internal/core/execute/v1/block/validate.go:48-51` — gated on `DoubleHashEntriesEnabled()`

### 4.4 The header leg is structurally unreachable

`calcHash` computes `t.header64bytes` (`transaction_hash.go:61`) — both legs were analysed —
but `HeaderIs64Bytes()` is commented out (`:33-36`) and has zero readers, while
`BodyIs64Bytes()` (`:38-41`) has three.

This asymmetry is correct. `TransactionHeader` is a single fixed struct whose `Principal` is
`validate:"required"` (`protocol/types_gen.go:982`) and marshals as field 1
(`types_gen.go:12779-12781`):

```go
if !(v.Principal == nil) {
	writer.WriteUrl(1, v.Principal)
}
```

So a marshaled header **always** begins with a field tag, a varint length, and ASCII that must
parse as `acc://…`. For the header leg to bite, `headerHash' || bodyHash'` would have to *be*
that structure — but those are SHA-256 outputs: pseudorandom, and not chooseable (the attacker
picks *X'*, not its hashes). Grinding *X'* until two hashes happen to form a well-formed header
with a parseable URL is infeasible.

**Every transaction carries its destination, and the destination is what makes the header leg
unreachable.** The body has no comparable required structure, which is why one leg needs a
runtime check and the other does not.

> **Recommendation.** Move this reasoning into a comment at `transaction_hash.go:61`. As it
> stands, `header64bytes` reads as dead code and a commented-out accessor reads as an
> oversight; both invite "cleanup" that would erase the record of the analysis.

### 4.5 Signature hashes

`SignatureMessage.Hash()` → `Signature.Hash()` (`pkg/types/messaging/types.go:176-180`) →
`signatureHash` = `sha256(marshal(sig))` (`protocol/signature_utils.go:44-48`). The marshaled
form is union-type-prefixed and field-tagged, so it cannot be coerced to
`headerHash' || bodyHash'`. Matching a target would need a genuine collision across two
structured families.

The **signing** hash (`signature_utils.go:50-56`) *is* `sha256(sigMdHash || txnHash)` — a
64-byte preimage — but it is what the key signs; it is never a chain entry and never
receipt-reachable.

### 4.6 The destination is bound into `Start`

`Start` is the transaction hash; the transaction hash commits to the header; the header carries
`Principal`. So the destination rides inside the proven object and a receipt cannot be
repurposed to claim a different principal.

This is worth stating because `protocol.AnnotatedReceipt` — documented as "annotated with the
account and chain it originates from" (`protocol/general.yml:335-345`) — is **never verified**.
The only consumer-side reference is a nil check (`msg_synthetic.go:75`), and it *could not* be
verified: a `merkle.Receipt` carries no chain identifier. That annotation is decoration. The
binding that matters is cryptographic and lives in the preimage of `Start`.

**Claim 2 holds on current `main`**, resting on §4.2 (reject `AccumulateDataEntry`), §4.3 (ban
64-byte bodies), §4.1/§4.4/§4.5 (the remaining preimages cannot be 64 attacker-chosen bytes).

---

## 5. The anchoring closure

Because §2 makes "is `Start` a node?" the only question, the set of anchored chains matters —
and it is **maximal**, so no argument may rely on a chain being unanchored.

Selection is by **dirtiness**, not by name, type, or account:

- `(*Account).dirtyChains()` — `internal/database/model_gen.go:709-732` — unions `mainChain`,
  `scratchChain`, `signatureChain`, `rootChain`, `bptChain`, `anchorSequenceChain`,
  `majorBlockChain`, all `syntheticSequenceChain[*]`, all `anchorChain[*]`.
- `(*Chain2).dirtyChains()` — `internal/database/account_chains.go:69-79` — returns
  `c.index.dirtyChains()` **plus** `c.inner`. Index chains are therefore in the set.
- `enumerateModifiedChains` — `internal/core/execute/v2/block/block_end.go:914-956` —
  discards `ChainUpdates.Entries` (line 915) and rebuilds from `Batch.UpdatedAccounts()`.
  Dirtiness, not executor bookkeeping, decides anchoring.

Only two filters exist, both by URL, neither by type or account class:

- `block_end.go:926-931` — skip `acc://<partition>.acme/synthetic`
- `block_end.go:131-134` — skip `acc://<partition>.acme/ledger`

and `.../synthetic` `main` is re-added by hand (`block_end.go:191-198` → `anchorSynthChain`,
`:437-457`) when `block.State.Produced > 0`. The BPT has **no** exclusions, so even
ledger-only chains are committed via `hashChains`
(`internal/core/execute/internal/bpt_prod.go:63-81`).

**Index chains are anchored.** `addChainAnchor` (`internal/core/execute/v2/block/utils.go:106-135`):

```go
err = rootChain.AddEntry(accountChain.Anchor(), false)      // :112 — unconditional
...
shouldIndex, err := shouldIndexChain(chain.Account(), chain.Name(), chain.Type())
if err != nil || !shouldIndex { return 0, false, err }      // :119-121
```

`shouldIndexChain` returning `false` for `ChainTypeIndex` (`utils.go:58-76`) only suppresses an
index chain's *own* index chain; its anchor is already in the root chain by line 112. Index
leaves are marshaled `IndexEntry` structs padded to 32 bytes (`chain.go:139-158`) — low-entropy
and structured, so a SHA-256 transaction hash will not equal one — but any claim of the form
"that chain isn't anchored" is false and must not appear in a soundness argument.

The fold: chain entry → `chain.Anchor()` (MDRoot) → partition root chain → `RootChainAnchor`
(`internal/core/crosschain/anchoring.go:126-168`) → DN `anchor(bvn)-root` → DN root chain →
DN `RootChainAnchor` → every partition.

---

## 6. Write paths

Every non-test chain write, by whether the value can be attacker-chosen:

| Site | Value | Attacker-chosen? |
|---|---|---|
| `v2/block/transaction.go:581` | `delivery.Transaction.GetHash()` (executed) | content only, success-gated |
| `v2/chain/state_cache.go:195`, `state_operation.go:70` | `st.txHash` | content only, success-gated |
| `internal/database/signatures.go:98` | `msg.Hash()` | no — `sha256(marshal(sig))` |
| `v2/block/utils.go:112` | `accountChain.Anchor()` | computed |
| `v2/block/utils.go:95` | marshaled `IndexEntry` | no |
| `v2/block/block_end.go:96` | `block.State.PreviousStateHash` | computed |
| `v2/block/block_begin.go:174, 250` | `txn.GetHash()` / `anchorTxn.GetHash()` | no (locally built) |
| `v2/block/synthetic.go:201` | `seq.Hash()` | framing local |
| `v2/chain/partition_anchor.go:95,102`, `directory_anchor.go:75,82` | **raw** `body.RootChainAnchor`, `body.StateTreeAnchor` | **yes — ⅔ validator quorum** |
| `v2/block/msg_network_maintenance_op.go:42,131` | **raw** `msg.Cause.Hash()` | internal message only |

**Data entry hashes never reach a chain.** `addDataEntry.Execute`
(`internal/core/execute/v2/chain/state_operation.go:44-71`):

```go
err := indexing.Data(st.batch, op.url).Put(op.hash, st.txHash[:])   // :60 — entry hash → KV INDEX
...
return nil, st.State.ChainUpdates.AddChainEntry(st.batch, chain, st.txHash[:], 0, 0)  // :70 — txHash → CHAIN
```

`indexing.Data` is a plain KV index (`internal/database/model.yml:257-269`, `type: index`, no
`Chain2`), not a Merkle structure. v1 is identical
(`internal/core/execute/v1/chain/state_operation.go:59-65`). This is a *second, independent*
barrier behind §4.2 — but note it does **not** subsume it, because §3.2 shows `entry.Hash()`
is receipt-reachable through `hashWriteData` **without ever being a chain entry**.

The anchor-pool Root/BPT chains are the only raw-field writes. They are gated by
`BlockAnchor.check` requiring `signer.EntryByKeyHash(...)` against `core.AnchorSigner(...)`
(`msg_block_anchor.go:180-184`) and `txnIsReady` requiring
`len(sigs) >= ValidatorThreshold(partition)` (`:228-247`); anchor bodies are rejected outside a
`BlockAnchor` wrapper (`msg_transaction.go:217-225`). At ⅔ validator compromise the anchor
content is forgeable by definition, and this is outside the threat model.

---

## 7. Residuals — what is *not* proven

### 7.1 A receipt proves recording, not execution — and the BPT route proves *pending*

Claims 1 and 2 establish that `Start` is genuinely a node of an anchored structure. They do
**not** establish that *X* executed. Nothing in a receipt distinguishes "recorded on `main` via
`recordSuccessfulTransaction`" from "recorded anywhere else," because `Validate` has no chain
binding (§1.3) and §5 shows the closure is maximal.

The natural statement of the needed invariant is:

> For a user-ADI principal, a raw `Transaction.GetHash()` reaches the anchored closure only via
> `recordSuccessfulTransaction`.

It has known exceptions in other principal classes — `anchor-sequence` (`block_begin.go:250`)
and the synthetic ledger (`synthetic.go:201`) both write hashes for transactions with status
`errors.Remote` at send time. **But §3.1 shows the invariant is the wrong shape entirely: a
transaction hash need not be *written* anywhere to be receipt-reachable. It only needs to be a
child of some node.** The following is a live instance.

#### 7.1.1 Pending transactions are receipt-reachable from the BPT root

`hashState` composes each account's BPT leaf from four raw-inserted sub-hashers
(`internal/core/execute/internal/bpt_prod.go:29-37`):

```go
func (a *observedAccount) hashState() (hash.Hasher, error) {
	var err error
	var hasher hash.Hasher
	hashState(&err, &hasher, true, a.Main().Get)          // h0: simple hash of main state
	hashState(&err, &hasher, false, a.hashSecondaryState) // h1
	hashState(&err, &hasher, false, a.hashChains)         // h2: merkle hash of chains
	hashState(&err, &hasher, false, a.hashPending)        // h3: merkle hash of transactions
	return hasher, err
}
```

`hashState` → `hashValue` → `hasher.AddValue(v)` (`bpt_prod.go:143-157, 159-166`) →
`append(v.MerkleHash())` (`hasher.go:109-111`) — **raw**. And `hashPending`
(`bpt_prod.go:86-102`) adds, for every TxID on the account's pending list:

```go
for _, txid := range loadState(&err, false, a.Pending().Get) {
	...
	// If the transaction is not a V1 transaction, add its hash directly
	hasher.AddTxID(txid)     // :102
}
```

Composing with §3.2's `AddTxID` analysis gives an unbroken inheritance path, every fold being
`sha256(l||r)` over 32-byte operands:

```
txHash  --{accountHash, Right:true}-->  x = combineHashes(txHash, accountHash)   [AddTxID]
        --{pending-hasher folds}------>  h3 = pendingHasher.MerkleHash()
        --{account-hasher folds}------>  leafValue = accountHasher.MerkleHash()
        --{BPT branch folds}---------->  BPT root = StateTreeAnchor
```

The BPT closes the loop with the same shape: `leaf.getHash()` returns
`*(*[32]byte)(e.Value)` — **the raw value, unhashed** — in the non-`ArbitraryValues` case
(`pkg/database/bpt/node.go:96-101`), and `branch.getHash()` is `sha256.Sum256(b[:])` over a
`[64]byte` (`node.go:105-117`). The BPT has no leaf/internal separation either. The root is
`batch.BPT().GetRootHash()` → `StateTreeAnchor` (`internal/core/crosschain/anchoring.go:157,167`),
which is written to the receiving partition's `anchor(p)-bpt` chain
(`v2/chain/partition_anchor.go:102`, `directory_anchor.go:82`) — i.e. **a registered anchor.**

**Therefore: a transaction that is merely pending has an arithmetically valid receipt to a
registered anchor.** It has executed nowhere. It may be permanently unexecutable — e.g. a
multisig transaction whose threshold is never met. The cost is the credits to submit it.

#### 7.1.2 The receipt cannot distinguish pending from executed

Both routes terminate at the same account leaf. `MerkleHash` over `[h0,h1,h2,h3]` is
`combineHashes(combineHashes(h0,h1), combineHashes(h2,h3))`, so:

- executed route: main-chain entry → chain MDRoot → `h2` → `{h3, Right:true}` → `combine(h2,h3)` → leaf
- pending route: `txHash` → `x` → `h3` → `{h2, Right:false}` → `combine(h2,h3)` → leaf

The two paths differ **only** in the `Right` flag of one entry. A verifier that knew the exact
leaf layout and checked entry flags positionally could tell them apart — but `Receipt.Validate`
performs no such check (§1.3), carries no verified position metadata (§4.6), and **nothing in
the codebase implements it.** A foreign verifier following the natural pattern — *validate the
receipt, confirm the anchor is one this network registered* — accepts the pending transaction
as proven.

#### 7.1.3 In-protocol consensus is unaffected

Both in-protocol trust sites resolve the anchor against the **root** anchor chain, never the
BPT chain:

- `msg_synthetic.go:174-177` — `AnchorChain(protocol.Directory).Root().IndexOf(syn.Proof.Receipt.Anchor)`
- `create_token_account.go:179-183` — `AnchorChain(protocol.Directory).Root().Get()` → `HeightOf(...)`

`StateTreeAnchor` lands on `.BPT()`, not `.Root()`, so a BPT-rooted receipt fails both. This is
load-bearing and appears to be incidental to the lookup's purpose rather than a stated control.
**It should be commented as a security boundary at both sites**, because "also check `.BPT()`"
is exactly the kind of change that reads as a completeness fix.

#### 7.1.4 Consequence for the ticket

There is no status-record escape hatch: the `Transaction` entity is top-level and `private: true`
(`internal/database/model.yml:298-300`), with `Status` as a private attribute (`:309-314`), so the
BPT indexes accounts only; and `recordSuccessfulTransaction` calls `Pending().Remove(...)` before
recording, so a delivered transaction's status is in the BPT for zero blocks.

So the BPT route is the only route available to a foreign verifier — and §7.1.1 shows it commits
pending and executed transactions into the same leaf, indistinguishably under `Validate`. **A
receipt to a registered anchor is not evidence of execution and cannot be made into evidence of
execution without a new mechanism.** Options, in increasing order of honesty:

1. Scope the claim to root-chain anchors only, and have the verifier check the anchor against
   `AnchorChain(...).Root()` — mirroring §7.1.3. This reduces the problem to §7.2's invariant,
   which is contingent but currently true.
2. Add explicit positional binding — salt receipts with the index at the anchor point, as
   `receipt_list.go:71-75` already proposes for the adjacent gap.
3. Publish a signed, anchored delivery record with its own domain-separated hash, so execution
   has a positive witness rather than being inferred from membership.

### 7.2 `RecordHistory` is unguarded

`internal/database/signatures.go:87-101`:

```go
// RecordHistory adds the message to the signature chain and history.
func (c *AccountTransaction) RecordHistory(msg messaging.Message) error {
	...
	h := msg.Hash()
	err = c.parent.SignatureChain().Inner().AddEntry(h[:], false)
	...
}
```

It takes the **interface** and writes `msg.Hash()` to an anchored chain (`ChainTypeTransaction`,
§5) **before** the transaction executes. And `pkg/types/messaging/types.go:170-174`:

```go
func (m *TransactionMessage) Hash() [32]byte {
	// A transaction message must contain nothing besides the transaction, so
	// this is safe
	return *(*[32]byte)(m.Transaction.GetHash())
}
```

A `TransactionMessage`'s hash **is** the transaction hash, verbatim. If a `TransactionMessage`
ever reaches `RecordHistory`, a raw `T` enters the closure for a transaction that never executed
— and §4.6's destination binding does not help, because that `T` carries the *correct*
principal. The transaction is real and well-formed; it simply did not run.

All six callers today pass non-transaction messages — `sig_user.go:441`, `sig_common.go:221`,
`sig_authority.go:134`, `msg_signature_request.go:139`, `msg_credit_payment.go:96`,
`msg_block_anchor.go:90`. The §7.1 invariant therefore holds **by the accident of which concrete
types six call sites happen to pass**. There is no type check, no comment, and no test.

> **Recommendation.** Add a type assertion rejecting `*messaging.TransactionMessage` in
> `RecordHistory`, plus a test. This converts the invariant from contingent to structural at
> negligible cost.

### 7.3 Pre-`v1-doubleHashEntries` history is not covered

`ExecutorVersionV1DoubleHashEntries = 3` (`protocol/enums_gen.go:126`). Under
`ExecutorVersionV1` (1) and `ExecutorVersionV1SignatureAnchoring` (2),
`entryIsAccepted` returned `true` for `AccumulateDataEntry` (v1 `write_data.go:157-163`).
During that era the §3.3 construction was **live**.

Consequently: **any receipt whose anchor is rooted in pre-upgrade history is not covered by
Claim 2.** A `DoubleHashDataEntry` cannot be forged today, but a receipt chaining to an anchor
from that era may have a fabricated `Start`. Any verifier accepting historical anchors — light
clients, `exp/checkpoint`, archival proofs — must treat pre-v3 anchors as unsound for this
purpose, or the claim must be scoped to anchors at or after the upgrade height.

This is not hypothetical repository trivia: the fix was consensus-gated precisely because the
old behaviour is preserved in history.

### 7.4 `ReceiptList.MerkleState` is caller-supplied

`pkg/database/merkle/receipt_list.go:27-45` validates a caller-supplied `r.MerkleState`
un-checked; only the **last** element is bound to the anchor via `r.Receipt`. The in-repo
comment at `:71-75` concedes the adjacent gap: *"the ReceiptList does not necessarily prove the
indices of the elements in the Merkle Tree. This could be solved by salting Receipts with the
index of the hash at the anchor point."* No consensus path consumes `ReceiptList`; if one is
added, this needs revisiting.

### 7.5 Known deprecated construction

`BadSyntheticMessage.Hash()` (`pkg/types/messaging/types.go:182-188`) is
`sha256(msgHash || anchorHash)` — a 64-byte preimage — with the in-repo comment *"This is unsafe
and buggy, which is why this type is deprecated."* Listed for completeness; the type is
deprecated.

---

## 8. Conclusion

Claims 1 and 2 hold on current `main`. Claim 1 is unconditional, reducing to SHA-256 preimage
resistance. Claim 2 is **conditional on two deliberate controls** — rejecting
`AccumulateDataEntry` and banning 64-byte bodies — which together enforce: *no
attacker-influenced, receipt-reachable value has an attacker-chosen 64-byte preimage.*

The absence of leaf/internal domain separation (§1.1, §1.2) is what makes those controls
necessary. They are correct and sufficient today, but they are point fixes over a structural
gap, and each new raw insertion (§3.2) or new transaction body type re-opens the question.

**But soundness was the easy half.** Claims 1 and 2 say a receipt's `Start` is genuinely a node
of an anchored structure. The property people actually want — *this transaction executed* —
does not follow, and §7.1 shows it is false today by construction, not merely unproven: the
inheritance lemma (§3.1) means a transaction hash never has to be *written* anywhere to be
receipt-reachable, and `hashPending` makes every **pending** transaction reachable from a
registered `StateTreeAnchor` for the price of the credits to submit it. Consensus is spared
only because both in-protocol trust sites happen to resolve anchors against `.Root()` rather
than `.BPT()` (§7.1.3).

The through-line is that **§3.3 and §7.1 are the same structural pattern in two places** — a raw
insertion of a value with a 64-byte preimage (`entry.Hash()` at `transaction_hash.go:104`;
`combine(txHash, accountHash)` at `hasher.go:104`). They differ in kind, and the distinction
matters: §3.3 was a **vulnerability**, found and plugged with a consensus-gated fix. §7.1 is
**not a vulnerability** — `hashPending` is correct, nothing consumes BPT-rooted receipts as
proof of execution, and there is nothing to exploit. It is a constraint on what a receipt can
be made to mean, and it is only load-bearing if someone tries to build the proof in §7.1.4.

Ranked recommendations:

1. **Do not ship a foreign-verifier execution proof on the BPT route** (§7.1.4). It proves
   pending, not executed. If a proof is needed now, scope it to root-chain anchors and check
   the anchor against `AnchorChain(...).Root()` explicitly.
2. **Comment the `.Root()`-not-`.BPT()` boundary** at `msg_synthetic.go:174-177` and
   `create_token_account.go:179-183` (§7.1.3). It is load-bearing and currently silent.
3. **Guard `RecordHistory`** against `*messaging.TransactionMessage` (§7.2) — cheap, converts a
   contingent invariant to a structural one.
4. **Comment `header64bytes`** at `transaction_hash.go:61` with the §4.4 reasoning, so the
   analysis is not mistaken for dead code.
5. **Scope any historical claim** to anchors at or after `ExecutorVersionV1DoubleHashEntries`
   (§7.3).

> **Structural recommendation.** Tag leaves with a domain separator distinguishing "transaction
> hash" from "entry hash" from "anchor" — e.g. `sha256(0x00 || leaf)` for leaves and
> `sha256(0x01 || l || r)` for internal nodes, in both `pkg/database/merkle` and
> `pkg/database/bpt`. This retires §4.2, §4.3, §4.4 **and §7.1** as load-bearing, and makes the
> entire class impossible rather than contingent. It is a consensus break needing a version gate
> and a snapshot boundary, on the model of `ExecutorVersionV1DoubleHashEntries`.
