# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 7032 timed reads, p50 1.6 ms, p95 15.3 ms, p99 49.0 ms, **max 1276.2 ms** (chain read, Directory, entry 1839 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 22 | 1.4 | 2.3 | 2.3 |
| 100–1000 | 2510 | 1.4 | 12.7 | 431.0 |
| 1000–5000 | 4500 | 1.9 | 16.2 | 1276.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3516 | 0.8 | 6.2 | 1276.2 |
| txn | 3516 | 2.4 | 29.1 | 356.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 1276.2 | chain | Directory | 1839 |
| 431.0 | chain | BVN2 | 975 |
| 356.2 | txn | BVN2 | 1633 |
| 325.5 | txn | BVN2 | 805 |
| 268.0 | txn | Directory | 1539 |
| 267.3 | txn | BVN1 | 1645 |
| 139.2 | txn | Directory | 940 |
| 130.5 | txn | Directory | 1059 |
| 115.1 | txn | Directory | 1539 |
| 101.0 | txn | BVN2 | 1759 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 23:43:25 | 6 | 1.7 | 1.8 | 1.8 | txn BVN2 age 7 | 0 | 0 | 0 | 10 |
| 23:44:25 | 24 | 1.4 | 2.3 | 2.3 | txn BVN2 age 97 | 0 | 0 | 0 | 101 |
| 23:45:25 | 42 | 1.3 | 1.8 | 2.1 | txn BVN2 age 156 | 0 | 0 | 0 | 161 |
| 23:46:26 | 60 | 1.3 | 8.3 | 10.3 | txn Directory age 220 | 0 | 0 | 0 | 220 |
| 23:47:26 | 78 | 1.5 | 12.3 | 18.3 | txn BVN2 age 275 | 0 | 0 | 0 | 280 |
| 23:48:27 | 96 | 1.3 | 3.7 | 5.3 | txn BVN2 age 335 | 0 | 0 | 0 | 340 |
| 23:49:27 | 114 | 1.0 | 1.6 | 1.8 | txn BVN2 age 395 | 0 | 0 | 0 | 400 |
| 23:50:28 | 132 | 1.3 | 13.6 | 37.1 | txn Directory age 460 | 0 | 0 | 0 | 460 |
| 23:51:29 | 150 | 1.1 | 3.5 | 5.7 | txn Directory age 520 | 0 | 0 | 0 | 520 |
| 23:52:29 | 168 | 1.3 | 9.3 | 46.2 | txn BVN2 age 574 | 0 | 0 | 0 | 580 |
| 23:53:30 | 186 | 1.1 | 3.5 | 9.5 | txn BVN2 age 634 | 0 | 0 | 0 | 640 |
| 23:54:31 | 204 | 1.2 | 1.9 | 7.6 | txn BVN2 age 693 | 0 | 0 | 0 | 700 |
| 23:55:32 | 222 | 1.6 | 5.5 | 32.2 | txn BVN1 age 753 | 0 | 0 | 0 | 761 |
| 23:56:34 | 240 | 1.6 | 26.9 | 325.5 | txn BVN2 age 805 | 0 | 0 | 0 | 821 |
| 23:57:34 | 252 | 1.9 | 27.2 | 88.6 | txn Directory age 880 | 0 | 0 | 0 | 880 |
| 23:58:34 | 270 | 1.7 | 36.6 | 139.2 | txn Directory age 940 | 0 | 0 | 0 | 940 |
| 23:59:34 | 288 | 1.7 | 32.9 | 431.0 | chain BVN2 age 975 | 0 | 0 | 0 | 999 |
| 00:00:35 | 300 | 1.9 | 37.4 | 130.5 | txn Directory age 1059 | 0 | 0 | 0 | 1059 |
| 00:01:34 | 300 | 1.4 | 5.3 | 13.0 | chain BVN1 age 1110 | 0 | 0 | 0 | 1119 |
| 00:02:36 | 300 | 1.9 | 38.2 | 93.4 | txn Directory age 1179 | 0 | 0 | 0 | 1179 |
| 00:03:35 | 300 | 1.7 | 9.0 | 40.0 | txn BVN1 age 1220 | 0 | 0 | 0 | 1239 |
| 00:04:35 | 300 | 1.6 | 6.7 | 17.7 | txn BVN2 age 1274 | 0 | 0 | 0 | 1299 |
| 00:05:36 | 300 | 1.7 | 7.5 | 26.2 | txn BVN2 age 1329 | 0 | 0 | 0 | 1359 |
| 00:06:37 | 300 | 1.7 | 14.5 | 64.1 | txn Directory age 1419 | 0 | 0 | 0 | 1419 |
| 00:07:38 | 300 | 1.8 | 20.2 | 64.9 | txn Directory age 1479 | 0 | 0 | 0 | 1479 |
| 00:08:40 | 300 | 2.1 | 46.5 | 268.0 | txn Directory age 1539 | 0 | 0 | 0 | 1539 |
| 00:09:39 | 300 | 2.4 | 12.2 | 35.0 | txn BVN1 age 1566 | 0 | 0 | 0 | 1599 |
| 00:10:41 | 300 | 4.1 | 25.9 | 356.2 | txn BVN2 age 1633 | 0 | 0 | 0 | 1659 |
| 00:11:40 | 300 | 2.4 | 15.7 | 53.0 | txn BVN1 age 1702 | 0 | 0 | 0 | 1718 |
| 00:12:41 | 300 | 2.1 | 25.5 | 101.0 | txn BVN2 age 1759 | 0 | 0 | 0 | 1779 |
| 00:13:43 | 300 | 2.4 | 20.2 | 1276.2 | chain Directory age 1839 | 0 | 0 | 0 | 1839 |
| 00:14:41 | 300 | 1.8 | 7.4 | 28.1 | txn BVN2 age 1879 | 0 | 0 | 0 | 1899 |
