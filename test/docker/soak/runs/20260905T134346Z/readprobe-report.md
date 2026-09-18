# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2844 timed reads, p50 1.8 ms, p95 8.6 ms, p99 21.6 ms, **max 94.5 ms** (txn read, BVN1, entry 981 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.5 | 3.4 | 3.9 |
| 100–1000 | 2514 | 1.7 | 8.2 | 94.5 |
| 1000–5000 | 300 | 2.3 | 14.4 | 50.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1422 | 0.8 | 3.8 | 26.6 |
| txn | 1422 | 2.6 | 13.5 | 94.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 94.5 | txn | BVN1 | 981 |
| 72.5 | txn | Directory | 874 |
| 63.5 | txn | Directory | 993 |
| 50.2 | txn | BVN1 | 1038 |
| 50.0 | txn | BVN2 | 985 |
| 48.5 | txn | BVN1 | 809 |
| 43.0 | txn | BVN2 | 802 |
| 40.7 | txn | BVN2 | 802 |
| 35.0 | txn | BVN1 | 1038 |
| 33.1 | txn | BVN2 | 859 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 13:47:12 | 6 | 1.5 | 1.7 | 1.7 | txn Directory age 15 | 0 | 0 | 0 | 16 |
| 13:48:12 | 24 | 1.9 | 3.4 | 3.9 | txn BVN1 age 93 | 0 | 0 | 0 | 96 |
| 13:49:13 | 42 | 1.9 | 14.1 | 26.1 | txn Directory age 156 | 0 | 0 | 0 | 156 |
| 13:50:13 | 60 | 1.6 | 13.5 | 17.0 | txn BVN1 age 212 | 0 | 0 | 0 | 216 |
| 13:51:14 | 78 | 1.9 | 10.2 | 20.3 | txn BVN1 age 271 | 0 | 0 | 0 | 275 |
| 13:52:14 | 96 | 1.0 | 2.1 | 7.3 | txn Directory age 335 | 0 | 0 | 0 | 335 |
| 13:53:15 | 114 | 1.4 | 3.7 | 9.8 | txn BVN1 age 390 | 0 | 0 | 0 | 395 |
| 13:54:15 | 132 | 1.5 | 4.9 | 9.1 | txn Directory age 455 | 0 | 0 | 0 | 455 |
| 13:55:16 | 150 | 1.2 | 2.1 | 9.3 | txn BVN2 age 510 | 0 | 0 | 0 | 514 |
| 13:56:16 | 168 | 1.6 | 5.4 | 13.8 | txn Directory age 574 | 0 | 0 | 0 | 574 |
| 13:57:17 | 186 | 1.9 | 7.3 | 23.8 | txn Directory age 634 | 0 | 0 | 0 | 634 |
| 13:58:18 | 204 | 1.9 | 5.7 | 26.0 | txn BVN2 age 687 | 0 | 0 | 0 | 694 |
| 13:59:19 | 222 | 2.1 | 9.2 | 17.7 | txn BVN1 age 749 | 0 | 0 | 0 | 754 |
| 14:00:20 | 240 | 2.1 | 11.1 | 48.5 | txn BVN1 age 809 | 0 | 0 | 0 | 814 |
| 14:01:21 | 258 | 2.2 | 12.1 | 72.5 | txn Directory age 874 | 0 | 0 | 0 | 874 |
| 14:02:21 | 276 | 2.2 | 7.2 | 19.1 | txn BVN1 age 920 | 0 | 0 | 0 | 933 |
| 14:03:21 | 288 | 2.6 | 14.1 | 94.5 | txn BVN1 age 981 | 0 | 0 | 0 | 993 |
| 14:04:22 | 300 | 2.3 | 14.4 | 50.2 | txn BVN1 age 1038 | 0 | 0 | 0 | 1053 |
