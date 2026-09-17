# BVN Routing: Buckets

**Issue:** #4136
**Branch:** `issue-4136-bucket-routing-di`
**Status:** Phase 1 implemented and measured; phases 2-4 open
**Date:** 2026-09-17

---

## What routing decides

Every account belongs to exactly one BVN. The decision is made from the
account's *identity*: `URL.Routing()` takes the first 8 bytes of the identity
account ID as a `uint64`, and the routing table maps that number to a
partition. Every account under an ADI therefore lives on the same BVN as the
ADI, which matters more than it first appears — see "Balance is per identity".

The table lives on chain, as a data account on the Directory
(`acc://dn.acme/routing`), and is carried in `GlobalValues.Routing`. It is
consensus data with a defined wire format, and it is served over the API. But
nothing outside the protocol interprets it: no wallet, explorer or SDK computes
routes.

## The problem with bit prefixes

Routing used to walk the number bit by bit against a table of
`{Length, Value, Partition}` prefix routes. A prefix can only divide the space
into powers of two:

| BVNs | share per BVN |
|---|---|
| 2 | 50 / 50 |
| **3** | **50 / 25 / 25** |
| 4 | even |
| 5 | 25 / 25 / 25 / 12.5 / 12.5 |
| 8 | even |

At three BVNs one carried twice the accounts of each of the others.

Prefixes also pin the leading bits of every ADI on a BVN. Execution sharding
shards on the ADI hash, so sharding on those same leading bits would see only
part of the space. Today that is dodged by taking the shard bits from different
bytes of the hash, which works but spends a second region of the hash to avoid
colliding with the first.

## The design: buckets

An account's **bucket** is the leading 20 bits of its routing number, so the
space divides into 1,048,576 buckets and a partition is assigned whole buckets.

Leading bits, not the number modulo a count. That choice is what makes the
change free: a bucket nests inside a prefix, so every prefix route is exactly a
contiguous run of buckets.

```
today, 3 BVNs, 2-bit prefixes        as 2^20 buckets
  prefix 00,01 -> BVN0  (50%)          BVN0 = [0,       524288)
  prefix 10    -> BVN1  (25%)          BVN1 = [524288,  786432)
  prefix 11    -> BVN2  (25%)          BVN2 = [786432, 1048576)
```

Converting an existing table to buckets is a change of representation that
moves no account, which is why the mechanism could land before account
migration exists.

## What is implemented

### Routing by bucket, moving nothing

`internal/api/routing/bucket.go` replaces the prefix walk. A table is converted
to bucket ranges once when it is loaded, and a lookup is a binary search over
as many ranges as there are routes.

The acceptance test is that no account moves. The prefix walk is kept verbatim
as a test-only reference (`prefix_reference_test.go`) and the new routing is
checked against it, not against a description of it: every table shape from 1
to 16 partitions, every bucket boundary and the numbers either side of it,
200,000 random routing numbers per shape, and 20,000 real account URLs
(`routing_equivalence_test.go`).

Two things improved on the way past:

- A table that leaves a bucket unassigned, or assigns one twice, is refused
  when it is loaded. The prefix tree reported a gap as
  `no entry for N at D.B` only once an account happened to land in it.
- Building the tree no longer sorts the caller's route slice in place. That
  slice belongs to the network's global values, shared with every other reader
  of them.

### An even split for new networks

`BuildEvenTable` (`internal/api/routing/even.go`) divides the buckets into
equal contiguous runs, one per partition. Genesis uses it.

This needs no change to the table's format. A run of buckets is rarely a single
prefix, but it is always a small union of them — a third of the space is a
quarter, then a sixteenth, then a sixty-fourth, and so on down — so each
partition's share is written as the fewest prefixes that cover it. Three
partitions come out as 33.3333/33.3333/33.3334 in **39 routes** instead of 3,
and nothing in the protocol cares how many routes a table has.

