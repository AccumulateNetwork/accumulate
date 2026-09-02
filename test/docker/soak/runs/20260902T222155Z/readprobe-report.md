# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 870 timed reads, p50 1.6 ms, p95 6.0 ms, p99 21.5 ms, **max 95.4 ms** (txn read, Directory, entry 94 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 430 | 1.7 | 7.9 | 95.4 |
| 100–1000 | 440 | 1.5 | 3.6 | 29.1 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 435 | 0.9 | 3.9 | 29.1 |
| txn | 435 | 2.0 | 6.9 | 95.4 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 95.4 | txn | Directory | 94 |
| 91.3 | txn | BVN2 | 72 |
| 70.9 | txn | BVN1 | 84 |
| 33.8 | txn | BVN2 | 90 |
| 33.5 | txn | BVN1 | 98 |
| 29.1 | chain | BVN2 | 147 |
| 23.7 | chain | BVN1 | 98 |
| 22.0 | txn | Directory | 134 |
| 21.5 | txn | Directory | 54 |
| 17.6 | chain | BVN2 | 90 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 22:23:59 | 6 | 1.7 | 2.1 | 2.1 | txn BVN2 age 6 | 0 | 0 | 0 | 8 |
| 22:25:00 | 24 | 1.0 | 1.4 | 1.7 | txn BVN2 age 30 | 0 | 0 | 0 | 34 |
| 22:26:00 | 42 | 2.2 | 13.4 | 21.5 | txn Directory age 54 | 0 | 0 | 0 | 54 |
| 22:27:01 | 60 | 2.3 | 9.2 | 91.3 | txn BVN2 age 72 | 0 | 0 | 0 | 74 |
| 22:28:02 | 78 | 2.8 | 17.6 | 95.4 | txn Directory age 94 | 0 | 0 | 0 | 94 |
| 22:29:02 | 96 | 1.6 | 2.3 | 10.0 | txn BVN1 age 98 | 0 | 0 | 0 | 114 |
| 22:30:03 | 114 | 1.8 | 7.9 | 23.7 | chain BVN1 age 98 | 0 | 0 | 0 | 134 |
| 22:31:03 | 132 | 1.6 | 6.6 | 33.5 | txn BVN1 age 98 | 0 | 0 | 0 | 154 |
| 22:32:04 | 150 | 1.6 | 2.5 | 5.8 | txn Directory age 174 | 0 | 0 | 0 | 174 |
| 22:33:04 | 168 | 1.4 | 2.6 | 13.4 | txn BVN1 age 98 | 0 | 0 | 0 | 191 |
