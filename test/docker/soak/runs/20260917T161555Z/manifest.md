# Soak run 20260917T161555Z

**Purpose:** 3 BVNs at 100 tps, chaos off, 1 h. First run of even bucket routing (#4136): BuildEvenTable divides the 2^20 buckets into equal contiguous runs, so three BVNs are 33.3333/33.3333/33.3334 where bit prefixes gave 50/25/25. Store is BlockchainDB 8751863. Question: do the three BVNs carry the same share of accounts and the same share of the work? Watch: accounts and block/CPU/disk per BVN, which must be even within a few percent; accepted tps vs 100; execution lag; refusals; heals must stay zero.

| field | value |
|---|---|
| started (UTC) | 2026-09-17T16:15:55Z |
| commit | `ea34d928467f37192f062131551aed0ac73f34af` |
| describe | `10k-tps-725-gea34d9284` |
| branch | `issue-4136-bucket-routing-di` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:048265a0c12a5c377db4c26295b7b71439b92c0c542ed9dc28dcb999cf267a7b` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | none (CHAOS=off) |
| topology | 3 BVNs, 12 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | off |
| target duration | 1h |
| target TPS | 100 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-17T17:18:16Z |
| elapsed | 1.01h |
| driver exit | 0 (clean) |
| dn height | 10 -> 3616 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 111 |
| seizure | SEIZED at 2026-09-17T16:32:32 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=1923 undeliv=synthetic BVN2->BVN3 undeliv=118 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 5 |
| read-back probe | Whole run: 16076 timed reads, p50 1.5 ms, p95 3.0 ms, p99 7.8 ms, max 27.5 ms (txn read, BVN3, entry 3266 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Result — three BVNs, evenly assigned, and what "balanced" turns out to mean

**Question.** With buckets divided evenly (#4136), do three BVNs carry the
same share?

**The assignment is exactly even.** Queried from the live network rather than
computed from the source:

| partition | buckets | share |
|---|---|---|
| BVN1 | 349,525 | 33.3333% |
| BVN2 | 349,525 | 33.3333% |
| BVN3 | 349,526 | 33.3334% |

39 routes, covering all 1,048,576 buckets with no gap and no overlap, against
the 50/25/25 that bit prefixes gave. The five overrides are the expected ones:
ACME and dn.acme to the Directory, each partition's own URL to itself.

**The run was clean.** 363,182 transactions at 100.3-100.9 tps against a
target of 100, ONE refusal in the whole hour, zero heals, nothing unexecuted,
no wedges, and 16,076 read-back probes with none failed (p50 1.5 ms, p99
7.8 ms, max 27.5 ms). Directory 10 -> 3,616, and the three BVNs stayed within
four blocks of each other from the first minute to the last.

**The work is not as even as the assignment, and that is arithmetic, not a
defect.** Share of store writes per BVN, averaged over each BVN's four nodes:

| measure | BVN1 | BVN2 | BVN3 | max/min |
|---|---|---|---|---|
| permanent puts | 33.79% | 37.05% | 29.16% | 1.271 |
| dynamic puts | 34.66% | 35.33% | 30.01% | 1.177 |
| permanent puts, steady state | 34.51% | 36.27% | 29.23% | 1.241 |

Two things account for the gap, and neither is the routing table.

1. **Bootstrap lands on one BVN.** The load generator funds everything from a
   single treasury lite account, and that account routes to BVN2, so all 100
   sub-treasury funding sends executed there. In the first sample BVN2 had
   2.13x BVN1's permanent puts; by the second interval that was 1.23 and it
   stays near there, which is the lump being diluted rather than a rate.

2. **Routing places an identity, not an account.** Every account under an ADI
   lives with the ADI, so the sample size for balance is the 239 identities
   this run created, not its 9,194 accounts. Routing 244 identities through
   this run's own table, 2,000 times, gives a median spread between the
   busiest and quietest BVN of 17.2% of a third, and a 90th percentile of
   32.0%. The observed 23.7% sits between them. At 1,000 identities the median
   spread falls to 9.0%, and at 100,000 to 1.0%.

   So a network with few, busy identities can be lopsided while its routing is
   exactly even, and no change to routing granularity fixes that -- 2^20
   buckets are already far finer than 239 identities. What fixes it is more
   identities, or assigning by something other than the identity.

**Two reporting defects, both already seen in 20260917T134651Z and both still
here.** `seizure` records SEIZED at 16:32:32 on synthetic BVN2->BVN3 with
about 118 undelivered, while `deliv` was climbing and `gap` was 0 -- the
watchdog tests that `undeliv` is non-zero for 20 polls, which is true of any
working pipeline. And `chaos events` reads 1 although `CHAOS=off`, because the
counter counts the `DISABLED for this run` line in chaos.log.

**Store under test.** BlockchainDB `8751863`. The routing change under test is
this branch's, not the store's.
