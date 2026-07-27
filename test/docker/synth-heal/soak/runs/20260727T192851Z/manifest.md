# Soak run 20260727T192851Z

**Purpose:** #4073 validation: 8h, 0.01tps, no bootstrap, 50% synthetic drops — quiet channels so tail losses occur and the interval reconcile must recover them

| field | value |
|---|---|
| started (UTC) | 2026-07-27T19:28:51Z |
| commit | `cff4c91d750797b1cd085778cef02523eccf696f` |
| describe | `v1.4.5-4-gcff4c91d7` |
| branch | `4073-idle-stream-reconcile` |
| uncommitted files | 0  |
| image | `acc-4073:test` |
| image id | `sha256:28d1704607694da9b77a75179a00e2cb857d88006422420209d231fcf6a92ffd` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%2+1` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 8h |
| target TPS | 0.01 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result — STOPPED EARLY, inconclusive by design

Stopped at ~2.9h of 8h. Not a failure, and not evidence either way.

| field | value |
|---|---|
| reconcile pulls (#4073) | **0** |
| stalled channels at end | **0** |
| generated | ~105, 0 rejected |
| containers | 12/12 throughout |

**Why it could not have proven the fix.** At 0.01 tps every channel keeps
receiving, so a dropped message almost always has a successor within minutes and
the ordinary #4064 gap healer recovers it long before the interval reconcile is
the only remaining path. Channels were still climbing when it was stopped
(BVN2→BVN1 at 36, BVN3→BVN1 at 23). A tail loss — the condition #4073 is about —
requires a channel to go silent AFTER a loss, which a long steady run prevents
rather than produces. Run 20260727T064442Z reproduced it in five minutes for
exactly the opposite reason: the run ENDED while a loss was outstanding.

Also of note: `pop-upgrade`, an unrelated Pop!_OS daemon, spun a full core from
~12:27 until it was killed at ~15:02, overlapping this run. The nodes were never
starved (each ~6% CPU, 135 MiB of a 4 GiB limit, box 78% idle) so results are not
believed affected, but the CPU column steps down partway through for that reason
and not because of anything the network did.

Superseded by the 1-second reconcile and the repeated short-run A/B.
