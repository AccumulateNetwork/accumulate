# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2802 timed reads, p50 1.9 ms, p95 14.8 ms, p99 45.3 ms, **max 1744.9 ms** (txn read, BVN2, entry 925 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 22 | 1.7 | 5.4 | 5.5 |
| 100–1000 | 2480 | 1.8 | 14.8 | 1744.9 |
| 1000–5000 | 300 | 2.5 | 21.4 | 48.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1401 | 0.8 | 4.4 | 193.3 |
| txn | 1401 | 2.8 | 27.9 | 1744.9 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 1744.9 | txn | BVN2 | 925 |
| 193.3 | chain | BVN2 | 807 |
| 111.1 | txn | BVN2 | 925 |
| 95.6 | txn | BVN1 | 921 |
| 94.4 | txn | BVN1 | 921 |
| 88.4 | txn | BVN2 | 925 |
| 80.5 | txn | Directory | 817 |
| 80.0 | txn | Directory | 817 |
| 75.9 | txn | BVN2 | 752 |
| 74.2 | txn | BVN2 | 807 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 14:31:05 | 6 | 1.7 | 2.1 | 2.1 | txn BVN2 age 12 | 0 | 0 | 0 | 15 |
| 14:32:05 | 24 | 1.7 | 5.4 | 5.5 | chain BVN1 age 92 | 0 | 0 | 0 | 101 |
| 14:33:06 | 42 | 1.2 | 2.3 | 10.8 | txn Directory age 160 | 0 | 0 | 0 | 160 |
| 14:34:06 | 60 | 1.1 | 1.7 | 1.9 | txn BVN2 age 216 | 0 | 0 | 0 | 220 |
| 14:35:07 | 78 | 1.7 | 6.7 | 11.9 | chain BVN2 age 275 | 0 | 0 | 0 | 279 |
| 14:36:07 | 96 | 1.4 | 2.8 | 8.7 | txn Directory age 339 | 0 | 0 | 0 | 339 |
| 14:37:08 | 114 | 1.4 | 2.0 | 6.5 | txn Directory age 399 | 0 | 0 | 0 | 399 |
| 14:38:09 | 132 | 2.0 | 32.6 | 44.2 | txn BVN1 age 448 | 0 | 0 | 0 | 459 |
| 14:39:10 | 150 | 1.5 | 27.4 | 38.9 | txn BVN1 age 508 | 0 | 0 | 0 | 519 |
| 14:40:10 | 162 | 2.2 | 5.4 | 8.9 | txn BVN1 age 567 | 0 | 0 | 0 | 578 |
| 14:41:10 | 180 | 2.1 | 7.0 | 29.6 | txn BVN2 age 631 | 0 | 0 | 0 | 638 |
| 14:42:11 | 198 | 1.7 | 3.5 | 39.4 | txn BVN1 age 687 | 0 | 0 | 0 | 698 |
| 14:43:12 | 216 | 1.8 | 5.8 | 75.9 | txn BVN2 age 752 | 0 | 0 | 0 | 758 |
| 14:44:14 | 234 | 3.1 | 45.3 | 193.3 | chain BVN2 age 807 | 0 | 0 | 0 | 817 |
| 14:45:13 | 252 | 1.9 | 7.0 | 37.9 | txn BVN2 age 868 | 0 | 0 | 0 | 877 |
| 14:46:16 | 270 | 2.9 | 38.6 | 1744.9 | txn BVN2 age 925 | 0 | 0 | 0 | 936 |
| 14:47:14 | 288 | 3.2 | 14.9 | 47.4 | txn BVN1 age 982 | 0 | 0 | 0 | 995 |
| 14:48:15 | 300 | 2.5 | 21.4 | 48.5 | txn Directory age 1055 | 0 | 0 | 0 | 1055 |
