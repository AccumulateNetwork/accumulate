# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 420 timed reads, p50 1.1 ms, p95 2.4 ms, p99 90.8 ms, **max 183.9 ms** (txn read, Directory, entry 33 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 312 | 1.1 | 3.7 | 183.9 |
| 100–1000 | 108 | 0.9 | 1.6 | 2.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 210 | 0.6 | 1.3 | 158.8 |
| txn | 210 | 1.3 | 3.8 | 183.9 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 183.9 | txn | Directory | 33 |
| 158.8 | chain | BVN1 | 33 |
| 135.5 | txn | Directory | 33 |
| 132.0 | txn | Directory | 33 |
| 90.8 | chain | BVN1 | 33 |
| 80.1 | txn | BVN2 | 30 |
| 68.8 | txn | BVN1 | 33 |
| 54.3 | txn | BVN1 | 33 |
| 50.8 | txn | BVN2 | 30 |
| 44.4 | txn | Directory | 33 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 04:28:49 | 6 | 1.5 | 2.3 | 2.3 | txn BVN1 age 8 | 0 | 0 | 0 | 8 |
| 04:29:50 | 24 | 9.9 | 158.8 | 183.9 | txn Directory age 33 | 0 | 0 | 0 | 33 |
| 04:30:50 | 42 | 1.2 | 2.2 | 2.9 | txn BVN1 age 50 | 0 | 0 | 0 | 53 |
| 04:31:50 | 60 | 1.1 | 2.9 | 3.8 | txn BVN2 age 50 | 0 | 0 | 0 | 73 |
| 04:32:51 | 78 | 0.9 | 1.4 | 3.8 | txn BVN1 age 77 | 0 | 0 | 0 | 93 |
| 04:33:51 | 96 | 1.1 | 1.9 | 2.5 | txn BVN2 age 50 | 0 | 0 | 0 | 113 |
| 04:34:52 | 114 | 0.9 | 1.3 | 1.7 | txn Directory age 133 | 0 | 0 | 0 | 133 |
