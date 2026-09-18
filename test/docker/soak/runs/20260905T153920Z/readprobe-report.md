# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 8814 timed reads, p50 2.4 ms, p95 27.6 ms, p99 80.1 ms, **max 1889.4 ms** (txn read, Directory, entry 2016 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.1 | 2.3 | 2.8 |
| 100–1000 | 2484 | 1.8 | 24.9 | 238.3 |
| 1000–5000 | 6300 | 2.7 | 28.9 | 1889.4 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 4407 | 0.9 | 8.4 | 236.9 |
| txn | 4407 | 4.1 | 45.9 | 1889.4 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 1889.4 | txn | Directory | 2016 |
| 1187.2 | txn | BVN1 | 1161 |
| 322.9 | txn | BVN2 | 1043 |
| 305.0 | txn | Directory | 1236 |
| 268.2 | txn | BVN2 | 1989 |
| 262.8 | txn | BVN2 | 1269 |
| 261.1 | txn | BVN1 | 2000 |
| 238.3 | txn | BVN2 | 915 |
| 236.9 | chain | BVN1 | 2000 |
| 219.8 | txn | Directory | 1056 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 15:43:30 | 6 | 2.2 | 2.8 | 2.8 | txn BVN1 age 7 | 0 | 0 | 0 | 11 |
| 15:44:30 | 24 | 1.1 | 1.9 | 2.1 | txn BVN1 age 94 | 0 | 0 | 0 | 98 |
| 15:45:31 | 42 | 1.6 | 3.8 | 5.8 | txn BVN2 age 153 | 0 | 0 | 0 | 158 |
| 15:46:31 | 60 | 1.2 | 3.4 | 5.2 | txn BVN2 age 213 | 0 | 0 | 0 | 218 |
| 15:47:32 | 78 | 1.3 | 2.8 | 3.3 | txn BVN1 age 272 | 0 | 0 | 0 | 277 |
| 15:48:32 | 96 | 1.3 | 3.3 | 8.7 | txn BVN2 age 332 | 0 | 0 | 0 | 337 |
| 15:49:33 | 114 | 1.6 | 3.6 | 8.4 | txn BVN1 age 391 | 0 | 0 | 0 | 397 |
| 15:50:33 | 132 | 1.6 | 8.4 | 21.2 | txn Directory age 457 | 0 | 0 | 0 | 457 |
| 15:51:34 | 150 | 1.7 | 7.8 | 38.4 | txn BVN1 age 510 | 0 | 0 | 0 | 517 |
| 15:52:35 | 168 | 1.9 | 9.3 | 54.3 | txn Directory age 577 | 0 | 0 | 0 | 577 |
| 15:53:36 | 186 | 1.6 | 16.2 | 92.8 | txn BVN2 age 625 | 0 | 0 | 0 | 637 |
| 15:54:37 | 198 | 2.4 | 33.2 | 101.7 | txn BVN2 age 682 | 0 | 0 | 0 | 697 |
| 15:55:36 | 216 | 1.7 | 5.8 | 13.9 | txn Directory age 757 | 0 | 0 | 0 | 757 |
| 15:56:38 | 234 | 2.7 | 11.2 | 74.1 | txn BVN1 age 802 | 0 | 0 | 0 | 817 |
| 15:57:40 | 252 | 4.1 | 51.3 | 155.9 | txn BVN2 age 856 | 0 | 0 | 0 | 877 |
| 15:58:43 | 270 | 6.3 | 80.8 | 238.3 | txn BVN2 age 915 | 0 | 0 | 0 | 937 |
| 15:59:39 | 288 | 2.4 | 9.9 | 55.7 | txn BVN1 age 982 | 0 | 0 | 0 | 997 |
| 16:00:44 | 300 | 4.9 | 92.3 | 322.9 | txn BVN2 age 1043 | 0 | 0 | 0 | 1056 |
| 16:01:41 | 300 | 3.0 | 31.9 | 99.0 | txn Directory age 1116 | 0 | 0 | 0 | 1116 |
| 16:02:44 | 300 | 3.6 | 45.9 | 1187.2 | txn BVN1 age 1161 | 0 | 0 | 0 | 1176 |
| 16:03:43 | 300 | 2.8 | 39.7 | 305.0 | txn Directory age 1236 | 0 | 0 | 0 | 1236 |
| 16:04:45 | 300 | 3.9 | 52.6 | 262.8 | txn BVN2 age 1269 | 0 | 0 | 0 | 1297 |
| 16:05:43 | 300 | 3.1 | 20.7 | 59.3 | txn BVN1 age 1334 | 0 | 0 | 0 | 1356 |
| 16:06:43 | 300 | 2.2 | 11.8 | 36.6 | txn BVN2 age 1391 | 0 | 0 | 0 | 1416 |
| 16:07:44 | 300 | 2.5 | 17.1 | 59.3 | txn Directory age 1477 | 0 | 0 | 0 | 1477 |
| 16:08:44 | 300 | 2.2 | 11.4 | 37.9 | txn Directory age 1537 | 0 | 0 | 0 | 1537 |
| 16:09:45 | 300 | 2.1 | 10.2 | 34.1 | txn BVN2 age 1567 | 0 | 0 | 0 | 1597 |
| 16:10:45 | 300 | 2.6 | 13.6 | 59.0 | txn Directory age 1656 | 0 | 0 | 0 | 1656 |
| 16:11:46 | 300 | 2.7 | 10.8 | 45.1 | txn BVN1 age 1670 | 0 | 0 | 0 | 1717 |
| 16:12:50 | 300 | 6.6 | 66.2 | 153.8 | txn Directory age 1777 | 0 | 0 | 0 | 1777 |
| 16:13:48 | 300 | 2.8 | 31.3 | 89.9 | txn Directory age 1836 | 0 | 0 | 0 | 1836 |
| 16:14:47 | 300 | 2.4 | 11.2 | 81.1 | txn Directory age 1896 | 0 | 0 | 0 | 1896 |
| 16:15:48 | 300 | 2.7 | 16.1 | 72.9 | txn BVN1 age 1934 | 0 | 0 | 0 | 1956 |
| 16:16:55 | 300 | 5.4 | 99.6 | 1889.4 | txn Directory age 2016 | 0 | 0 | 0 | 2016 |
| 16:17:49 | 300 | 2.3 | 12.1 | 40.0 | txn BVN1 age 2050 | 0 | 0 | 0 | 2076 |
| 16:18:49 | 300 | 2.7 | 13.2 | 143.3 | txn BVN2 age 2106 | 0 | 0 | 0 | 2136 |
| 16:19:50 | 300 | 2.2 | 8.5 | 14.6 | txn Directory age 2196 | 0 | 0 | 0 | 2196 |
| 16:20:50 | 300 | 2.5 | 10.7 | 115.3 | txn BVN2 age 2225 | 0 | 0 | 0 | 2256 |
