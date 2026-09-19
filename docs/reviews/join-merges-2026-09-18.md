# Review: the fifteen merges of 2026-09-18 (`260445c5f` → `bff83e840`)

Scope: everything merged into `dagbft-integration` today — #4290–#4296 (E11 join),
#4304, #4317/#4318, block-ledger-last, #4295 (served body), #4344/#4345, #4348.
`go build ./...` is clean; `./internal/node/join/... ./internal/core/bootstrap/...`
are green. The findings below are all in production code, and none of them is
caught by a test in the tree.

## 1. The buffer→block map is valid only on the FIRST `StartCollecting`, and two callers break it

`collect.go:StartCollecting` sets `s.collectFrom = s.lastBlockIndex` **and** empties
the buffer. `performHandoff` then reads `skip = q - collectFrom` and produces
`buffer[skip:]` as blocks `q+1, q+2, …`. That arithmetic is only true if the node
collected *every* group from `collectFrom+1` on.

Two callers violate it:

- `cmd/accumulated/run/dagbft.go:547` calls `s.service.StartCollecting()` **before**
  `s.service.Start()`. `s.lastBlockIndex` is set by `initializeGenesis`
  (`internal/node/dagbft/service.go:202`), which runs *inside* `Start`. So the first
  call sets `collectFrom = 0`. It is then repaired only by accident: `join.Run`
  calls `StartCollecting()` again as the first statement of its loop, which sets
  `collectFrom = R` **and discards whatever was collected between `Start` and the
  goroutine being scheduled**. Those groups were blocks `R+1…`; the buffer now starts
  at `R+k+1` while `collectFrom` still says `R`, and every produced block after the
  handoff is off by `k`.
- The buffer-overrun restart has the same shape and a much wider window: `join.Run`
  loops back, `StartCollecting` empties a buffer of up to 8192 groups and resets
  `collectFrom` to the node's own last executed block, which has not moved (a joining
  node executes nothing). The network has. The next group collected is *not* block
  `R+1`.

`performHandoff` guards only `q < collectFrom` and `skip > len(buffer)`. Neither can
see a hole. The failure is silent and is exactly the divergence the join exists to
prevent.

Fix shape: `collectFrom` must be set once, at the point the node starts receiving
committed groups, and a restart of the join must either keep the buffer or refuse to
hand off until it has re-derived the mapping from something other than "how many
groups have I seen".

## 2. `settleBatch` discards everything it settled in earlier rounds

`internal/node/join/state.go`, `settleBatch`. `pulled` counts *this call's* settles.
A held batch that settles five accounts in round 1 returns `done=false` and stays
open. If the last round settles none:

```go
if pulled == 0 {
    h.batch.Discard()      // the five accounts from round 1 go with it
    return 0, refused, true
}
```

The five were counted as pulled and logged as pulled, are **not** added to `refused`,
and their state is gone. Only the every-8th-round BPT diff brings them back. Given
the code's own note that "five settle rounds in six end here", this is the common
path, not the corner.

## 3. Held accounts are asked for again every round

Nothing filters the round's ask list against `s.held`. `changedAccounts` names
`<partition>/ledger` and `<partition>/synthetic` unconditionally and `enumerate.Stale`
names any account whose leaf differs — and a held account's leaf differs by
construction, because it has not been committed. So an account held for up to
`maxSettleRounds` rounds is re-fetched on each of them, into a different batch, at a
different block. Consequences: duplicate network work; two `Pending`s for one account
whose commits race (the older block can land last and rewind the leaf); and the
`pull.MaxHeld` budget of 1024 filling with duplicates, after which `pull.Fetch`
returns `NotReady` and everything else in the round is refused.

## 4. `APIPeers.Validators` does not drop this node — so a node alone concludes "nobody has staging"

`internal/node/join/peers.go:36` returns `FindService(sequencer:<partition>)`
unfiltered. `QueryPeers` has a `Self` field and drops it, with a comment explaining
that `p2p.DialNetwork` installs a self-discoverer unconditionally; `APIPeers` has no
such field.