It applies to networks being created. An existing network keeps the assignment
it has, because changing it moves accounts.

## What is not implemented

- **Interleaving.** Each partition still holds one contiguous run, so the
  leading bits of a hash still say which partition an account is on, and
  execution sharding on those same bits still sees only part of the space. An
  interleaved assignment (`bucket % N`) is a rule rather than a list of ranges
  — a million single-bucket routes is not a table anyone would write — so it
  needs the table format to change, and that shape should be designed together
  with the migration that needs it.
- **Account migration.** Rebalancing an existing network requires moving
  accounts, and moving them safely requires an answer for work already in
  flight: a routing change propagates as globals, not as a barrier, so for some
  interval parts of the network disagree about where an ADI lives.

## Balance is per identity, not per account

Routing places an *identity*, and every account under an ADI travels with it.
So the sample size for "are the BVNs balanced" is the identity count, not the
account count.

Measured on a three-BVN network at 100 tps for an hour (soak run
`20260917T161555Z`), with an exactly even table:

| measure | BVN1 | BVN2 | BVN3 |
|---|---|---|---|
| buckets assigned | 33.3333% | 33.3333% | 33.3334% |
| permanent store writes | 33.79% | 37.05% | 29.16% |

The run created 239 identities and 9,194 accounts. Routing 244 identities
through that same table 2,000 times gives a median spread between busiest and
quietest of 17.2% of a third, and a 90th percentile of 32.0%; the observed
23.7% sits between them. At 1,000 identities the median falls to 9.0%, and at
100,000 to 1.0%.

**Finer buckets cannot fix this.** A million buckets are already far finer than
a few hundred identities. What fixes it is more identities, or assigning by
something smaller than one.

A second, separate effect: the load generator funds everything from a single
treasury lite account, so whichever BVN holds it absorbs the whole bootstrap.
In that run it was BVN2, at 2.13x in the first sample, diluting to about 1.24.

## How a routing change is enacted

Not a code deploy. The table is a data account on the Directory, so changing an
assignment is a governance write to `acc://dn.acme/routing`, taking effect as
the new globals propagate. That splits the work:

- **The format** is code, released with the node.
- **The assignment** — rebalancing, adding a BVN — is a governance write, made
  whenever operators choose, with no release involved.

## Where the code is

| what | where |
|---|---|
| bucket function, table, validation | `internal/api/routing/bucket.go` |
| even assignment for new networks | `internal/api/routing/even.go` |
| the route tree | `internal/api/routing/tree.go` |
| prefix table builder (still used by tools) | `internal/api/routing/simple.go` |
| genesis picks the table | `internal/node/genesis/bootstrap.go` |
| the routing number | `pkg/url/url.go`, `URL.Routing()` |
| the table's type | `protocol/general.yml`, `Route` / `RoutingTable` |
| served over the API | `internal/api/v3/network.go` |

Six implementations of `RouteAccount` must agree; all delegate to the route
tree above, so the change is in one place.

## Tests that pin this

| test | what it holds |
|---|---|
| `TestBucketRoutingMatchesPrefixRouting` | no account moves, against the old walk |
| `TestBucketRoutingMatchesPrefixRoutingForADIs` | the same over real account URLs |
| `TestBucketRoutingPreservesTheSkew` | a converted table keeps its 50/25/25 |
| `TestBuildEvenTableIsEven` | a new network's split is even to within one bucket |
| `TestBuildEvenTableStaysSmall` | expressing it as prefixes does not blow up the table |
| `TestNewRouteTreeRejectsBadTables` | gaps and overlaps are refused at load |
| `TestNewRouteTreeDoesNotModifyTheTable` | the caller's routes are not sorted in place |

## BPT sharding is unaffected

`BPT.Insert` keys on `record.Key.Hash()`, a chained SHA-256 over
`("Account", url)`, not the routing hash. Prefix routing never constrained BPT
shard spread and buckets do not either.
