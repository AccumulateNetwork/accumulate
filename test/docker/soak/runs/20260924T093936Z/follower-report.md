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
| blocks the follower stated a root for without sending (#) | 2845 |
| anchored blocks compared, follower vs a validator (#) | 2845 |
| root/BPT mismatches (#) | 0 |
| first mismatching block | none |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 1, at 2026-09-24T09:51:50Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
