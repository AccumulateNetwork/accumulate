# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 7908 timed reads, p50 1.7 ms, p95 11.6 ms, p99 35.2 ms, **max 1220.2 ms** (txn read, BVN1, entry 861 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.4 | 2.8 | 4.1 |
| 100–1000 | 2478 | 1.3 | 8.8 | 1220.2 |
| 1000–5000 | 5400 | 1.8 | 12.7 | 700.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3954 | 0.8 | 4.6 | 424.3 |
| txn | 3954 | 2.2 | 17.4 | 1220.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 1220.2 | txn | BVN1 | 861 |
| 700.2 | txn | BVN1 | 1501 |
| 427.2 | txn | Directory | 1950 |
| 424.3 | chain | BVN2 | 562 |
| 256.1 | txn | BVN1 | 1501 |
| 238.7 | txn | BVN1 | 626 |
| 168.6 | txn | Directory | 874 |
| 142.9 | chain | Directory | 1293 |
| 139.4 | txn | BVN1 | 626 |
| 93.7 | txn | BVN2 | 618 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 00:52:32 | 6 | 1.8 | 2.0 | 2.0 | txn Directory age 10 | 0 | 0 | 0 | 10 |
| 00:53:33 | 24 | 1.4 | 2.8 | 4.1 | chain Directory age 96 | 0 | 0 | 0 | 96 |
| 00:54:33 | 42 | 1.2 | 2.0 | 8.3 | txn BVN2 age 152 | 0 | 0 | 0 | 156 |
| 00:55:34 | 60 | 1.1 | 1.6 | 1.8 | chain BVN2 age 211 | 0 | 0 | 0 | 216 |
| 00:56:34 | 78 | 1.2 | 2.2 | 7.6 | chain BVN1 age 270 | 0 | 0 | 0 | 276 |
| 00:57:34 | 96 | 1.2 | 1.8 | 4.2 | txn BVN2 age 329 | 0 | 0 | 0 | 336 |
| 00:58:35 | 114 | 1.4 | 6.1 | 16.1 | txn BVN1 age 389 | 0 | 0 | 0 | 395 |
| 00:59:35 | 132 | 1.1 | 1.7 | 3.4 | txn BVN2 age 448 | 0 | 0 | 0 | 455 |
| 01:00:36 | 150 | 0.9 | 1.5 | 2.4 | txn Directory age 515 | 0 | 0 | 0 | 515 |
| 01:01:37 | 168 | 1.2 | 5.7 | 424.3 | chain BVN2 age 562 | 0 | 0 | 0 | 573 |
| 01:02:37 | 180 | 1.5 | 30.7 | 238.7 | txn BVN1 age 626 | 0 | 0 | 0 | 633 |
| 01:03:37 | 198 | 1.4 | 7.7 | 54.5 | txn Directory age 694 | 0 | 0 | 0 | 694 |
| 01:04:38 | 216 | 1.6 | 7.1 | 18.2 | txn BVN1 age 745 | 0 | 0 | 0 | 755 |
| 01:05:39 | 234 | 1.9 | 12.1 | 64.0 | txn BVN1 age 802 | 0 | 0 | 0 | 815 |
| 01:06:41 | 252 | 1.9 | 35.3 | 1220.2 | txn BVN1 age 861 | 0 | 0 | 0 | 874 |
| 01:07:40 | 270 | 1.8 | 7.4 | 32.8 | txn BVN1 age 923 | 0 | 0 | 0 | 934 |
| 01:08:40 | 288 | 1.6 | 7.1 | 28.6 | txn BVN1 age 963 | 0 | 0 | 0 | 994 |
| 01:09:40 | 300 | 1.5 | 11.7 | 40.9 | txn BVN2 age 1034 | 0 | 0 | 0 | 1053 |
| 01:10:41 | 300 | 2.0 | 14.5 | 55.7 | chain BVN1 age 1100 | 0 | 0 | 0 | 1113 |
| 01:11:42 | 300 | 1.8 | 20.4 | 76.5 | txn BVN2 age 1159 | 0 | 0 | 0 | 1173 |
| 01:12:42 | 300 | 1.9 | 23.2 | 73.1 | txn Directory age 1233 | 0 | 0 | 0 | 1233 |
| 01:13:43 | 300 | 1.9 | 8.6 | 142.9 | chain Directory age 1293 | 0 | 0 | 0 | 1293 |
| 01:14:43 | 300 | 1.9 | 6.3 | 24.8 | chain BVN2 age 1337 | 0 | 0 | 0 | 1353 |
| 01:15:43 | 300 | 1.9 | 15.7 | 66.0 | txn Directory age 1412 | 0 | 0 | 0 | 1412 |
| 01:16:43 | 300 | 1.8 | 7.5 | 20.2 | txn BVN2 age 1451 | 0 | 0 | 0 | 1472 |
| 01:17:45 | 300 | 2.5 | 16.5 | 700.2 | txn BVN1 age 1501 | 0 | 0 | 0 | 1532 |
| 01:18:44 | 300 | 1.8 | 6.9 | 36.8 | txn Directory age 1592 | 0 | 0 | 0 | 1592 |
| 01:19:45 | 300 | 1.9 | 18.3 | 50.9 | chain BVN1 age 1625 | 0 | 0 | 0 | 1651 |
| 01:20:46 | 300 | 2.4 | 32.1 | 76.1 | txn BVN1 age 1686 | 0 | 0 | 0 | 1711 |
| 01:21:45 | 300 | 1.7 | 6.3 | 23.0 | txn Directory age 1771 | 0 | 0 | 0 | 1771 |
| 01:22:46 | 300 | 1.8 | 11.4 | 78.5 | txn BVN2 age 1806 | 0 | 0 | 0 | 1831 |
| 01:23:46 | 300 | 1.6 | 6.2 | 18.6 | txn BVN1 age 1842 | 0 | 0 | 0 | 1891 |
| 01:24:49 | 300 | 1.9 | 14.0 | 427.2 | txn Directory age 1950 | 0 | 0 | 0 | 1950 |
| 01:25:48 | 300 | 1.9 | 7.2 | 19.8 | txn BVN2 age 1993 | 0 | 0 | 0 | 2011 |
| 01:26:48 | 300 | 2.0 | 16.2 | 39.2 | txn Directory age 2071 | 0 | 0 | 0 | 2071 |
