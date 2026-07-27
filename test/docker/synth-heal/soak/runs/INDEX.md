# Soak runs

Every run appends one row. Details in `<runId>/manifest.md`.

| run | commit | executor | healing | elapsed | exit | dn height | heals | note |
|---|---|---|---|---|---|---|---|---|
| [20260725T233615Z](20260725T233615Z/manifest.md) | `v1.4.4.2-20-ga048e0035` | v2-jiuquan | synthetic+anchor | 4.96h of 24h | STOPPED | 7→12230 | 407 syn / 495 anc | stopped early for the v1.4.5 build; no wedge in ~5h |
| [20260727T053231Z](20260727T053231Z/manifest.md) | `v1.4.5-2-gd5e5be6c2` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 9→297 | 0→3142 | #4073 proof A: 5m with the interval reconcile |
| [20260727T054232Z](20260727T054232Z/manifest.md) | `v1.4.5-2-gd5e5be6c2-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 28→244 | 0→12 | #4073 proof B: no fix, DN->BVN1 seq 1-2 dropped, major blocks every minute |
| [20260727T055020Z](20260727T055020Z/manifest.md) | `v1.4.5-2-gd5e5be6c2-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.11h | 0 | 7→273 | 0→97 | #4073 proof B2: no fix, tail of DN->BVN1 dropped |
| [20260727T055822Z](20260727T055822Z/manifest.md) | `v1.4.5-2-gd5e5be6c2-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.1h | 0 | 28→275 | 0→6669 | #4073 proof C: WITH fix, same drops as B2 |

## #4073 short-run series, 2026-07-27

Four 5-minute runs attempting a system-level reproduction of the lost-prefix
stall. **None reproduced it, and that is a property of the bug, not a gap in
effort** — see below.

| run | image | drops | major blocks | outcome |
|---|---|---|---|---|
| 20260727T053231Z | `acc-4073:test` (fix) | `*:%499+3` | 12h | clean; DN produced **nothing** to any BVN in 5 min |
| 20260727T054232Z | `acc-release:v1.4.5` (no fix) | `bvn-BVN1:%997+3` | 1 min | DN→BVN1 7/7 delivered — recovered without the fix |
| 20260727T055020Z | `acc-release:v1.4.5` (no fix) | `bvn-BVN1:%7+1` | 1 min | DN→BVN1 7/7 delivered — recovered without the fix |
| 20260727T055822Z | `acc-4073:test` (fix) | `bvn-BVN1:%7+1` | 1 min | clean, 6669 heals, no seizure — parity with the run above |

**Why a 5-minute soak cannot reproduce #4073.** The bug requires a loss with
*nothing following it for a long time*. With the stock `majorBlockSchedule` of
`0 */12 * * *` the DN→BVN stream carries ~2 messages a day — quiet enough to
break, but it produces nothing at all inside a 5-minute window (run 1). Raising
the schedule to once a minute makes the stream active enough to exercise, but
that hands the ordinary #4064 gap healer exactly the later message it needs, and
it recovers on its own (runs 2 and 3). The precondition and the short window are
mutually exclusive.

**The deterministic proof is `TestSyntheticHealingLostPrefix`**, which
constructs the condition directly: one synthetic, dropped, with no traffic behind
it. Verified red/green — with the reconcile branch disabled the produced
synthetic never delivers.

What the short runs do establish: the fix causes no regression in normal healing
(run 4 matches run 3 under identical drops), the harness now supports sub-hour
durations, and the `undeliv` metric and its seizewatch trip are wired in.
