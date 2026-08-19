# Synthetic-healing docker test (#4064)

> **Running a long soak?** Do it on the lab server, not a laptop — 13
> containers will saturate one for as long as the run lasts. See
> [DEPLOY-REMOTE.md](DEPLOY-REMOTE.md) for the SSH procedure and the four
> traps that have bitten (Go invisible to non-interactive SSH, `pgrep -f`
> self-matching, `-dirty` manifests, mixed UTC/local timestamps).
>
> **Before quoting any result**, read [soak/MONITOR-AUDIT.md](soak/MONITOR-AUDIT.md):
> five dashboard panels read metrics no node exports and are permanently `0`.

Reproduces a **wedged synthetic stream** on a real multi-container libp2p network
and proves that **receiver-pull healing** recovers it.

## What it does

1. `init` generates a 3-node network (each node runs the Directory plus one BVN)
   and turns on `enable-synthetic-healing` for every node.
2. Every node runs with `ACC_DEBUG_DROP_SYNTHETIC=*:1`, a debug hook that
   **silently drops the first cross-partition synthetic the node emits**.
   Production has no automatic synthetic retry, so that single drop wedges every
   later synthetic behind it — exactly the mainnet incident this issue is about.
3. `driver` funds from the genesis faucet and sends ACME across a partition
   boundary five times. The first send's synthetic is dropped; the rest pile up
   pending.
4. With healing enabled, the destination partition pulls the missing message
   from the source partition's sequencer over real libp2p and re-submits it; the
   normal `MessageIsReady` cascade then drains the tail.
5. The driver asserts the recipient eventually receives **all five** deposits,
   and `run-test.sh` confirms `accumulate_crosschain_heals_total` incremented —
   proving the recovery went through the receiver-pull healer, not another path.

## Run

```sh
./run-test.sh            # builds the image if needed, runs, tears down
KEEP=1 ./run-test.sh     # leave the network up afterwards for inspection
```

Expected tail:

```
PASS: synthetic stream wedged and healed; all deposits delivered
-- drop (wedge formed) --
acc-bvn2 | ... DEBUG dropping synthetic envelope destination=acc://bvn-BVN1.acme
-- heal counter (receiver-pull fired) --
bvn1:
    accumulate_crosschain_heals_total{partition="BVN1",source="BVN2",type="synthetic"} 1
RESULT: PASS (stream wedged and healed via receiver-pull)
```

## Notes

- `ACC_DEBUG_DROP_SYNTHETIC=<partition|*>:<count>` is a debug/test-only hook
  (a no-op unless set); it is the deterministic way to reproduce a lost
  synthetic in a real network.
- The recovery takes ~10–50 s because production uses a 10 s jitter/back-off
  window (`syntheticHealWindow`) so that, with multiple validators, only one
  or two actually issue the pull.
- Healing is enabled per node via `enable-synthetic-healing = true` in the
  `coreValidator` configuration (default off).
