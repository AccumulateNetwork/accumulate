# Synthetic-healing docker test (#4064, #4067)

Reproduces a **wedged synthetic stream** on a real multi-container libp2p network
and proves that **receiver-pull healing** recovers it — across **every**
transaction type that produces a cross-partition synthetic.

## What it does

1. `init` generates a 3-node network (each node runs the Directory plus one BVN)
   and turns on `enable-synthetic-healing` for every node.
2. Every node runs with `ACC_DEBUG_DROP_SYNTHETIC=$DROP` (default `*:1`), a
   debug hook that **silently drops cross-partition synthetics**. Production has
   no automatic synthetic retry, so a single drop wedges every later synthetic
   behind it — exactly the mainnet incident this issue is about.
3. `driver` funds from the genesis faucet and drives the **mixed** workload —
   see [Workloads](#workloads) — so every heal path is exercised, not just token
   deposits.
4. With healing enabled, the destination partition pulls the missing message
   from the source partition's sequencer over real libp2p and re-submits it; the
   normal `MessageIsReady` cascade then drains the tail.
5. The driver asserts every followed transaction and every message it produced
   was delivered, **and** that every expected synthetic type was produced;
   `run-test.sh` confirms the heal counters incremented — proving the recovery
   went through the receiver-pull healer, not another path.

## Workloads

`driver` takes `-workload`:

- **`mixed` (default)** — a weighted mix of every user transaction type that
  produces a cross-partition synthetic. It bootstraps its own accounts (ADIs,
  key pages, token accounts, one lite data account per foreign partition) and
  then loops the mix, so a run exercises every heal path rather than just the
  token-deposit one:

  | workload step | synthetic produced |
  | --- | --- |
  | lite → lite transfer | `syntheticDepositTokens` |
  | ADI token account transfer | `syntheticDepositTokens` |
  | remote ADI creation | `syntheticCreateIdentity` |
  | credit purchase | `syntheticDepositCredits` |
  | token burn | `syntheticBurnTokens` (to ACME on the DN) |
  | write data to a remote lite data account | `syntheticWriteData` |
  | cross-partition authority write | `signatureRequest`, `creditPayment` |

  The last row is the `MessageForTransaction` heal path: the principal lives on
  one partition and its only authority lives on another, so initiating the
  transaction sends both messages across the boundary.

- **`transfers`** — lite → lite ACME sends only, the original #4064 repro.

The verdict walks the produced-message tree of every transaction it followed and
reports produced/delivered **per synthetic type**. A type that was never
produced fails the run, so the driver cannot silently stop covering a heal path
and still report success.

```
== cross-partition message coverage ==
   creditPayment                    produced=6      delivered=6
   signatureRequest                 produced=6      delivered=6
   syntheticBurnTokens              produced=2      delivered=2
   ...
```

`test/e2e/synth_mixed_test.go` (`TestMixedWorkloadSyntheticCoverage`) is the
simulator counterpart: it runs one transaction of each type and asserts every
expected synthetic type actually crosses a partition boundary.

## Run

```sh
./run-test.sh                      # builds the image if needed, runs, tears down
KEEP=1 ./run-test.sh               # leave the network up afterwards for inspection
DROP='*:%16' ./run-test.sh         # recurring drops instead of one per node
WORKLOAD=transfers ./run-test.sh   # the original lite -> lite-only repro
COUNT=128 TIMEOUT=30m ./run-test.sh
```

Expected tail:

```
== cross-partition message coverage ==
   creditPayment                    produced=9      delivered=9
   signature                        produced=9      delivered=9
   signatureRequest                 produced=18     delivered=18
   syntheticBurnTokens              produced=3      delivered=3
   syntheticCreateIdentity          produced=3      delivered=3
   syntheticDepositCredits          produced=9      delivered=9
   syntheticDepositTokens           produced=28     delivered=28
   syntheticWriteData               produced=12     delivered=12
   writeData                        produced=9      delivered=9

PASS: every tracked transaction was delivered and every expected synthetic type was produced
-- drop (wedge formed) --
acc-bvn2 | ... DEBUG dropping synthetic envelope destination=acc://bvn-BVN1.acme
-- heal counters via ConsensusStatus, every validator (receiver-pull fired) --
  acc-bvn1:
    BVN1 "syntheticHeals":2
RESULT: PASS (stream wedged and healed via receiver-pull)
```

`signature` and `writeData` are in the table because they genuinely cross a
partition boundary too: a cross-auth transaction forwards its signature to the
principal's partition, and the transaction itself is executed there. They are
healed like any other cross-partition message. A `!` in the left margin marks a
type with undelivered messages, or one that was expected but never produced —
either fails the run.

The soak (`soak/soak.sh`) drives the same mixed workload at a fixed TPS for
hours, with chaos restarts, using `*:%97+3` so drops recur throughout.

## Notes

- `ACC_DEBUG_DROP_SYNTHETIC=<partition|*>:<count>` is a debug/test-only hook
  (a no-op unless set); it is the deterministic way to reproduce a lost
  synthetic in a real network.
- The recovery takes ~10–50 s because production uses a 10 s jitter/back-off
  window (`syntheticHealWindow`) so that, with multiple validators, only one
  or two actually issue the pull.
- Healing is enabled per node via `enable-synthetic-healing = true` in the
  `coreValidator` configuration (default off).
- **A wedge at the head of a stream, with no traffic behind it, does not
  self-heal.** Receiver-pull healing needs a later sequence number to arrive
  before it notices the gap. This test only works because it fires all five
  sends without waiting, so sequences 2–5 expose the hole at sequence 1. A
  driver that submits one message and blocks on it will wait forever — which is
  why `../synth-mixed/` uses modulo drops (`*:%16`) that skip the low sequence
  numbers its account bootstrap occupies.
