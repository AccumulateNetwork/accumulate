# Network parameters — Specification

What a network declares about itself, as opposed to what each node happens to
be configured with.

## 1. Architecture — what we are doing

A network has properties that are true of the *network*, not of a node: the
acceptance thresholds, the major block schedule, the fee schedule, the limits.
They are recorded in `NetworkGlobals` when the network is deployed, they are
readable from the network afterwards, and every node uses the recorded value.

**Block interval is one of them.** A network runs at one cadence. The
alternative — each node pacing itself from its own configuration — does not
produce a network property at all: both engines advance on a quorum, so the
cadence becomes whatever the quorum-th node produces. That has three
consequences, and all three are defects rather than untidiness:

- a misconfigured node cannot be detected, because there is no network value to
  compare it against;
- the cadence changes when the validator set changes, since who is in the
  quorum decides how fast the network runs;
- everything denominated in blocks — anchor emission, pending expiry, the
  execution-lag bound — rests on a number nothing guarantees.

### The invariants

1. **A deployed network declares its block interval.** A default is
   permitted; an *unrecorded* default is not. After deployment the value is
   read from the network, never inferred from a node's binary.
2. **A node paces from the network's value.** Not from its own.
3. **A node that disagrees does not run.** The divergence is refused and named
   at startup — not silently honoured, which would restore the emergent
   cadence, and not silently overridden, because an operator who set a value is
   entitled to be told their node is not running it.
4. **Catch-up is exempt.** A node behind the frontier ignores pacing entirely,
   and enforcing the network value must not change that: pacing a catch-up is
   an absorbing state, and it once pinned 4 of 12 validators behind for a whole
   run.

## 2. Specification — how it is implemented

**Recorded.** `NetworkGlobals.BlockInterval`, a duration, beside
`MajorBlockSchedule` — the timing parameter that already lives there. It is an
appended field, so a network deployed before it decodes with the field absent.
`NewGlobals` fills in `protocol.DefaultBlockInterval` when the operator states
nothing, so genesis always records a value. Zero means nothing was stated and
takes the default; a negative value is a wrong statement rather than an absent
one, so it survives to be refused by `genesis.Init`.

**Resolved at startup.** `resolveBlockInterval` takes what the network declares
and what the node was configured with:

| network declares | node states | result |
|---|---|---|
| a value | nothing | the network's |
| a value | the same | the network's |
| a value | something else | **refused, both values named** |
| nothing | a value | the node's, with a warning |
| nothing | nothing | the default, with a warning |

A network that declares nothing is one deployed before the field existed. Those
keep running on local configuration; they are not locked out.

**Applied.** Rounds pace at half the block interval, because Bullshark commits
a leader every other round. The resolved value sets
`NodeConfig.MinRoundInterval`, which is the only pacing input the primary has.
The catch-up exemption is upstream of it, in the primary's `behind` check, and
is untouched by resolution.

**The recorded default and the pacing default are the same number** in two
packages that cannot reference each other. A test asserts their equality: if
they drift, recording a default at deployment silently re-paces every node that
stated nothing.

### Not yet done

Changing the interval after deployment. The value is fixed when the network is
deployed and there is no ratification path: a change requires redeploying.
Making it changeable — a threshold of validators ratifying, activating at a
major block boundary so partitions switch together — is #4267's remaining
scope. Until then a deployed network's cadence is immutable, which is a
limitation, not a design.
