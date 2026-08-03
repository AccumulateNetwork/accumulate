# Bug: DN→BVN network-definition updates never reach BVN partitions (DAG-BFT, v2-kourou)

**Status:** open — needs protocol fix. Root cause **corrected 2026-07-22** (see
"Correction" below); the original DN-side-queueing hypothesis was wrong.
**Area:** DAG-BFT cross-partition synthetic delivery (family of #4054/#4056/#4057)
**Found:** 2026-07-15, while building the #4058 churn-soak P3 (on-chain follower→validator promotion)
**Severity:** high — on-chain validator promotion/demotion (and any `dn.acme/network` change) applies on the Directory but never propagates to BVN committees, so a promoted validator can never participate on its BVN.

## Summary

When the `NetworkDefinition` on `dn.acme/network` is updated (e.g. adding an active
validator), the change executes on the **Directory**: the DN's active globals advance
(`Network.Version` 0→1) and the **DN committee updates** on every node. But the change
is **never applied on the BVN partitions**: the BVN's network version stays 0 and the
**BVN committee never updates**, so a newly-promoted validator's BVN headers are rejected
as `unknown validator` / `Header epoch mismatch`.

The DN **does** produce the synthetic `messaging.NetworkUpdate` toward every BVN. It is
never **delivered**, and the BVN can never discover that it is missing.

## Correction to the original analysis (2026-07-22)

The first version of this document concluded that `b.State.NetworkUpdate` was empty and
that "the DN never emits the synthetic NetworkUpdate", based on a log grep that found no
emission lines. **That inference was wrong** — there are no log lines on that path to
find. Direct inspection of chain state disproves it:

```
query acc://dn.acme/synthetic  →  produced=1 toward each of bvn-BVN1/2/3
query acc://dn.acme/synthetic main chain, index 1, expanded:
  type: sequenced
  message: { type: networkUpdate, accounts: [{ name: network, body: writeData ... }] }
  source: acc://dn.acme   destination: acc://bvn-BVN1.acme   number: 1
```

The `writeData` payload contains the promoted key. So the whole DN-side chain is healthy:
`processNetworkAccountUpdates` → `st.State.NetworkUpdate` → `MergeTransaction` →
`b.State.NetworkUpdate` → `produceBlockMessages` → `produceSynthetic`. Nothing in
`network_accounts.go` needs fixing.

## Actual root cause: the first synthetic is dropped and the gap is undetectable

Two facts combine:

1. **Dispatch of the produced synthetic is one-shot** and this message was dropped. The
   BVN never received it.
2. **The BVN cannot detect the loss.** Healing is gated on `healNeeded()`
   (`block_end.go:475-491`), which only reports a gap when a `Pending` window contains a
   **nil** entry — i.e. when a *later* message arrived and revealed a hole. On BVN1 the
   synthetic ledger has **no `Sequence` entries at all**:

   ```
   query acc://bvn-BVN1.acme/synthetic → account exists, pending.total = 0, no sequence[]
   ```

   With an empty `Sequence`, `healNeeded()` iterates nothing and returns false. Healing
   (`requestMissingSyntheticTransactions`) is never launched. The BVN has no evidence that
   DN→BVN synthetic #1 ever existed.

Because a network-definition change is a **rare, one-off event**, no subsequent synthetic
ever arrives to expose the hole. The message is lost permanently.

This is precisely why **anchors are unaffected**: anchors are produced every block, so a
drop is revealed by the very next anchor and healed. Measured on the same net:

| Ledger | DN side | BVN1 side |
|---|---|---|
| `anchors` (dn.acme ↔ bvn-BVN1) | received=1 delivered=1 | received=5 delivered=5 |
| `synthetic` (dn.acme → bvn-BVN1) | **produced=1** | **no sequence entries** |

Note also `buildDirectoryAnchor` (`block_end.go:924-926`) deliberately leaves
`anchor.Updates` empty on Vandenberg+, so on v2-kourou the *reliable, self-healing* anchor
transport for network-account updates is disabled and the **only** transport is the
one-shot synthetic described above.

## Reproduction (12-node net — NOT topology-specific)

Originally seen only on the 5-node 1-active-validator promote net. **Re-confirmed
2026-07-22 on the full 12-node `test/docker/docker-compose.yml` net** (3 BVNs × 4
validators, all dual DN+BVN, executor v2-kourou), so the earlier "may be specific to the
minimal bootstrap topology" hypothesis is ruled out.

Steps:

1. `docker compose -f test/docker/docker-compose.yml up -d`; wait for all four partitions
   to advance.
2. Build `test/cmd/promote-validator` (now supports `-pubkey <hex>` to promote a key with
   no corresponding node — enough to probe propagation without needing P2 follower mode).
3. Promote a fresh random key on **Directory only** (keeps every BVN committee untouched,
   so BVN liveness margin is unaffected):

   ```
   promote-validator -server http://localhost:26660 -pubkey $(openssl rand -hex 32) \
     -operators <12 node accumulate.toml paths> -partitions Directory
   ```

Observed:

| Signal | Directory | BVN1/2/3 |
|--------|-----------|----------|
| `network.version` | 0 → **1** | **0** (unchanged) |
| validator count | 12 → **13** | **12** (unchanged) |
| `Validator set changed, updating committee` | ✅ every node, same height (9501), epoch 0→1 | ❌ never |

The DN-side committee mechanism is deterministic and correct — every node logged the
update at the identical height.

**Known harness gap:** a *second* promotion submitted against the same net was accepted by
`Submit` but never reached `dn.acme/network`'s main chain (no pending entry, no chain
entry, version stayed 1). Not yet diagnosed; `promote-validator` does not print the
envelope txid, which is the first thing to add before chasing it.

