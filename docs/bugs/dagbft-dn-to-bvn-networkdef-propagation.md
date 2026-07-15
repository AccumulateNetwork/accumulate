# Bug: DN→BVN network-definition updates never reach BVN partitions (DAG-BFT, v2-kourou)

**Status:** open — needs protocol fix
**Area:** DAG-BFT cross-partition globals propagation (family of #4054/#4056/#4057)
**Found:** 2026-07-15, while building the #4058 churn-soak P3 (on-chain follower→validator promotion)
**Severity:** high — on-chain validator promotion/demotion (and any `dn.acme/network` change) applies on the Directory but never propagates to BVN committees, so a promoted validator can never participate on its BVN.

## Summary

When the `NetworkDefinition` on `dn.acme/network` is updated (e.g. adding an active
validator), the change executes on the **Directory**: the DN's active globals advance
(`Network.Version` 0→1) and the **DN committee updates** on every node. But the change
is **never propagated to the BVN partition**: the BVN's network version stays 0 and the
**BVN committee never updates**, so a newly-promoted validator's BVN headers are rejected
as `unknown validator` / `Header epoch mismatch`.

This is **not** a discovery, anchor-delivery, or major-block-timing problem (all ruled
out empirically — see below). The DN simply does not emit the synthetic network update
to the BVN.

## Impact

- On-chain promotion works for the Directory committee but not the BVN committee. A dual
  DN+BVN node promoted on-chain participates in DN consensus but is a non-member on its BVN.
- Blocks the #4058 churn-soak **P3** acceptance (add a validator, watch every partition's
  committee bump). The promotion *mechanism* is otherwise proven correct on the DN side.

## Reproduction

Harness (branch `p3-onchain-promotion`):

- `test/docker/docker-compose.promote.yml` + `test/docker/promote-network.yml` — 5 dual
  DN+BVN nodes (1 active validator + 4 followers) on **executor v2-kourou**, plus a
  `accp-bootstrap` DHT-server node so service discovery works.
- `test/cmd/promote-validator` — reads a node's own validator key, pulls the live
  `NetworkDefinition` via v3 `NetworkStatus`, `AddValidator(pubkey, partition, active=true)`,
  bumps `Version`, and submits the `WriteData` to `dn.acme/network` signed by
  `dn.acme/operators/1`.
- `test/docker/promote-test.sh` — brings the net up, promotes `bvn1-2` on-chain, checks the
  committee updates.

Steps: bring the net up, wait for BVN blocks, run `promote-validator` for
`-partitions Directory,BVN1`. The submit executes (`network version 0 → 1`).

## Observed behavior (empirical)

After the promotion executes:

| Signal | Directory | BVN1 |
|--------|-----------|------|
| network version (`NetworkStatus`) | **1** | **0** |
| `Validator set changed, updating committee` | ✅ all nodes | ❌ never, any node |
| promoted node's headers | accepted | rejected (`unknown validator` / `Header epoch mismatch currentEpoch=1 headerEpoch=0`) |

Pinpoint run (major blocks forced to 1/min): a major block occurred (`majorHeight=1`)
and the BVN committee **still** did not update. A grep of the DN node for any
`WillChangeGlobals`/`NetworkUpdate`/globals-toward-BVN emission after the promotion is
**empty**. The BVN keeps executing DN *block anchors* but never receives/applies a
network-*definition* update.

## Root-cause analysis

On **v2-kourou** (`ExecutorVersionV2Kourou > ExecutorVersionV2Vandenberg`, `protocol/version.go:14,53,69`),
`V2VandenbergEnabled()` is true, so the intended DN→BVN transport for a network-definition
change is the **synthetic `messaging.NetworkUpdate`**, not anchor `Updates`:

- `internal/core/execute/v2/block/block_end.go:1015-1030` — at block end, if
  `V2VandenbergEnabled() && len(b.State.NetworkUpdate) > 0`, the DN produces a
  `messaging.NetworkUpdate{Accounts: b.State.NetworkUpdate}` to each BVN.
