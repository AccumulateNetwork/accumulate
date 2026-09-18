# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2768 timed reads, p50 1.6 ms, p95 4.9 ms, p99 3243.3 ms, **max 8040.4 ms** (txn read, BVN1, entry 396 blocks old); 25 failed, 25 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.5 | 16.0 | 22.5 |
| 100–1000 | 2580 | 1.6 | 5.0 | 8040.4 |
| 1000–5000 | 148 | 1.3 | 2.0 | 3.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1384 | 0.7 | 2.4 | 8040.3 |
| txn | 1384 | 2.0 | 6.2 | 8040.4 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.4 | txn | BVN1 | 396 |
| 8040.3 | txn | BVN1 | 775 |
| 8040.3 | chain | Directory | 744 |
| 8040.3 | txn | BVN3 | 399 |
| 8040.3 | txn | BVN3 | 399 |
| 8040.2 | txn | BVN3 | 399 |
| 8040.2 | chain | BVN1 | 775 |
| 8040.2 | txn | BVN3 | 399 |
| 8040.1 | chain | BVN2 | 744 |
| 8040.0 | chain | BVN2 | 744 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 01:55:45 | 8 | 3.3 | 16.0 | 16.0 | chain BVN2 age 35 | 0 | 0 | 0 | 37 |
| 01:56:45 | 32 | 1.5 | 8.3 | 22.5 | txn BVN1 age 94 | 0 | 0 | 0 | 98 |
| 01:57:46 | 56 | 1.7 | 8.1 | 12.8 | txn Directory age 157 | 0 | 0 | 0 | 157 |
| 01:58:46 | 80 | 1.6 | 2.5 | 3.2 | txn BVN2 age 213 | 0 | 0 | 0 | 214 |
| 01:59:46 | 104 | 1.3 | 2.4 | 2.9 | chain BVN2 age 272 | 0 | 0 | 0 | 272 |
| 02:00:47 | 128 | 1.4 | 2.6 | 5.0 | txn Directory age 325 | 0 | 0 | 0 | 331 |
| 02:03:11 | 152 | 1.8 | 8039.2 | 8040.4 | txn BVN1 age 396 | 9 | 0 | 9 | 399 |
| 02:03:13 | 160 | 1.4 | 3.8 | 13.7 | txn BVN2 age 440 | 0 | 0 | 0 | 475 |
| 02:04:13 | 184 | 1.6 | 2.6 | 8.6 | txn BVN1 age 531 | 0 | 0 | 0 | 534 |
| 02:05:14 | 208 | 1.8 | 2.8 | 7.4 | txn BVN1 age 591 | 0 | 0 | 0 | 594 |
| 02:06:15 | 232 | 2.8 | 9.9 | 39.8 | chain BVN2 age 617 | 0 | 0 | 0 | 651 |
| 02:07:15 | 256 | 1.6 | 3.7 | 23.7 | txn Directory age 681 | 0 | 0 | 0 | 710 |
| 02:10:42 | 280 | 1.7 | 8039.4 | 8040.3 | txn BVN1 age 775 | 16 | 0 | 16 | 777 |
| 02:10:43 | 288 | 1.6 | 3.9 | 21.5 | txn BVN1 age 876 | 0 | 0 | 0 | 914 |
| 02:11:44 | 300 | 1.5 | 2.5 | 12.6 | chain BVN1 age 934 | 0 | 0 | 0 | 974 |
| 02:12:45 | 300 | 1.3 | 2.1 | 8.1 | chain BVN1 age 994 | 0 | 0 | 0 | 1034 |
