# Routing

Accumulate is partitioned into multiple block validator networks (BVNs) plus one
directory network (DN). Identities are partitioned across these partition works.
Almost all identities are partitioned to a BVN. A few identities are partitioned
to the DN. The routing algorithm determines how identities are partitioned
across the BVNs and DN.

The routing table consists of a list of routes consisting of a prefix length,
value, and partition ID, plus a set of overrides consisting of an account URL
and a partition ID. Routes are matched to identities by taking the first N bits
of the account ID (the prefix length) and comparing it to the value. Thus,
routes match blocks of identities by account ID. Each override matches a single
identity, allowing control of the routing of specific identities.

The routing table must be unambiguous. Every identity must match exactly one
route. This means in effect that the routes describe a bitwise trie (prefix
tree) with no empty leafs. For example, this routing table:

| Prefix | Partition |
| ------ | --------- |
| 00     | BVN A     |
| 01     | BVN B     |
| 10     | BVN C     |
| 110    | BVN D     |
| 111    | BVN E     |

is equivalent to this radix trie:

- 00 → BVN A
- 01 → BVN B
- 10 → BVN C
- 11 ↴
  - 0 → BVN D
  - 1 → BVN E