# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 8250 timed reads, p50 2.2 ms, p95 11.1 ms, p99 26.7 ms, **max 214.5 ms** (txn read, BVN1, entry 1446 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.8 | 3.3 | 9.2 |
| 100–1000 | 2618 | 1.7 | 5.2 | 37.4 |
| 1000–5000 | 5602 | 2.7 | 13.5 | 214.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 4125 | 1.0 | 5.0 | 45.0 |
| txn | 4125 | 3.3 | 15.3 | 214.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 214.5 | txn | BVN1 | 1446 |
| 148.3 | txn | BVN2 | 1223 |
| 94.9 | txn | BVN2 | 1186 |
| 91.4 | txn | BVN1 | 1446 |
| 86.9 | txn | BVN2 | 1223 |
| 84.1 | txn | Directory | 1770 |
| 77.4 | txn | BVN2 | 1237 |
| 75.1 | txn | BVN1 | 1446 |
| 68.4 | txn | Directory | 1770 |
| 62.3 | txn | Directory | 1530 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 22:31:10 | 6 | 1.9 | 2.0 | 2.0 | txn Directory age 15 | 0 | 0 | 0 | 15 |
| 22:32:10 | 24 | 1.8 | 3.3 | 9.2 | txn BVN1 age 89 | 0 | 0 | 0 | 93 |
| 22:33:11 | 42 | 1.1 | 1.7 | 2.9 | txn BVN1 age 149 | 0 | 0 | 0 | 153 |
| 22:34:11 | 60 | 1.9 | 5.7 | 11.5 | txn Directory age 213 | 0 | 0 | 0 | 213 |
| 22:35:12 | 78 | 1.1 | 1.8 | 1.9 | txn BVN2 age 268 | 0 | 0 | 0 | 273 |
| 22:36:12 | 96 | 1.4 | 6.9 | 10.9 | txn BVN1 age 328 | 0 | 0 | 0 | 332 |
| 22:37:13 | 114 | 1.6 | 4.6 | 11.7 | chain Directory age 392 | 0 | 0 | 0 | 392 |
| 22:38:13 | 132 | 1.6 | 5.6 | 15.0 | txn Directory age 452 | 0 | 0 | 0 | 452 |
| 22:39:14 | 150 | 1.9 | 6.1 | 19.1 | txn BVN1 age 506 | 0 | 0 | 0 | 512 |
| 22:40:14 | 168 | 1.7 | 4.4 | 13.5 | txn BVN1 age 566 | 0 | 0 | 0 | 572 |
| 22:41:15 | 186 | 1.6 | 3.4 | 12.9 | chain BVN1 age 625 | 0 | 0 | 0 | 632 |
| 22:42:16 | 204 | 1.7 | 4.1 | 7.0 | chain BVN2 age 681 | 0 | 0 | 0 | 692 |
| 22:43:17 | 222 | 1.9 | 4.6 | 9.5 | chain BVN2 age 740 | 0 | 0 | 0 | 752 |
| 22:44:17 | 240 | 2.2 | 5.2 | 9.2 | txn BVN1 age 802 | 0 | 0 | 0 | 813 |
| 22:45:18 | 258 | 1.7 | 5.0 | 10.0 | txn BVN1 age 865 | 0 | 0 | 0 | 873 |
| 22:46:19 | 276 | 1.8 | 5.7 | 30.1 | txn BVN1 age 921 | 0 | 0 | 0 | 933 |
| 22:47:20 | 294 | 2.4 | 7.6 | 37.4 | txn BVN2 age 943 | 0 | 0 | 0 | 994 |
| 22:48:21 | 300 | 2.4 | 5.9 | 16.3 | txn BVN2 age 988 | 0 | 0 | 0 | 1053 |
| 22:49:21 | 300 | 2.6 | 10.0 | 35.5 | txn BVN2 age 1012 | 0 | 0 | 0 | 1113 |
| 22:50:21 | 300 | 2.3 | 9.6 | 35.5 | txn Directory age 1172 | 0 | 0 | 0 | 1172 |
| 22:51:21 | 300 | 2.1 | 7.8 | 37.7 | txn BVN2 age 1063 | 0 | 0 | 0 | 1232 |
| 22:52:22 | 300 | 2.8 | 9.7 | 35.5 | txn BVN1 age 1206 | 0 | 0 | 0 | 1292 |
| 22:53:22 | 300 | 3.0 | 11.9 | 51.2 | txn BVN1 age 1238 | 0 | 0 | 0 | 1351 |
| 22:54:23 | 300 | 2.6 | 7.8 | 46.6 | txn BVN2 age 1137 | 0 | 0 | 0 | 1410 |
| 22:55:23 | 300 | 2.9 | 13.3 | 43.6 | txn BVN2 age 1155 | 0 | 0 | 0 | 1470 |
| 22:56:24 | 300 | 2.8 | 14.4 | 62.3 | txn Directory age 1530 | 0 | 0 | 0 | 1530 |
| 22:57:25 | 300 | 2.7 | 11.0 | 94.9 | txn BVN2 age 1186 | 0 | 0 | 0 | 1590 |
| 22:58:26 | 300 | 3.4 | 18.3 | 44.5 | txn BVN2 age 1200 | 0 | 0 | 0 | 1650 |
| 22:59:26 | 300 | 2.9 | 8.3 | 51.2 | txn Directory age 1711 | 0 | 0 | 0 | 1711 |
| 23:00:29 | 300 | 4.5 | 37.8 | 214.5 | txn BVN1 age 1446 | 0 | 0 | 0 | 1770 |
| 23:01:27 | 300 | 3.7 | 15.0 | 42.5 | txn BVN1 age 1467 | 0 | 0 | 0 | 1831 |
| 23:02:28 | 300 | 3.0 | 9.4 | 18.2 | txn BVN2 age 1237 | 0 | 0 | 0 | 1890 |
| 23:03:29 | 300 | 3.8 | 14.4 | 45.7 | txn BVN2 age 1237 | 0 | 0 | 0 | 1950 |
| 23:04:29 | 300 | 1.9 | 3.8 | 11.6 | txn Directory age 2010 | 0 | 0 | 0 | 2010 |
| 23:05:31 | 300 | 3.8 | 25.7 | 77.4 | txn BVN2 age 1237 | 0 | 0 | 0 | 2068 |
| 23:06:31 | 300 | 2.6 | 12.9 | 48.9 | txn Directory age 2130 | 0 | 0 | 0 | 2130 |
