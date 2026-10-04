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
| blocks the follower stated a root for without sending (#) | 7151 |
| anchored blocks compared, follower vs a validator (#) | 7124 |
| root/BPT mismatches (#) | 11 |
| first mismatching block | BVN3 block 2017: follower root 420cbd65 / bpt 3bdfe9e6, `acc-bvn3-fol2` root 5c5c732a / bpt adc9781c |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-04T17:31:11Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |

## Every mismatching block

| partition | block | follower root/bpt | validator | its root/bpt |
|---|---|---|---|---|
| BVN3 | 2017 | root 420cbd65 / bpt 3bdfe9e6 | `acc-bvn3-fol2` | root 5c5c732a / bpt adc9781c |
| BVN3 | 2018 | root e3b610d3 / bpt bae9f2e5 | `acc-bvn3-fol2` | root 8da5a3b8 / bpt 19a3caec |
| BVN3 | 2019 | root ff73ca07 / bpt 5a3e2e5b | `acc-bvn3-fol2` | root c1a2c8a1 / bpt 58b47860 |
| BVN3 | 2020 | root 1181f2be / bpt 9f35383f | `acc-bvn3-fol2` | root 84be9cae / bpt 94a73e86 |
| BVN3 | 2021 | root 333eadeb / bpt 24edb311 | `acc-bvn3-fol2` | root 656a9d8a / bpt 8f657db0 |
| BVN3 | 3340 | root 0ad54859 / bpt 6a5c994f | `acc-bvn3-fol2` | root 7237295f / bpt bb71e9c3 |
| BVN3 | 3341 | root 71f041fd / bpt 13160064 | `acc-bvn3-fol2` | root 41deb49d / bpt eea924b9 |
| BVN3 | 3342 | root 87875723 / bpt 567f1e67 | `acc-bvn3-fol2` | root a61a458d / bpt 742c4e9e |
| BVN3 | 3367 | root 62112bbe / bpt 47f59b3a | `acc-bvn3-fol2` | root 55928b11 / bpt 19034056 |
| BVN3 | 3368 | root 4418910e / bpt 465a5f2f | `acc-bvn3-fol2` | root fc829b34 / bpt 207739cc |
| BVN3 | 3369 | root 80313fb0 / bpt 6df041d9 | `acc-bvn3-fol2` | root cc919405 / bpt 8dea9dcf |
