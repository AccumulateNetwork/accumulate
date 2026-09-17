# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 4950 timed reads, p50 1.5 ms, p95 9.6 ms, p99 41.0 ms, **max 1156.2 ms** (txn read, BVN2, entry 846 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.3 | 2.7 | 4.2 |
| 100–1000 | 2520 | 1.2 | 8.5 | 1156.2 |
| 1000–5000 | 2400 | 1.7 | 10.0 | 74.6 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2475 | 0.8 | 4.3 | 227.5 |
| txn | 2475 | 1.8 | 14.8 | 1156.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 1156.2 | txn | BVN2 | 846 |
| 606.0 | txn | BVN1 | 857 |
| 451.8 | txn | BVN1 | 857 |
| 417.2 | txn | BVN2 | 846 |
| 411.0 | txn | BVN2 | 846 |
| 318.5 | txn | Directory | 874 |
| 305.7 | txn | BVN1 | 857 |
| 276.0 | txn | BVN1 | 857 |
| 227.5 | chain | BVN2 | 846 |
| 216.8 | txn | BVN2 | 846 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 03:26:40 | 6 | 1.7 | 2.7 | 2.7 | txn BVN2 age 7 | 0 | 0 | 0 | 10 |
| 03:27:40 | 24 | 1.3 | 2.7 | 4.2 | txn Directory age 96 | 0 | 0 | 0 | 96 |
| 03:28:40 | 42 | 1.1 | 2.1 | 2.4 | txn Directory age 156 | 0 | 0 | 0 | 156 |
| 03:29:41 | 60 | 1.0 | 1.3 | 1.7 | txn BVN2 age 211 | 0 | 0 | 0 | 216 |
| 03:30:41 | 78 | 1.2 | 2.0 | 9.6 | chain Directory age 275 | 0 | 0 | 0 | 275 |
| 03:31:41 | 96 | 1.0 | 1.6 | 20.2 | txn Directory age 335 | 0 | 0 | 0 | 335 |
| 03:32:42 | 114 | 0.9 | 1.6 | 6.4 | txn BVN1 age 386 | 0 | 0 | 0 | 395 |
| 03:33:42 | 132 | 1.3 | 4.5 | 34.4 | txn Directory age 455 | 0 | 0 | 0 | 455 |
| 03:34:43 | 150 | 1.3 | 4.7 | 41.0 | txn BVN2 age 502 | 0 | 0 | 0 | 515 |
| 03:35:44 | 168 | 1.0 | 1.5 | 7.3 | txn BVN1 age 564 | 0 | 0 | 0 | 575 |
| 03:36:44 | 186 | 1.1 | 1.6 | 15.7 | txn BVN2 age 627 | 0 | 0 | 0 | 635 |
| 03:37:45 | 204 | 1.5 | 4.4 | 17.0 | txn Directory age 695 | 0 | 0 | 0 | 695 |
| 03:38:46 | 222 | 1.5 | 4.6 | 11.6 | txn Directory age 755 | 0 | 0 | 0 | 755 |
| 03:39:47 | 240 | 1.4 | 15.9 | 72.9 | txn Directory age 815 | 0 | 0 | 0 | 815 |
| 03:40:54 | 258 | 1.8 | 138.9 | 1156.2 | txn BVN2 age 846 | 0 | 0 | 0 | 874 |
| 03:41:48 | 276 | 1.6 | 15.3 | 194.9 | txn Directory age 934 | 0 | 0 | 0 | 934 |
| 03:42:48 | 294 | 1.3 | 3.3 | 11.4 | txn BVN1 age 978 | 0 | 0 | 0 | 995 |
| 03:43:49 | 300 | 1.5 | 5.3 | 74.6 | txn BVN1 age 1037 | 0 | 0 | 0 | 1055 |
| 03:44:49 | 300 | 1.5 | 6.5 | 15.9 | txn BVN2 age 1099 | 0 | 0 | 0 | 1114 |
| 03:45:51 | 300 | 4.5 | 26.6 | 64.1 | txn BVN1 age 1142 | 0 | 0 | 0 | 1174 |
| 03:46:50 | 300 | 1.6 | 6.9 | 41.6 | txn Directory age 1234 | 0 | 0 | 0 | 1234 |
| 03:47:51 | 300 | 1.7 | 6.6 | 20.3 | txn BVN2 age 1273 | 0 | 0 | 0 | 1294 |
| 03:48:52 | 300 | 2.0 | 7.4 | 20.7 | txn BVN1 age 1328 | 0 | 0 | 0 | 1354 |
| 03:49:53 | 300 | 1.6 | 5.7 | 18.2 | txn Directory age 1414 | 0 | 0 | 0 | 1414 |
| 03:50:54 | 300 | 2.4 | 11.4 | 46.6 | txn BVN2 age 1452 | 0 | 0 | 0 | 1474 |
