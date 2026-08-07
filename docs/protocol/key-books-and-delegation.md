# Key books, delegation, and side keys

Who may change what on a key page, and which changes a delegate can make on its
own authority. Everything here is drawn from the v2 executor; source references
are given so the rules can be re-checked when the executor changes.

## The model

A **key book** owns an ordered list of **key pages**. A page holds **entries**
(`KeySpec`), a signature threshold, and optional per-page restrictions.

An entry carries a public key hash, a delegate URL, **or both**:

| entry | satisfied by |
|---|---|
| key hash only | a signature from that key |
| delegate only | an authority signature from the delegate book |
| key hash **and** delegate | either — see [Side keys](#side-keys) |

A delegate must be a **book**, never a page. `verifyIsNotPage`
(`internal/core/execute/v2/chain/update_key_page.go`) rejects
`example.acme/book/1` where `example.acme/book` is required. Self-delegation is
also rejected: a delegate that is a parent of the principal page fails with
"self-delegation is not allowed".

## Authorities are AND, pages are OR

An account's authority set lists key books. **Every** authority must vote accept
before a transaction executes — `userTransactionIsReady` ends with "the
transaction is only ready if all authorities have voted"
(`internal/core/execute/v2/block/transaction.go`). Adding a second authority to
an account therefore makes both books required for *every* transaction on it;
it does not create an alternative signer, and it cannot be scoped to particular
transaction types.

Within one book the opposite holds: a book's vote is produced by **any single
page** reaching its own threshold. Pages are alternatives, not a quorum.

The only per-page transaction restriction is a blacklist covering exactly two
operations — `UpdateKeyPage` and `UpdateAccountAuth` (`AllowedTransactionBit` in
`protocol/enums.yml`), enforced at signature acceptance
(`internal/core/execute/v2/block/sig_common.go`). There is no mechanism to
restrict a page to, say, token transfers.

## Page priority

Lower page index means higher priority. For `UpdateKeyPage`, a signer belonging
to the **same book** as the principal page must not be lower priority:

```go
// Lower indices are higher priority
if signerPageIdx > principalPageIdx {
    return false, errors.Unauthorized.WithFormat(
        "signer %v is lower priority than the principal %v", ...)
}
```
(`internal/core/execute/v2/chain/update_key_page.go`)

So page 1 may rewrite page 2, page 2 may never touch page 1, and a page **may
modify itself** (equal index is permitted) — except that a page cannot change
its own allowed-operations. Signers from a *different* book skip this check
entirely and fall through to normal authorization.

This is what makes a two-page book useful: put operational keys on page 2 and
retain page 1 as the rotation authority. A compromise of page 2 cannot escalate
to page 1, and page 1 can always rotate page 2 out. To also prevent the
compromised key from disrupting its own page, blacklist `UpdateKeyPage` on
page 2 — a page cannot lift its own blacklist.

## What a delegate may do alone

`UpdateKey` is a distinct transaction from `UpdateKeyPage`, with its own
authorization rules, and it is the mechanism by which an entry's holder
maintains that entry.

`UpdateKey.AuthorityIsAccepted` admits both the principal page and **any
delegate of it**:

```go
// Delegates are allowed to sign
_, _, ok := page.EntryByDelegate(sig.Authority)
if ok { return false, nil }
```

`TransactionIsReady` then completes as soon as the initiator's own authority
signature arrives. **It never consults the page's accept threshold.** On a
4-of-7 page, a delegate changing its own entry needs only its own signature.
(`internal/core/execute/v2/chain/update_key.go`)

What it changes is narrow. The body carries only `NewKeyHash`, and Execute calls
`updateKey(..., &KeySpecParams{KeyHash: body.NewKeyHash}, /* preserveDelegate */ true)`.
Because the new params carry no delegate and `preserveDelegate` is true, the
entry's `Delegate` is left untouched and only `PublicKeyHash` is written:

```go
entry.PublicKeyHash = new.KeyHash
if new.Delegate != nil || !preserveDelegate {
    entry.Delegate = new.Delegate
}
```

`UpdateKey` also refuses to modify a validator key book, and deliberately does
not bump the page version or reset `LastUsedOn`. The version matters in practice:
signatures specify the signer version, so after an `UpdateKey` the page is still
at its previous version and subsequent signatures must use that number, not an
incremented one.

### Side keys

Applying `UpdateKey` to a **delegate-only** entry gives that entry a direct key
hash *in addition to* its delegation. The holder can subsequently sign either
directly with that key or through the delegated book. This is the supported way
to add a signing key to a slot you hold by delegation, and it needs no
cooperation from the page's other entries.

**The two signing paths bill different accounts.** The signature fee is charged
to the signer named on the signature itself (`SignatureContext.getSigner`, debited
in `internal/core/execute/v2/block/sig_user.go`). Signing **directly** with a side
key charges the page that holds the entry; signing **through the delegation**
charges the delegate's own page. So attaching a side key does not just change how
you sign, it moves who pays: the page you are signing *for* funds direct
signatures, while the delegate funds delegated ones. Keep credits on whichever
page you actually intend to use, and note that a delegated page with no credits
cannot sign even though the entry is valid.

**Containing a sidecar key with an unreachable home page.** A key can be made
usable *only* as a sidecar: park it on its own page alongside a second entry
nobody can sign for, and set that page's threshold to 2. The page can never
reach its threshold, so the key cannot sign for anything its own book
authorizes — but where the same key's hash is attached to an entry on another
book's page it is matched by key hash, and its home page's threshold is
irrelevant. Verified by `TestDelegate_InertHomePageButUsableSidecar`.

Two caveats. Higher-priority pages of the same book are unaffected, so the book
itself retains full authority via page 1 — the containment applies to the key,
not to the book. And for the unusable second entry, prefer adding a **random
32-byte key hash** over a generated keypair: the page stores only the hash, so
a hash with no known preimage provably has no corresponding private key, whereas
a discarded keypair is only as dead as your confidence that the private half was
destroyed.

**One entry contributes one signature, even with a side key.**
`compareSignatureSetEntries` (`internal/database/signatures.go`) keys the active
signature set on (KeyIndex, delegation path) rather than key index alone, which
raised the question of whether one entry holding both a key hash and a delegate
could contribute twice toward a single threshold — once signed directly, once
through the delegation. **It cannot.** `TestDelegate_SideKeyDoesNotDoubleCount`
signs both ways from a single entry against a threshold of 2 and the transaction
does not execute; a signature from a second entry then completes it, confirming
the threshold was genuinely reachable. A side key widens *how* you can sign, not
*how much* your entry counts.

## What requires page authorization

Everything else is `UpdateKeyPage` against the page, subject to page priority
and the page's threshold:

- adding or removing entries
- **repointing an entry's delegate** to a different book
- changing the accept or reject threshold
- changing allowed operations

`UpdateKey` cannot repoint a delegate — its body has no delegate field. Moving a
slot from `example.acme/book` to `example.acme/otherBook` is an
`UpdateKeyOperation` inside `UpdateKeyPage`, which is threshold-governed.

Adding or repointing a delegate additionally requires **the incoming delegate's
consent**: `updateKeyPage_getNewOwners` collects `Entry.Delegate` /
`NewEntry.Delegate` into the transaction's additional authorities
(`internal/core/execute/v2/chain/update_key_page.go`), and those must vote
accept alongside normal authorization. A book cannot be conscripted into a
delegation it has not agreed to.

## Verification

Every claim above is asserted by `test/e2e/txn_delegate_authority_test.go`:

| test | claim |
|---|---|
| `TestDelegate_RotatesOwnEntryBelowThreshold` | a delegate rotates its own entry alone on a 4-of-7 page |
| `TestDelegate_UpdateKeyAddsSideKeyPreservingDelegate` | `UpdateKey` adds a key while preserving the delegate; the side key then signs directly; the page version is not bumped |
| `TestDelegate_CannotRepointOwnDelegateAlone` | repointing a delegate does not execute below the page threshold |
| `TestDelegate_SideKeyDoesNotDoubleCount` | one entry contributes one signature, with a control proving the threshold is reachable |
| `TestDelegate_SideKeyVsDelegatePaysDifferentCredits` | a direct signature is paid by the page holding the entry; a delegated one by the delegate's page |
| `TestDelegate_InertHomePageButUsableSidecar` | a key on an unreachable-threshold page cannot sign through its own book, yet still signs as a sidecar elsewhere |

## Summary

| change | who can authorize it |
|---|---|
| rotate the key hash on your own entry | the entry holder alone (`UpdateKey`) |
| add a side key to your delegate-only entry | the entry holder alone (`UpdateKey`) |
| repoint your entry's delegate | `UpdateKeyPage`: page threshold, or a higher-priority page — **plus** the incoming delegate's consent |
| add or remove an entry | `UpdateKeyPage`: page threshold, or a higher-priority page |
| change a page's threshold | `UpdateKeyPage`: page threshold, or a higher-priority page |
| change a page's allowed operations | a higher-priority page only |
| change an account's authority set | `UpdateAccountAuth`: existing authorities **and** each added authority |

## Worked example

A page `staking.acme/book/2` with `acceptThreshold: 4` and seven delegate
entries, one of which is `example.acme/book`:

- `example.acme` **can**, alone, rotate the key on its entry, or attach a side
  key to it, via `UpdateKey`. The 4-of-7 threshold does not apply.
- `example.acme` **cannot**, alone, repoint its entry to `example.acme/stakingBook`.
  That is `UpdateKeyPage`, requiring four of the seven entries to sign — or a
  single signature from the higher-priority `staking.acme/book/1` — and
  `example.acme/stakingBook` must also consent.
