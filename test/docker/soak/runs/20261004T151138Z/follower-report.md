# Follower (#4365) — gate 0

One node with a validator's wiring and a key in no committee, launched with the network. What the run's log and its NetworkDefinition capture say about it.

| what | value |
|---|---|
| follower | `acc-bvn3-fol1`, partitions BVN3, Directory |
| follower key in the NetworkDefinition | inactive in the definition |
| active validators per partition, from the NetworkDefinition | BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| committee size per partition (validators, at genesis) | BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| committees that disagreed across nodes (#) | 0 |
| follower in no committee | yes |
| validators added to a committee during the run (#) | 0 |
| anchors the follower dispatched (#4367: must be 0) (#) | 0 |
| blocks the follower stated a root for without sending (#) | 4308 |
| anchored blocks compared, follower vs a validator (#) | 4299 |
| root/BPT mismatches (#) | 18 |
| first mismatching block | BVN3 block 1000: follower root eb0bce7a / bpt a7d3d571, `acc-bvn3-fol2` root 478976e7 / bpt 282c62eb |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-04T15:14:01Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |

## Every mismatching block

| partition | block | follower root/bpt | validator | its root/bpt |
|---|---|---|---|---|
| BVN3 | 1000 | root eb0bce7a / bpt a7d3d571 | `acc-bvn3-fol2` | root 478976e7 / bpt 282c62eb |
| BVN3 | 1001 | root b7da61ae / bpt 72821a44 | `acc-bvn3-fol2` | root a60382ae / bpt c44d2bf7 |
| BVN3 | 1002 | root 73f28d03 / bpt 82c170a6 | `acc-bvn3-fol2` | root ccd2a379 / bpt b71a2afa |
| BVN3 | 1003 | root f4814bf7 / bpt 7b07d13f | `acc-bvn3-fol2` | root b9e499d4 / bpt 3cb0613f |
| BVN3 | 1011 | root 14c0a6b4 / bpt de51d173 | `acc-bvn3-fol2` | root 78036d9b / bpt 9e1316bb |
| BVN3 | 1012 | root 3fdb34a6 / bpt ec58f751 | `acc-bvn3-fol2` | root 8bfa737d / bpt 3aceb8b0 |
| BVN3 | 1013 | root 7b905f1e / bpt 9fd7bf66 | `acc-bvn3-fol2` | root 98d51a08 / bpt 904b0896 |
| BVN3 | 1014 | root d4a9e359 / bpt a8f37153 | `acc-bvn3-fol2` | root 1e9b9129 / bpt ceb37fd7 |
| BVN3 | 1015 | root 8dc0d7f8 / bpt aaf35a56 | `acc-bvn3-fol2` | root ecc9f56d / bpt ff92d9ec |
| BVN3 | 1016 | root ca02f1d7 / bpt fb05ba75 | `acc-bvn3-fol2` | root eddd23ba / bpt ae3e8e0b |
| BVN3 | 1017 | root 8bc9172c / bpt 331c78ee | `acc-bvn3-fol2` | root c1182298 / bpt a1f4b367 |
| BVN3 | 1018 | root 9c83a3c9 / bpt 8e6bdb78 | `acc-bvn3-fol2` | root 6fcf2247 / bpt 5c8421c3 |
| BVN3 | 1019 | root d0f25dfe / bpt 187eb8d8 | `acc-bvn3-fol2` | root 5a19d413 / bpt 05a27d1c |
| BVN3 | 1020 | root 0dbcb98d / bpt f2da8dfc | `acc-bvn3-fol2` | root 3adc0095 / bpt 63189ddf |
| BVN3 | 1021 | root 60c3ac63 / bpt d95780f4 | `acc-bvn3-fol2` | root 094bd6c1 / bpt 6d497691 |
| BVN3 | 1022 | root 78db691f / bpt 56964bcb | `acc-bvn3-fol2` | root ed408071 / bpt fe59a8e1 |
| BVN3 | 1023 | root 17773cf1 / bpt f03b76d5 | `acc-bvn3-fol2` | root 23066644 / bpt c033ebce |
| BVN3 | 1024 | root a30dafca / bpt fb16d076 | `acc-bvn3-fol2` | root 80d1b4b7 / bpt 8df8087e |
