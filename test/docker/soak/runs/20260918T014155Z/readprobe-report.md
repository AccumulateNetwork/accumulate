# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 872 timed reads, p50 1.5 ms, p95 7.5 ms, p99 8039.9 ms, **max 8040.2 ms** (txn read, BVN3, entry 339 blocks old); 13 failed, 13 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.4 | 11.5 | 13.8 |
| 100–1000 | 832 | 1.5 | 7.5 | 8040.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 436 | 0.7 | 2.0 | 14.7 |
| txn | 436 | 2.0 | 19.3 | 8040.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.2 | txn | BVN3 | 339 |
| 8040.1 | txn | BVN1 | 337 |
| 8040.1 | txn | BVN2 | 328 |
| 8040.1 | txn | BVN1 | 337 |
| 8040.1 | txn | BVN3 | 339 |
| 8040.0 | txn | BVN1 | 337 |
| 8040.0 | txn | BVN3 | 339 |
| 8039.9 | txn | BVN1 | 425 |
| 8039.9 | txn | BVN2 | 391 |
| 8039.7 | txn | BVN3 | 339 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 01:43:50 | 8 | 2.3 | 13.8 | 13.8 | txn BVN1 age 35 | 0 | 0 | 0 | 38 |
| 01:44:50 | 32 | 1.4 | 3.7 | 4.0 | txn BVN3 age 94 | 0 | 0 | 0 | 98 |
| 01:45:51 | 56 | 1.3 | 2.6 | 3.0 | txn BVN2 age 154 | 0 | 0 | 0 | 158 |
| 01:46:51 | 80 | 1.5 | 2.5 | 3.5 | txn Directory age 216 | 0 | 0 | 0 | 216 |
| 01:47:52 | 104 | 1.3 | 2.5 | 3.4 | txn BVN2 age 273 | 0 | 0 | 0 | 276 |
| 01:50:29 | 128 | 1.9 | 8040.0 | 8040.2 | txn BVN3 age 339 | 11 | 0 | 11 | 342 |
| 01:50:52 | 136 | 1.7 | 7.5 | 8039.9 | txn BVN1 age 425 | 2 | 0 | 2 | 427 |
| 01:51:30 | 152 | 1.5 | 11.5 | 31.4 | txn BVN3 age 487 | 0 | 0 | 0 | 487 |
| 01:52:31 | 176 | 1.6 | 10.4 | 21.2 | txn BVN1 age 544 | 0 | 0 | 0 | 546 |
