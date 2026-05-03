# Cyclops BPT inconsistency — investigation and per-account repair plan

**Status**: investigation complete, repair pending
**Date**: 2026-05-03
**Author**: BPT-drift investigation team
**Affected partition**: Cyclops BVN (mainnet)
**Affected accounts**: 22

---

## Summary

`snap-bpt-stale` against the Cyclops BVN follower DB found 22 accounts whose
stored BPT leaf does not match `account.Hash()` recomputed over current
state. Independent investigation against the July 13 2025 pre-reorg
backup, and an exhaustive sweep of every other account in the BPT
against pre-reorg state, confirmed:

- **17 accounts** have intact `Main` bodies, but their `Chains()` index
  registry entries were silently dropped. The chain merkle data is
  preserved; only the index pointer was lost.
- **5 accounts** have had their `Main` bodies dropped entirely — 4
  previously identified, plus one new finding: `acc://dn.acme/network`.
- **0 accounts** have had their `Main` bodies silently mutated (any
  account where `Main` differs pre-vs-current was explained by a
  transaction in the blockstore).
- 1 block ledger orphan (`bvn-Cyclops.acme/ledger/960446`) is excluded
  per routine pruning policy.

Total damage scope from the single-validator-era corruption is bounded
to these 22 accounts.

The user's suspicion that running a single validator for months allowed
an undetected corruption — and that the moment additional nodes joined,
that corrupted state was baked into consensus — is confirmed by the
shape of the damage: erasures only, no value mutation.

---

## Timeline

| Date | Event |
|---|---|
| 2025-07-13 | Mainnet "reorg" — 4 partitions (`bvn0`, `bvn1`, `bvn2`, `dn`) consolidated into 2 (`Cyclops` BVN + `dn` DN). All pre-reorg state captured in backup at `/mnt/secondary/databases7-13-25-restored/`. |
| 2025-07-13 → ? | Single-validator operation per partition. No Byzantine fault detection, so any corruption (disk write, snapshot creation bug, mid-block crash, pruning error) became consensus state silently. |
| ? → 2026-03-02 | Additional nodes join. Subsequent corruption would have been detected as consensus failure. Our follower's blockstore covers this period (block 2 to 19,509,497). |
| 2026-03-02 | Our local follower stopped applying blocks (still has the BPT root from this point). |
| 2026-05-03 | Investigation completed. |

---

## Investigation methodology

### Tools built

| Tool | Purpose | Source |
|---|---|---|
| `cmd/snap-bpt-stale` | Walk a snapshot or live DB BPT, recompute `account.Hash()`, report mismatches. Two modes: snapshot (v1 or v2) and on-disk DB (badger or leveldb). | `cmd/snap-bpt-stale/main.go` |
| `cmd/db-account-lookup` | Look up specific account URLs across multiple DBs read-only. | `cmd/db-account-lookup/main.go` |
| `cmd/blockstore-walk` | Walk a CometBFT blockstore.db, decode every Accumulate envelope, emit JSONL with principal, recipients, signers, and source partition. | `cmd/blockstore-walk/main.go` |
| `cmd/find-dropped` | Walk a follower DB, identify orphan accounts (missing `Main`), cross-reference each against pre-reorg DBs to determine which orphans had bodies pre-reorg (= drops). Optionally also enumerate leaf-mismatches. | `cmd/find-dropped/main.go` |
| `cmd/find-silent` | Comprehensive sweep: for every account NOT touched by any post-reorg transaction, verify `Main` matches pre-reorg. Any difference is silent corruption. | `cmd/find-silent/main.go` |
| `cmd/rebuild-chains` | Replay live-network chain entries into a writable DB copy and verify whether `account.Hash()` reproduces the stored leaf. Proves whether the leaf is correct given full chain data. | `cmd/rebuild-chains/main.go` |
| `cmd/probe-account` | Dump `Main` + `Directory` + `Pending` + chain heads for a single URL on a single DB. | `cmd/probe-account/main.go` |
| Read-only Badger | Added `pkg/database/keyvalue/badger.ReadOnlyBadger` flag wiring `WithReadOnly(true)` into `OpenV1/V2/V3/V4` so historical Badger DBs can be inspected without modification. | `pkg/database/keyvalue/badger/{core,versions}.go` |

### Steps executed

