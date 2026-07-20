# Mixed-workload docker test (#4067)

The `synth-heal` harness wired for the **mixed** workload: instead of only
lite → lite token sends, it drives every user transaction type that produces a
cross-partition synthetic, and reports coverage per synthetic type.

See [`../synth-heal/README.md`](../synth-heal/README.md) for how the wedge is
created (`ACC_DEBUG_DROP_SYNTHETIC`) and how receiver-pull healing recovers it —
that part is identical. What differs here is only what the driver sends.

## Run

```sh
./run-test.sh                        # builds the image if needed, runs, tears down
COUNT=48 TIMEOUT=20m ./run-test.sh   # longer run
KEEP=1 ./run-test.sh                 # leave the network up afterwards
```

It uses its own compose project (`synthmix`), container names (`acc-mx-*`),
image tag (`acc-synthmix:test`) and host ports (26670+), so it can run
alongside `../synth-heal/` and its soak without colliding.

## Expected tail

```
== workload (user transactions followed) ==
  lite-transfer            submitted=16     delivered=16
  adi-transfer             submitted=12     delivered=12
  data-write               submitted=12     delivered=12
  credit-purchase          submitted=9      delivered=9
  cross-auth               submitted=9      delivered=9
  token-burn               submitted=3      delivered=3
  adi-create               submitted=3      delivered=3

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

== Evidence ==
-- drop (wedge formed) --
acc-mx-bvn2 | ... WARN DEBUG dropping sequenced envelope anchor=false destination=acc://bvn-BVN1.acme
-- heal counters via ConsensusStatus, every validator (receiver-pull fired) --
  acc-mx-bvn1:
    BVN1 "syntheticHeals":3
RESULT: PASS (mixed workload delivered; healing recorded)
```

`signature` and `writeData` are in the table because they genuinely cross a
partition boundary too: a cross-auth transaction forwards its signature to the
principal's partition, and the transaction itself is executed there. They are
healed like any other cross-partition message.

A `!` in the left margin marks a type with undelivered messages, or one that was
expected but never produced — either fails the run.