## Fix vectors

1. **Make the receiver able to discover a missing first message.** The general fix for the
   whole class: have the DN→BVN anchor carry the sender's produced-synthetic count so the
   BVN can compare against its own `received` and pull anything it never saw. Today a
   receiver can only detect an *interior* gap, never a missing prefix. This also closes
   the same hole for any other rare one-off synthetic.
2. **Restore the reliable transport for network-account updates.** Populate
   `DirectoryAnchor.Updates` on Vandenberg+ as well (application is idempotent), so the
   every-block anchor stream carries the change in addition to the synthetic. Narrower
   than (1) but directly unblocks P3.
3. Make synthetic dispatch retry rather than one-shot (necessary but not sufficient —
   without (1) a drop that outlives the retries is still undetectable).

## Impact

- On-chain promotion works for the Directory committee but not the BVN committee. A dual
  DN+BVN node promoted on-chain participates in DN consensus but is a non-member on its BVN.
- Blocks the #4058 churn-soak **P3** acceptance, and therefore the week-long soak
  (`docs/plans/dagbft-week-churn-soak.md` says do not start until P1–P3 pass).

## Ruled out

- **Discovery / `noPeer`.** A one-time startup race; a deployed bootstrap DHT-server node
  restores `query:*` discovery. The BVN gap persists regardless.
- **Major-block gating.** A major block occurred; the BVN committee still did not update.
- **Minimal-topology artifact.** Reproduces identically on the 12-node net.
- **DN-side queueing / `b.State.NetworkUpdate` being empty.** Disproved — the synthetic is
  on the DN's synthetic chain (see Correction).

## Key files

- `internal/core/execute/v2/block/block_end.go:475-491` (`healNeeded` — the gap-detection
  gate that cannot see a missing prefix), `:506+` (`requestMissingSyntheticTransactions`),
  `:924-926` (anchor `Updates` disabled on Vandenberg+), `:1015-1030` (synthetic
  `NetworkUpdate` production — confirmed working)
- `internal/core/execute/v2/block/network_accounts.go:115-146` (DN queues — confirmed working)
- `internal/core/execute/v2/block/msg_network_update.go:94-118` (BVN applies — never reached)
- `pkg/consensus/adapter/executor_bridge.go:100-151`, `internal/node/dagbft/service.go:604-647`
  (per-partition committee update — verified correct)

## Reproducer artifacts

`test/cmd/promote-validator/` (with `-pubkey`), and on branch `p3-onchain-promotion`
`test/docker/{docker-compose.promote.yml,promote-network.yml,promote-test.sh}`.