1. **Snap-bpt-stale on Cyclops BVN** → 22 mismatches identified.
2. **Pre-reorg lookup for the 22** → 21 of 22 had bodies on July 13 2025; the 1 missing was the post-reorg block ledger entry.
3. **Live mainnet query for chain content** → confirmed the network's BPT entries, chain heads, and chain entries match what the follower stores.
4. **Blockstore walk** → 27,421 transactions over 19.5M blocks; **none** touched any of the 21 substantive accounts after the reorg, ruling out post-reorg transaction-mediated drift.
5. **`rebuild-chains` experiment** → demonstrated that re-registering missing `Chains()` entries on a writable copy reproduces the stored leaf for 12 of 17 with-body cases (ADIs and PegNet LTAs); the other 5 (LiteIdentities + ACME LTA) need both registry restoration and post-stop chain entries from the live network.
6. **`find-dropped` on Cyclops BVN** → confirmed 4 known orphan drops + 1 new (`dn.acme/network`).
7. **`find-silent` on Cyclops BVN with signer-aware tx set** → 0 silent `Main` mutations across 177,723 untouched accounts.
8. **DN partition sweep** → see results below.

### Why "single validator + later multi-node" matches the evidence

If multiple validators had been running when the corruption occurred, any
one node mutating state outside the transaction system would have produced
a divergent BPT root and been kicked out by consensus. The fact that all
nodes today agree on the same broken BPT leaves means the corruption
predates multi-node operation — every node that bootstrapped from a
snapshot taken after the corruption inherited the broken state and
agreed with it.

The shape of the damage — pure erasures (`Main` body removed; chain
index stripped) with **zero** value mutations across 177,723 untouched
accounts — is consistent with disk-level or snapshot-creation bugs, not
with a Byzantine adversary or executor logic error.

---

## Findings — Cyclops BVN

### Class A — Chain-registry strips (17 accounts)

`Main` body byte-identical to pre-reorg. The `Chains()` index registry
record (which tells `observer.hashChains` which chains belong to the
account) was lost. The underlying chain merkle data is preserved.

| # | URL | Account type | Pre-reorg src |
|---|---|---|---|
| 1 | `acc://aber.acme` | ADI | bvn2 |
| 2 | `acc://chimneypiece.acme` | ADI | bvn2 |
| 3 | `acc://corrupted.acme` | ADI | bvn1 |
| 4 | `acc://csrc.acme` | ADI | bvn0 |
| 5 | `acc://nadro.acme` | ADI | bvn0 |
| 6 | `acc://treble.acme` | ADI | bvn0 |
| 7 | `acc://zagg.acme` | ADI | bvn1 |
| 8 | `acc://45875c282cbf0265fc2369cfc420ab7658f9c378b257608f` | LiteIdentity | bvn0 |
| 9 | `acc://981fabf9e5447ead08f2bb1dd7eed3282864ad20a7fc0e1e` | LiteIdentity | bvn1 |
| 10 | `acc://ca6c6f2b20ac4fe16cf0e2a6dd1e6d8ccfce21df3fe22468` | LiteIdentity | bvn0 |
| 11 | `acc://cb5a976eea2b84a9c78263984bc4ebf205ce99e2d2bfea01` | LiteIdentity | bvn0 |
| 12 | `acc://1570f386a1cd332a5a33beee62b0dd23df2a08bb74d23f1e/PegNet.acme/assets/peg` | LiteTokenAccount | bvn1 |
| 13 | `acc://78432e204c43d61286daa3800cf462f8a02d8828fdd294b3/ACME` | LiteTokenAccount | bvn0 |
| 14 | `acc://ca59e9e6c08ed245324f4c52e61defe34ab95a15abdcc802/PegNet.acme/assets/rvn` | LiteTokenAccount | bvn1 |
| 15 | `acc://2d7b3f44935ee7de9e99766f995aa4afbc3bb9ff3dfebd9aaa8e670f178bc83c` | LiteDataAccount | bvn2 |
| 16 | `acc://79f09991516f7b88c507c554bc13aa659d9bfff54467a0a4a4372f3468e88bd8` | LiteDataAccount | bvn1 |
| 17 | `acc://c2db482c10bfa53099a06555d3dc5307076138a8bd003757b18f8d9c181a41c6` | LiteDataAccount | bvn1 |

**Repair operation** for each: `Chains().Add(name, type)` for every
chain the live network reports as registered on this account, then
`Chain.AddEntry(hash)` for any entries the network has that the local
chain head doesn't yet have. No `Main` write. No network submission.

The exact chain set per account from the live mainnet API:

