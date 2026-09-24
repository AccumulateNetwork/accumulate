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
| blocks the follower stated a root for without sending (#) | 2420 |
| anchored blocks compared, follower vs a validator (#) | 2420 |
| root/BPT mismatches (#) | 16 |
| first mismatching block | Directory block 192: follower root dc09c513 / bpt 463fd311, `acc-bvn1-val1` root dc09c513 / bpt 64dab40f |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-09-24T11:19:37Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |

## Every mismatching block

| partition | block | follower root/bpt | validator | its root/bpt |
|---|---|---|---|---|
| Directory | 192 | root dc09c513 / bpt 463fd311 | `acc-bvn1-val1` | root dc09c513 / bpt 64dab40f |
| Directory | 193 | root 4a948c28 / bpt c1848a75 | `acc-bvn1-val1` | root 6360c8a9 / bpt 6ae3eb4d |
| Directory | 223 | root 128fd80f / bpt 5093b70a | `acc-bvn1-val1` | root 128fd80f / bpt 8496f8d5 |
| Directory | 224 | root 76a42984 / bpt 5f75b9ef | `acc-bvn1-val1` | root 4352f217 / bpt 499069ff |
| Directory | 225 | root 67d175cb / bpt 662c93db | `acc-bvn1-val1` | root fa782128 / bpt cffe8bc2 |
| Directory | 226 | root 8c14040c / bpt 4ab333fe | `acc-bvn1-val1` | root c59b3463 / bpt 39779d64 |
| Directory | 426 | root 124b9f85 / bpt dcc7d0aa | `acc-bvn1-val1` | root 124b9f85 / bpt 3d855723 |
| Directory | 427 | root b287e020 / bpt 568a38b1 | `acc-bvn1-val1` | root b9d0e04d / bpt 8dbc2530 |
| Directory | 428 | root bc1c5763 / bpt e28b5f56 | `acc-bvn1-val1` | root 8e824f1e / bpt 8bbf423a |
| Directory | 433 | root 3d952078 / bpt d6a3e251 | `acc-bvn1-val1` | root 25f4fd68 / bpt 50d79cb6 |
| Directory | 434 | root 178ff11e / bpt 264ccf36 | `acc-bvn1-val1` | root a647b01a / bpt 61cf6f8c |
| Directory | 435 | root 86b6de93 / bpt 6bfdba98 | `acc-bvn1-val1` | root e5726ae3 / bpt da94caf4 |
| Directory | 436 | root 3daf117c / bpt be54ec65 | `acc-bvn1-val1` | root 1f759c1e / bpt 9a8ed09d |
| Directory | 479 | root 6ac6d1e0 / bpt b61c78a5 | `acc-bvn1-val1` | root 6bb1af59 / bpt 2cc5682e |
| Directory | 480 | root 1ac2b257 / bpt 34f6badf | `acc-bvn1-val1` | root 2dc49d59 / bpt 8481da74 |
| Directory | 481 | root 22a8b579 / bpt 380287e9 | `acc-bvn1-val1` | root ef278acf / bpt 40544b09 |
