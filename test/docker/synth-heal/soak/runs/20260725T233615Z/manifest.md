# Soak run 20260725T233615Z

**Purpose:** #4070 hold-for-anchor on the reconciled release lineage at v2-jiuquan, conductor synthetic+anchor healing

| field | value |
|---|---|
| started (UTC) | 2026-07-25T23:36:15Z |
| commit | `1a179c10fdfca6bc5c90779cfeca9479d4eeb61c` |
| describe | `v1.4.1-snapshot-39-g1a179c10f` |
| branch | `merge/release-1.4.4.2-into-main` |
| uncommitted files | 0  |
| executor version | **v2-jiuquan** |
| healing | enable-anchor-healing = true;enable-synthetic-healing = true |
| synthetic drops | `*:%499+3` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 24h |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result — STOPPED EARLY (not a completed run)

Stopped by operator request at ~4.96h of a 24h target, to free the machine for
the v1.4.5 release build. **This is not a pass**: it is 21% of the intended
duration and must not be cited as validation of #4070.

| field | value |
|---|---|
| ended (UTC) | 2026-07-26T04:36Z |
| elapsed | 4.96h of 24h target |
| stopped by | operator, to build the release |
| generated | 35,416 / 172,800 (rejected 310) |
| dn height | 7 -> 12,230 |
| synthetic heals | 407 |
| anchor heals | 495 |
| stuck / errors | 0 / 0 |
| worst stream gap | 0 (all streams `recv == deliv`) |
| chaos events | 25 (13 restart, 9 pause, 3 skip) |
| seizure | none detected |

What it does support: ~5h at v2-jiuquan with conductor synthetic + anchor
healing, 25 chaos events, no wedge and no stream gap at any point observed.

Caveats on the observation window: soakmon and seizewatch were started ~4h in,
and until then the flow matrix was empty because this lineage does not emit
accumulate_crosschain_sequence — so gap/wedge detection only covers the final
~1h. The rejection count is ~97% fail:overburn-credits, an artifact removed in
a048e0035 but present in this run's binary. Snapshot: final-soakmon.json.