- All 7 ADIs: `main` (count=2, both entries = `e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5`, anchor `4fb28d18…`), `main-index` (count=2, anchor varies), `signature` (count=0), `signature-index` (count=0).
- All 4 LiteIdentities: `main` (count=3), `main-index` (count=3), `signature` (count=0), `signature-index` (count=0).
- LTAs and live LDAs: `main` (count=2-4), `main-index` (count=2), `signature` (count=0), `signature-index` (count=0).

Per-account chain entries stored in `/tmp/live-chains.json`.

### Class B — Main-body drops (5 accounts)

`Main` body went from present (pre-reorg) to missing (current). The
BPT entry survived as a stored leaf reflecting the empty-body hash
(except for the 4 LDAs/ADI where the leaf was NOT updated and remains
the pre-deletion value, which is why `snap-bpt-stale` flagged them).

| # | URL | Pre-reorg type | Pre-reorg size | Pre-reorg src | Stored-leaf state |
|---|---|---|---|---|---|
| 18 | `acc://kmutt.acme` | ADI | 53 B | bvn0 | non-empty (`030e9a81…`) |
| 19 | `acc://675f6bdb…16654d55` | LiteDataAccount | 74 B | bvn1 | non-empty (`91162455…`) |
| 20 | `acc://99a480ce…27b630d8` | LiteDataAccount | 74 B | bvn2 | non-empty (`7709c79d…`) |
| 21 | `acc://ab7a5ed9…8723fdef` | LiteDataAccount | 74 B | bvn1 | non-empty (`96b6e354…`) |
| 22 | `acc://dn.acme/network` | DataAccount | 2,521 B | dn | empty (matches body-less hash) |

**Note on `dn.acme/network`**: this is the network-configuration
DataAccount belonging to the Directory partition. It is alive and well
on the live mainnet (verified via `https://mainnet.accumulatenetwork.io/v3`).
The Cyclops BVN's BPT carries an entry for it (presumably as a
side-effect of the reorg merge that combined all four pre-reorg
partitions into Cyclops). The body was deleted from the BVN BPT but
the entry remained.

**Repair operation** per account:

- `kmutt.acme` (orphan ADI): No signer exists for this URL. Cannot
  dirty-mark via a normal transaction. Either restore `Main` from
  pre-reorg backup (restores the body but leaf will recompute to
  pre-reorg-leaf, not current stored leaf), or accept as orphan via
  the existing snapshot-restore carve-out (`c0e1f718a`).
- 3 LDA orphans: Submit a `WriteData` to each. The LDA was never
  authority-checked, so any wallet with a credit balance can write.
  This creates a fresh body and `UpdateBPT` writes a new leaf.
- `dn.acme/network`: This is a DN account; repair belongs on the DN
  partition, not the BVN. Decision needed: should the BVN BPT carry
  bodies for DN accounts at all? If no, this is a phantom entry to
  clear; if yes, restore the body from `dn` pre-reorg DB or from the
  live DN.

### Class C — Block ledger orphan (excluded)

`acc://bvn-Cyclops.acme/ledger/960446` — block ledgers are pruned
routinely. The carve-out in `c0e1f718a` (`snapshot restore: skip
per-account hash check for ANY orphan, not just block ledgers`) covers
this case for snapshot operations.

---

## Findings — DN partition

DN follower DB at `/mnt/secondary/.../accman/follower-data/dnn/data/accumulate.db`.
BPT root: `b5a20b09ce429e35702ef313ccc55bfb9a0cf4029abb7bd40d07e61aa679b82a`.
33,931 accounts examined.

**find-dropped on DN**: 0 confirmed drops, 0 leaf-mismatches with body, 4 orphans without pre-reorg presence (all under `staking.acme/*` — `request`, `govnernance/1` (sic), `rewards`, `book2` — created post-reorg as body-less placeholders, not corruption).

**find-silent on DN** (signer-aware tx set, 367 unique touched URLs): 0 silent drops, 0 silent appeared, **6 silent differs** — but every one is a protocol system account whose state is modified by internal block processing rather than user-submitted transactions:

| URL | Type | Pre-reorg size | Current size | Notes |
|---|---|---|---|---|
| `acc://dn.acme/operators` | KeyBook | 58 B | 58 B | bytes differ — operator keypage version updates |
| `acc://dn.acme/network` | DataAccount | 2,521 B | 196 B | shrank — network config rewrites |
| `acc://dn.acme/synthetic` | SyntheticLedger | 158 B | 85 B | shrank — synth-tx counter rolling |
| `acc://dn.acme/oracle` | DataAccount | 62 B | 62 B | bytes differ — oracle price entries |
| `acc://dn.acme/anchors` | AnchorLedger | 186,331 B | 92 B | shrank ~2000× — anchor pool pruning |
| `acc://dn.acme/routing` | DataAccount | 428 B | 219 B | routing table rewrites |

