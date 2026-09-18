# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2496 timed reads, p50 2.1 ms, p95 24.7 ms, p99 73.7 ms, **max 493.7 ms** (chain read, Directory, entry 994 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.5 | 2.9 | 3.1 |
| 100–1000 | 2466 | 2.1 | 24.9 | 493.7 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1248 | 0.9 | 7.0 | 493.7 |
| txn | 1248 | 2.9 | 40.2 | 256.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 493.7 | chain | Directory | 994 |
| 256.5 | txn | Directory | 994 |
| 207.3 | txn | Directory | 994 |
| 199.4 | txn | BVN2 | 980 |
| 182.0 | txn | BVN1 | 920 |
| 178.0 | txn | Directory | 994 |
| 129.2 | txn | Directory | 994 |
| 118.6 | txn | BVN1 | 920 |
| 108.3 | txn | Directory | 994 |
| 100.1 | txn | BVN2 | 980 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 14:09:45 | 6 | 1.6 | 1.7 | 1.7 | txn BVN1 age 8 | 0 | 0 | 0 | 10 |
| 14:10:46 | 24 | 1.5 | 2.9 | 3.1 | txn Directory age 96 | 0 | 0 | 0 | 96 |
| 14:11:46 | 42 | 1.2 | 1.8 | 1.9 | txn Directory age 156 | 0 | 0 | 0 | 156 |
| 14:12:47 | 60 | 1.3 | 26.5 | 32.8 | txn BVN1 age 212 | 0 | 0 | 0 | 216 |
| 14:13:47 | 78 | 1.4 | 2.6 | 5.7 | txn BVN2 age 270 | 0 | 0 | 0 | 276 |
| 14:14:48 | 96 | 1.2 | 13.8 | 19.9 | txn Directory age 336 | 0 | 0 | 0 | 336 |
| 14:15:48 | 114 | 1.8 | 9.9 | 35.1 | txn BVN2 age 389 | 0 | 0 | 0 | 396 |
| 14:16:49 | 132 | 2.0 | 30.9 | 41.3 | txn BVN2 age 449 | 0 | 0 | 0 | 456 |
| 14:17:49 | 144 | 2.2 | 5.6 | 10.5 | txn Directory age 515 | 0 | 0 | 0 | 515 |
| 14:18:50 | 162 | 1.9 | 3.3 | 13.4 | txn BVN2 age 568 | 0 | 0 | 0 | 575 |
| 14:19:51 | 180 | 2.0 | 5.4 | 38.1 | txn BVN2 age 627 | 0 | 0 | 0 | 635 |
| 14:20:52 | 198 | 2.3 | 9.7 | 75.1 | txn BVN1 age 684 | 0 | 0 | 0 | 695 |
| 14:21:52 | 216 | 2.5 | 13.0 | 51.2 | txn BVN1 age 745 | 0 | 0 | 0 | 754 |
| 14:22:52 | 234 | 1.8 | 6.9 | 19.3 | txn BVN1 age 804 | 0 | 0 | 0 | 814 |
| 14:23:54 | 252 | 2.6 | 33.7 | 93.3 | txn BVN1 age 863 | 0 | 0 | 0 | 874 |
| 14:24:55 | 270 | 3.2 | 48.1 | 182.0 | txn BVN1 age 920 | 0 | 0 | 0 | 933 |
| 14:25:59 | 288 | 6.1 | 72.6 | 493.7 | chain Directory age 994 | 0 | 0 | 0 | 994 |