`takeStaging` counts every peer it asks in `asked`, including this node. This node
refuses its own staging (it is BOOTING) or answers `Block == 0`, and either way is
counted. So `found == 1` when the only "validator" found is the node itself, the
`found == 0` guard added for #4296 does not fire, `Run` returns `NoPeerHasStaging`,
and the daemon calls `executeFromOwnState`. A node that restarts while it cannot see
its partition executes from its own state with empty staging — the #4290 divergence,
through the door #4296 was opened to close.

## 5. A successful join is logged as a failure

`cmd/accumulated/run/dagbft.go`, the `go func` around `join.Run`:

```go
case outcome == join.NoPeerHasStaging: …
default:
    slog.Error("The join did not complete; this node is not executing", …, "error", err)
```

`default` is `outcome == Joined, err == nil`. The one path that means success logs an
error, with `error=<nil>`, and says the node is not executing when it is. The block is
a verbatim copy of the `err != nil` case.

## 6. The block-ledger walk is dead after the first 128 blocks of a join

`localBlock()` returns `s.executed`, frozen for the life of the join — correctly, per
#4295. `changedAccounts` sets `s.wide = true` and returns nothing when
`peer - r > MaxLedgerSpan` (128). A joining node's `r` never moves and the peer's
block advances one per second, so after ~two minutes every round is `wide` and the
join runs on the full BPT page diff alone — the expensive backstop the ledger walk
exists to avoid (~211 MB/scan by the earlier measurement), every round, forever.
`TestJoinKeepsUpFromTheBlockLedgerAlone` exercises only the under-128 case.

The span the walk wants is "since the block my state is now", which is not `R` once
the pull has replaced most of the store.

## 7. `takeStaging` cannot tell "every peer refused" from "every peer is empty"

A peer that errors, a peer that is itself joining (NotReady, #4295) and a peer whose
staging is genuinely empty all land in the same bucket, and after `DefaultRounds`(10)
× `DefaultRetry`(2 s) ≈ 20 s the answer is `NoPeerHasStaging` → execute from own
state. Under chaos, several nodes joining at once is normal, and 20 s of mutual
refusal is not evidence that the network restarted as a whole. `NoPeerHasStaging`
should require that every validator *answered* with an empty stage.

## 8. The block ledger's content changed with no version gate

`anchorSynthChains` now emits one `BlockEntry` per appended synthetic chain entry,
each with its own index, instead of one per chain; and the record is written after it
rather than before. The record is hashed onto the block-ledger chain, which is in the
ledger account's hash, which is in the BPT. Both the Jiuquan and the pre-Jiuquan
branch changed. That is a state change on the DI line with no `ExecutorVersion` bump:
fine for fresh installs, fatal for any node running a build from before today against
one from after. If the fresh-install rule still holds, say so in the commit; if the
soak ever rolls binaries one node at a time, it does not hold.

## 9. `queryMinorBlock` changed behaviour for existing `Expand: false` callers

`names := entryRange.Expand != nil && !*entryRange.Expand` now returns bare
(account, chain, index) triples. Previously `Expand: false` still went through
`loadBlockEntry`. The REST surface parses `entry_expand` from the query string
(`pkg/api/v3/rest/query.go:133`), so this is an external API change, not only an
internal one.

## Still true, not addressed by today's test work

`test/simulator/partition.go:CompleteJoin` still copies the peer's store wholesale
(`memory.Export`/`Import`), so every simulator test of a "join" still exercises no
pull, no verify and no root match. Today's new e2e tests (`join_verify`,
`join_block_ledger`, `join_named_peer`) do drive `join.NewState` and the real pull,
which is the right direction — but the restart-at-R → take staging → pull (R,Q] →
match → hand off sequence is still exercised by no test.

## What is right

- #4295's shape is correct: nothing that hashes is synthesised on the way out,
  `Sighted` travels beside the body, and every consumer (`debug sequence`,
  `heal_common`, API v2) was moved to `SightedAccount()`.
- `pull` writes through a nested batch and verifies before committing; `Pending.past`
  discards its batch where the decision is made rather than carrying a "trust me" flag.
- `nodestate.Serving` is nil-safe on both the pointer and the interface, and the
  daemon substitutes `Always{}` rather than handing a typed-nil into an interface.