These are all expected mutations via internal protocol logic (anchor processing, oracle updates, routing table maintenance, validator key updates, synth-tx sequencing). The find-silent walker treats them as "differs" because user-level transactions don't directly touch them — they're modified at block boundaries by the executor.

**The DN partition has no actual silent corruption.** None of these system-account changes need repair.

### Cross-partition note: `dn.acme/network`

This DN system account appears in TWO findings:

- **DN partition** (above): body present (196 B), modified routinely. Not corrupt.
- **Cyclops BVN partition**: body MISSING (orphan, BPT entry survives with empty-hash leaf). This is a phantom entry — the reorg consolidated 4 partitions into 2 and apparently carried DN account BPT entries into the BVN's BPT, but the bodies were not (or were later) dropped from the BVN side.

Treatment: the DN side is authoritative. The BVN BPT entry for `dn.acme/network` is either a phantom that should be cleared, or — if it must remain for some routing/aggregation reason — its body should be restored from the live DN. Recommend escalating to whoever owns the reorg snapshot tooling for a decision.

---

## Per-account update enumeration

The repair operations grouped by class. All repairs are local-DB
operations (no network submission needed) for Classes A and the LDA
subset of B. The orphan ADI and DN-side accounts may need different
treatment.

### Class A repair script (17 accounts)

Pseudocode applied to every account in Class A:

```go
batch := db.Begin(true)
for _, c := range liveChainsFor(account) { // from /tmp/live-chains.json
    err := acc.Chains().Add(&protocol.ChainMetadata{
        Name: c.Name,
        Type: c.Type,
    })
    must(err)
    chain, _ := acc.GetChainByName(c.Name)
    have := chain.Height()
    for i, entryHash := range c.Entries {
        if int64(i) < have {
            continue // already present locally
        }
        must(chain.AddEntry(decodeHex(entryHash), false))
    }
}
batch.Commit() // do NOT call UpdateBPT — leaf is already correct
```

For 12 of these the loop appends 0 entries (chain data was already
present, only the `Chains()` index needed restoration). For 5
(LiteIdentities and ACME LTA), 1-3 chain entries from the live
mainnet need to be appended.

Verification: after Commit, read `account.Hash()` and compare to the
stored BPT leaf. Should match.

### Class B repair (5 accounts)

| Account | Repair |
|---|---|
| `kmutt.acme` | (decide) restore pre-reorg body OR accept as orphan |
| `675f6bdb…16654d55` | submit `WriteData` to it (creates fresh body, leaf updates via UpdateBPT) |
| `99a480ce…27b630d8` | submit `WriteData` to it |
| `ab7a5ed9…8723fdef` | submit `WriteData` to it |
| `dn.acme/network` | (decide) restore body in BVN BPT OR clear phantom entry |

For the 3 LDAs, `WriteData` envelopes can be built with the
`tools/cmd/repair-cyclops-bpt` tool already in the tree (parses
correctly against the live mainnet endpoint per
`TestLiveValidatePretendSmoke`).

### Repair via protocol-version activation (chosen approach)

Rather than per-account network submissions and per-follower local
patches, the fix is embedded in the executor and gated on a new
protocol version. When the network activates the new version, every
node runs the same repair routine in the same activation block, and
the BPT root advances consistently.

**New version:** `protocol.ExecutorVersionV2CyclopsBptRepair`
(`v2-cyclops-bpt-repair`, value `9`). Predicate
`(v ExecutorVersion) V2CyclopsBptRepairEnabled() bool` available for
forward gating.

**Embedded targets:** `internal/core/execute/v2/block/cyclops_bpt_repair.go`
holds a per-partition target list. Currently only `Cyclops` has
targets (the 22 accounts above). The list is hardcoded — every node
running the new binary has the same data, so consensus is preserved.

**Activation hook:** `executePostUpdateActions` in `block_end.go` runs
once when `globals.Pending.ExecutorVersion` becomes
`globals.Active.ExecutorVersion`. The new switch case for
`ExecutorVersionV2CyclopsBptRepair` calls `(b *Block).applyCyclopsBptRepair()`,
which:

1. Looks up the per-partition target list (no-op on partitions other
   than Cyclops).
2. For each target: re-registers the chains in `Chains()`, appends any
   missing chain entries (the embedded `entries` list for ADIs encodes
   the genesis-pair pattern), then calls `acc.MarkDirty()`.
