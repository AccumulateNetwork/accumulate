# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 7928 timed reads, p50 1.5 ms, p95 3.2 ms, p99 10.1 ms, **max 8041.2 ms** (txn read, BVN1, entry 1229 blocks old); 215 failed, 28 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.4 | 2.8 | 2.8 |
| 100–1000 | 3088 | 1.5 | 3.1 | 8040.1 |
| 1000–5000 | 4800 | 1.5 | 3.3 | 8041.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3964 | 0.7 | 1.6 | 8040.1 |
| txn | 3964 | 2.0 | 4.1 | 8041.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8041.2 | txn | BVN1 | 1229 |
| 8040.4 | txn | BVN3 | 1274 |
| 8040.3 | txn | BVN2 | 1308 |
| 8040.3 | txn | BVN3 | 1961 |
| 8040.2 | txn | Directory | 2008 |
| 8040.2 | txn | BVN1 | 2035 |
| 8040.2 | txn | BVN2 | 1308 |
| 8040.1 | txn | BVN1 | 1229 |
| 8040.1 | chain | BVN2 | 511 |
| 8040.0 | txn | BVN2 | 1308 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 20:34:43 | 8 | 2.4 | 2.8 | 2.8 | txn BVN1 age 35 | 0 | 0 | 0 | 38 |
| 20:35:44 | 32 | 1.4 | 2.0 | 2.3 | txn BVN2 age 94 | 0 | 0 | 0 | 98 |
| 20:36:44 | 56 | 1.4 | 3.3 | 3.7 | txn Directory age 158 | 0 | 0 | 0 | 158 |
| 20:37:44 | 80 | 1.5 | 3.9 | 10.1 | txn Directory age 217 | 0 | 0 | 0 | 217 |
| 20:38:45 | 104 | 1.4 | 5.1 | 13.8 | txn BVN1 age 273 | 0 | 0 | 0 | 277 |
| 20:39:46 | 128 | 1.5 | 2.6 | 7.8 | chain BVN1 age 332 | 0 | 0 | 0 | 337 |
| 20:40:46 | 152 | 1.6 | 5.4 | 12.5 | txn BVN2 age 392 | 0 | 0 | 0 | 397 |
| 20:41:47 | 176 | 1.4 | 2.6 | 3.2 | txn BVN3 age 452 | 0 | 0 | 0 | 457 |
| 20:42:47 | 200 | 1.5 | 3.2 | 9.5 | txn BVN3 age 512 | 0 | 0 | 0 | 517 |
| 20:44:42 | 224 | 1.6 | 17.9 | 8040.1 | chain BVN2 age 511 | 5 | 0 | 5 | 577 |
| 20:44:48 | 232 | 1.6 | 3.9 | 7.1 | txn BVN1 age 630 | 0 | 0 | 0 | 631 |
| 20:45:49 | 256 | 1.5 | 3.1 | 11.7 | chain BVN3 age 663 | 0 | 0 | 0 | 691 |
| 20:46:50 | 280 | 1.4 | 2.7 | 11.9 | txn BVN1 age 749 | 0 | 0 | 0 | 751 |
| 20:47:50 | 300 | 1.4 | 2.5 | 3.6 | txn BVN3 age 783 | 0 | 0 | 0 | 811 |
| 20:48:51 | 300 | 1.6 | 3.1 | 13.0 | chain BVN2 age 870 | 0 | 0 | 0 | 871 |
| 20:49:52 | 300 | 1.5 | 2.6 | 6.1 | chain BVN2 age 930 | 0 | 0 | 0 | 931 |
| 20:50:53 | 300 | 1.4 | 2.7 | 9.9 | txn Directory age 992 | 0 | 0 | 0 | 992 |
| 20:51:54 | 300 | 1.5 | 3.2 | 10.3 | chain BVN3 age 1023 | 0 | 0 | 0 | 1052 |
| 20:52:54 | 300 | 1.4 | 2.8 | 15.6 | chain BVN3 age 1082 | 0 | 0 | 0 | 1112 |
| 20:53:55 | 300 | 1.6 | 2.9 | 14.0 | chain BVN2 age 1169 | 0 | 0 | 0 | 1172 |
| 20:54:56 | 300 | 1.5 | 2.6 | 10.6 | txn Directory age 1232 | 0 | 0 | 0 | 1232 |
| 20:58:17 | 300 | 1.8 | 4929.5 | 8041.2 | txn BVN1 age 1229 | 14 | 0 | 14 | 1308 |
| 20:58:18 | 300 | 1.6 | 3.7 | 23.4 | txn BVN3 age 1262 | 0 | 0 | 0 | 1428 |
| 20:59:19 | 300 | 1.4 | 2.2 | 9.6 | txn BVN2 age 1487 | 0 | 0 | 0 | 1488 |
| 21:00:20 | 300 | 1.5 | 2.4 | 21.0 | chain BVN3 age 1474 | 0 | 0 | 0 | 1548 |
| 21:01:21 | 300 | 1.7 | 2.5 | 30.2 | txn BVN3 age 1534 | 0 | 0 | 0 | 1607 |
| 21:02:21 | 300 | 1.6 | 3.5 | 17.7 | txn BVN1 age 1667 | 0 | 0 | 0 | 1667 |
| 21:03:22 | 300 | 1.4 | 2.3 | 6.8 | txn BVN1 age 1727 | 0 | 0 | 0 | 1727 |
| 21:04:23 | 300 | 1.6 | 2.9 | 11.0 | txn BVN3 age 1713 | 0 | 0 | 0 | 1787 |
| 21:05:24 | 300 | 1.5 | 2.2 | 7.0 | txn Directory age 1824 | 0 | 0 | 0 | 1847 |
| 21:06:25 | 300 | 1.7 | 2.8 | 28.0 | txn BVN1 age 1907 | 0 | 0 | 0 | 1907 |
| 21:07:25 | 300 | 1.7 | 5.0 | 19.7 | txn Directory age 1944 | 0 | 0 | 0 | 1967 |
| 21:09:52 | 300 | 0.3 | 4.7 | 8040.3 | txn BVN3 age 1961 | 196 | 0 | 9 | 2035 |