- `internal/core/execute/v2/block/block_end.go:924-925` — the anchor `Updates` path is
  taken **only when `!V2VandenbergEnabled()`**, i.e. it is *inactive* on v2-kourou.

The BVN application side is present and correct:
- `internal/core/execute/v2/block/msg_network_update.go:94-118` → `processOld` re-executes
  the WriteData against `bvnX.acme/network` → `ParseNetwork` → BVN `globals.Pending` moves →
  BVN `block_end.go:277` fires `WillChangeGlobals` → per-partition committee update
  (`pkg/consensus/adapter/executor_bridge.go:100-151`, `internal/node/dagbft/service.go:604-647`).

The per-partition committee wiring is verified good (the BVN daemon runs its own
bridge/service on the BVN bus). **So the break is upstream of application: the DN never
produces the synthetic `NetworkUpdate`.**

**Prime suspect:** `b.State.NetworkUpdate` is empty at `block_end.go:1017`, i.e. executing
the promotion `WriteData` on the DN does not queue the change for BVN distribution. The DN
queues it at `internal/core/execute/v2/block/network_accounts.go:125-146` (only the
Directory does this, gated at `:129`). If that gate isn't met (or the WriteData path to
`dn.acme/network` doesn't reach it), `b.State.NetworkUpdate` stays empty → no synthetic
`NetworkUpdate` is produced → the BVN never learns of the change, even though the DN's own
`globals.Pending`/committee advanced.

The next diagnostic step for a fixer: instrument or inspect `b.State.NetworkUpdate` (and
the queueing gate at `network_accounts.go:129`) while a `dn.acme/network` WriteData executes
on the DN, and confirm whether it is populated. If empty, the fault is in the DN-side
queueing; if populated, the fault is in synthetic production/delivery at
`block_end.go:1015-1030` or dispatch.

## Ruled out

- **Discovery / `noPeer`.** Was a one-time startup race; a deployed `accp-bootstrap`
  DHT-server node (this branch) restores `query:*` discovery. BVN gap persists regardless.
- **The bootstrap seq-1 anchor drop.** Real (conductor dispatch is one-shot, `dagbft.go:220`)
  but self-heals with discovery working — the BVN anchor ledger reaches `delivered=2`. Base
  anchor delivery is healthy; the missing thing is specifically the *network-definition* update.
- **Major-block gating.** A major block occurred; the BVN committee still did not update.

## Fix vectors

1. Ensure a `dn.acme/network` WriteData populates `b.State.NetworkUpdate` on the Directory
   (verify the gate at `network_accounts.go:129` for the v2-kourou/Vandenberg path), so the
   synthetic `NetworkUpdate` at `block_end.go:1015` is produced to every BVN.
2. Confirm the synthetic `NetworkUpdate` is dispatched to and delivered on the BVN, and that
   `msg_network_update.go` applies it (bumps `bvnX.acme/network` version → BVN
   `WillChangeGlobals` → committee update).

## Key files

- `internal/core/execute/v2/block/block_end.go:277-288` (WillChangeGlobals),
  `:924-925` (anchor Updates, pre-Vandenberg only), `:1015-1030` (synthetic NetworkUpdate)
- `internal/core/execute/v2/block/network_accounts.go:89-91,115-146` (DN queues, BVN blocks local)
- `internal/core/execute/v2/block/msg_network_update.go:94-118` (BVN applies)
- `pkg/consensus/adapter/executor_bridge.go:100-151`, `internal/node/dagbft/service.go:604-647`
  (per-partition committee update — verified correct)
- `protocol/version.go:14,52-53,69` (version ordering; Kourou ≥ Vandenberg)

## Reproducer artifacts

`p3-onchain-promotion` branch: `test/cmd/promote-validator/`,
`test/docker/{docker-compose.promote.yml,promote-network.yml,promote-test.sh}`.