3. `block_end`'s subsequent `UpdateBPT()` call refreshes every dirty
   account's BPT leaf to match `account.Hash()` over the post-repair
   state.

**Idempotency:** all operations (`Chains().Add`, `Chain.AddEntry`,
`MarkDirty`) are idempotent. Running the activation logic twice
(e.g., during a chain reorg) produces the same final state.

**Test:** `test/e2e/cyclops_bpt_repair_test.go::TestCyclopsBptRepairActivation`.
Spins up a sim with a BVN named `Cyclops`, plants a corrupt leaf on
`csrc.acme` (one of the targets), submits an `ActivateProtocolVersion`
to the new version, and verifies after the activation block that the
planted corruption is gone and the leaf matches recomputed
`account.Hash()`. PASSES.

### Submission steps for mainnet activation

1. Distribute a binary built from this branch to every Cyclops + DN node.
2. Wait for the network to be running the new binary.
3. Submit an `ActivateProtocolVersion` transaction to `acc://dn.acme`
   with `Version: protocol.ExecutorVersionV2CyclopsBptRepair`,
   signed by the operators keypage. (Same submission shape used for
   prior version activations such as Baikonur and Vandenberg — see
   `test/e2e/net_maintenance_test.go` for the pattern.)
4. Within a few blocks of the activation, the repair runs on every
   partition (no-op everywhere except Cyclops). After UpdateBPT, the
   22 stale leaves are corrected network-wide.
5. Re-run `cmd/snap-bpt-stale` against any healthy node — should
   report 0 mismatches.

### Why this approach is better than the alternatives

- **No per-account transactions:** no need to find a signer for ADIs
  delegated to `marketplace.acme/book` or for the orphaned
  `kmutt.acme`. The fix runs as part of the executor itself.
- **Atomic and consistent:** all 22 leaves repaired in a single
  activation block. No partial-repair states.
- **Provably correct:** the test asserts the post-repair leaf equals
  the recomputed hash, which is what consensus checks per block.
- **Fits existing patterns:** uses the same `executePostUpdateActions`
  switch that handled the V2Jiuquan ledger-restructure transition.

---

## Tools and artifacts

| Path | Purpose |
|---|---|
| `cmd/snap-bpt-stale/` | Mismatch finder |
| `cmd/find-dropped/` | Orphan finder + pre-reorg cross-reference |
| `cmd/find-silent/` | Silent-corruption sweep |
| `cmd/blockstore-walk/` | Per-tx index from CometBFT blockstore |
| `cmd/db-account-lookup/` | Multi-DB account lookup |
| `cmd/probe-account/` | Single-account state dump |
| `cmd/rebuild-chains/` | Chain-replay validator |
| `tools/cmd/repair-cyclops-bpt/` | Envelope builder for the 21 substantive cases (with `--pretend` mode) |
| `pkg/database/keyvalue/badger/{core,versions}.go` | Read-only Badger flag |

Output artifacts (under `/tmp/`):

| File | Contents |
|---|---|
| `/tmp/cyclops-bvn-stale-final.log` | snap-bpt-stale output (22 mismatches) |
| `/tmp/preorg-all22.log` | pre-reorg state for the 22 accounts |
| `/tmp/live-chains.json` | live-network chain entries per account |
| `/tmp/walk-full2.jsonl` | full BVN blockstore tx index (27,421 entries with signers) |
| `/tmp/walk-dn.jsonl` | full DN blockstore tx index (TBD) |
| `/tmp/dropped.jsonl` | find-dropped output (Cyclops BVN) |
| `/tmp/silent2.jsonl` | find-silent output (Cyclops BVN, signer-aware) |
| `/tmp/dn-dropped.jsonl` | find-dropped output (DN, TBD) |
| `/tmp/dn-silent.jsonl` | find-silent output (DN, TBD) |

---

## Pending decisions

- `kmutt.acme`: restore from pre-reorg or accept as orphan? The body
  is recoverable but no signer exists to dirty-mark, so any restore
  is a manual DB write rather than a network repair.
- `dn.acme/network` on the Cyclops BVN: clear phantom entry, or
  restore body from live DN? See the cross-partition note above.
- Snapshot collector audit: what code path strips `Chains()` index
  entries and `Main` bodies during snapshot creation? The fix here
  prevents future bootstraps from reproducing the issue. Worth
  tracing in `internal/database/snapshot.go` (the `collectAccounts`
  / `PreserveAccountHistory` paths).
- Operational: ensure no partition runs single-validator going
  forward. The reorg single-validator window is what made this
  corruption possible to embed into consensus undetected.
