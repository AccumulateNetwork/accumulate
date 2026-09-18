# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3676 timed reads, p50 1.5 ms, p95 3.6 ms, p99 17.5 ms, **max 8040.4 ms** (chain read, Directory, entry 829 blocks old); 24 failed, 24 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.2 | 2.6 | 4.2 |
| 100–1000 | 2664 | 1.5 | 3.7 | 8040.4 |
| 1000–5000 | 972 | 1.5 | 3.3 | 23.6 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1838 | 0.7 | 1.9 | 8040.4 |
| txn | 1838 | 2.0 | 4.3 | 8040.4 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.4 | chain | Directory | 829 |
| 8040.4 | txn | BVN2 | 384 |
| 8040.3 | chain | BVN3 | 866 |
| 8040.3 | txn | Directory | 396 |
| 8040.3 | chain | BVN1 | 842 |
| 8040.3 | txn | BVN2 | 384 |
| 8040.2 | chain | Directory | 829 |
| 8040.2 | txn | BVN1 | 397 |
| 8040.1 | chain | Directory | 829 |
| 8040.1 | chain | BVN2 | 755 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 02:32:43 | 8 | 2.2 | 2.6 | 2.6 | txn BVN1 age 34 | 0 | 0 | 0 | 37 |
| 02:33:44 | 32 | 1.2 | 1.9 | 4.2 | txn BVN2 age 94 | 0 | 0 | 0 | 97 |
| 02:34:44 | 56 | 1.5 | 4.1 | 5.3 | chain Directory age 157 | 0 | 0 | 0 | 157 |
| 02:35:45 | 80 | 1.2 | 2.6 | 3.1 | txn Directory age 213 | 0 | 0 | 0 | 213 |
| 02:36:45 | 104 | 1.4 | 2.9 | 3.7 | txn BVN1 age 270 | 0 | 0 | 0 | 273 |
| 02:37:46 | 128 | 1.5 | 3.2 | 4.5 | txn BVN1 age 329 | 0 | 0 | 0 | 332 |
| 02:40:39 | 152 | 1.8 | 8039.6 | 8040.4 | txn BVN2 age 384 | 13 | 0 | 13 | 399 |
| 02:40:54 | 160 | 1.7 | 5.5 | 6337.6 | txn BVN3 age 399 | 0 | 0 | 0 | 501 |
| 02:41:41 | 184 | 1.6 | 3.3 | 8.9 | txn BVN2 age 516 | 0 | 0 | 0 | 563 |
| 02:42:42 | 208 | 1.5 | 2.6 | 4.0 | txn BVN1 age 621 | 0 | 0 | 0 | 623 |
| 02:43:42 | 232 | 1.5 | 4.5 | 21.5 | txn BVN3 age 681 | 0 | 0 | 0 | 681 |
| 02:44:43 | 256 | 1.5 | 3.3 | 17.8 | chain BVN3 age 739 | 0 | 0 | 0 | 739 |
| 02:45:44 | 280 | 1.4 | 2.7 | 7.4 | txn BVN2 age 755 | 0 | 0 | 0 | 799 |
| 02:48:23 | 296 | 1.7 | 14.1 | 8040.4 | chain Directory age 829 | 11 | 0 | 11 | 866 |
| 02:48:25 | 300 | 1.6 | 3.7 | 35.4 | txn BVN2 age 911 | 0 | 0 | 0 | 955 |
| 02:49:25 | 300 | 1.5 | 3.1 | 47.4 | txn Directory age 971 | 0 | 0 | 0 | 1017 |
| 02:50:26 | 300 | 1.5 | 3.4 | 11.7 | txn Directory age 1032 | 0 | 0 | 0 | 1077 |
| 02:51:27 | 300 | 1.5 | 3.6 | 15.5 | txn BVN1 age 1093 | 0 | 0 | 0 | 1137 |
| 02:52:28 | 300 | 1.4 | 3.2 | 23.6 | txn BVN1 age 1153 | 0 | 0 | 0 | 1197 |
