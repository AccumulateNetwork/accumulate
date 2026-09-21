#!/usr/bin/env python3
# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""The stranded series of a run, read once and corrected once (#4364).

`submissions.csv` holds one row per (node, partition) per sample. Both of
the manifest's rows — the stranded headline and the per-disturbance step
table — are readings of the SAME series, and both have been wrong in ways
that cancel a loss into silence. This module is where the series is built,
so the two cannot drift apart: `soak.sh` imports it from both.

Three corrections live here, each from a run or a review:

**A sample is a reading only when every (node, partition) reported.** A
container mid-restart answers no scrape, its rows are blank, and summing
what is left dips the total by that node's real count on exactly the
sample it was unreachable. The window minimum then takes the dip as the
settled level: a loss masked at the restart, or a negative step.

**Prometheus counters are process-local, so a restarted or re-added node
starts again at 0.** Its pre-restart cumulative loss leaves the fleet
series, the floor steps DOWN, and every later loss of that size is masked
— while "the figure does not climb between disturbances" is satisfied by
construction at every re-add. Since #4364's own run removes and re-adds
the follower, that is not an edge case; it is the run. A decrease in a
pair's `accepted` is the reset signal (counters are monotone within a
process), and from then on that pair carries an offset of everything it
had stranded before.

**The figure is not monotone**, though its inputs are: it rises when a
submission is accepted and falls when the relay is answered. So a level is
the MINIMUM over a window of samples and never a single reading.
"""
import csv
import datetime

STRANDED = "acceptedNeitherCertifiedTakenNorRefused"


def _epoch(t):
    return datetime.datetime.strptime(t, "%Y-%m-%dT%H:%M:%SZ").replace(
        tzinfo=datetime.timezone.utc).timestamp()


def _int(x):
    x = (x or "").strip()
    if not x:
        return None
    try:
        return int(x)
    except ValueError:
        return None


def load(path, role="", window=120):
    """Read `submissions.csv` and return the corrected series.

    `window` is the span a level is read over — the same
    `STEP_WINDOW_SECS` the step table uses — and it is what a reset
    carries: the FLOOR of the pair's last `window` of complete samples,
    not its last reading. The figure jitters by whatever is in flight, so
    a restart that lands on a spike would otherwise carry the spike
    forever: 3,7,3,7 restarting at the 7 carries 14 for a settled 6. It
    never invents a climb — the offset is constant thereafter — but it
    inflates the headline, permanently and invisibly.

    Returns a dict:

    ``pairs``     every (node, partition) the file mentions for this role
    ``samples``   one per timestamp, ascending, each
                  ``{time, epoch, complete, byPair, total, reported}``
                  where `byPair` and `total` are RESET-CORRECTED and are
                  present only on a complete sample
    ``resets``    ``[(time, node, partition, carried)]``, when each was
                  detected and what it carried forward
    ``dropped``   how many samples were not complete
    ``error``     a sentence, when the file cannot be read at all

    A pair's rows are deduped by (time, node, partition), last row wins:
    the forced final row can share a second with a periodic one, and these
    are counters — two readings of one counter are one reading.
    """
    try:
        rows = [r for r in csv.DictReader(open(path))
                if not role or r.get("role") == role]
    except OSError:
        return {"error": "no `submissions.csv`", "pairs": set(), "samples": [],
                "resets": [], "dropped": 0}
    if not rows:
        return {"error": "no rows for this role", "pairs": set(),
                "samples": [], "resets": [], "dropped": 0}

    pairs = {(r.get("node"), r.get("partition")) for r in rows}
    at = {}
    for r in rows:
        at.setdefault(r["time"], {})[(r.get("node"), r.get("partition"))] = r

    # Per pair, across the run: the offset its resets have accumulated, the
    # last raw `accepted` seen (to spot the next reset) and the pair's
    # recent raw stranded readings (whose FLOOR the offset takes on when a
    # reset happens — one reading could be a jitter spike).
    off, prev_acc, recent = {}, {}, {}
    resets, samples, dropped = [], [], 0

    for t in sorted(at):
        got = at[t]
        reported = {k for k, r in got.items()
                    if _int(r.get(STRANDED)) is not None}
        if reported != pairs:
            dropped += 1
            samples.append({"time": t, "epoch": _epoch(t), "complete": False,
                            "byPair": {}, "total": None, "reported": reported})
            continue
        by_pair = {}
        now = _epoch(t)
        for k, r in got.items():
            raw = _int(r.get(STRANDED))
            acc = _int(r.get("accepted"))
            # A counter that went BACKWARDS is a new process, not a
            # correction: carry forward what the old one had SETTLED at —
            # the floor of its last `window` of readings, because one
            # reading could be a jitter spike and the offset is permanent.
            if (acc is not None and prev_acc.get(k) is not None
                    and acc < prev_acc[k]):
                hist = [v for tt, v in recent.get(k, ())
                        if tt >= now - window] or \
                    [v for _, v in recent.get(k, ())][-1:]
                carried = min(hist) if hist else 0
                off[k] = off.get(k, 0) + carried
                resets.append((t, k[0], k[1], carried))
                recent[k] = []
            if acc is not None:
                prev_acc[k] = acc
            recent.setdefault(k, []).append((now, raw))
            recent[k] = [(tt, v) for tt, v in recent[k] if tt >= now - window]
            by_pair[k] = raw + off.get(k, 0)
        samples.append({"time": t, "epoch": _epoch(t), "complete": True,
                        "byPair": by_pair, "total": sum(by_pair.values()),
                        "reported": reported})
    return {"error": None, "pairs": pairs, "samples": samples,
            "resets": resets, "dropped": dropped}


def complete(series):
    return [s for s in series["samples"] if s["complete"]]


def points(series):
    """(epoch, corrected fleet total) for every complete sample."""
    return [(s["epoch"], s["total"]) for s in complete(series)]


def floor_of(pts, lo, hi):
    """The settled level over [lo, hi): the minimum, never one reading.

    Returns (value, epoch of the first point used) or (None, None).

    There is no "first N points" option, and there was: the after-floor
    took the first two samples from the settle, on the reasoning that a
    LATER loss should not be billed to the disturbance. A minimum already
    ignores a later loss — a loss RAISES the figure — so all `first_n`
    could do was exclude later, LOWER readings: after-samples 5, 4, 3, 3
    read 4 with it and 3 without, overstating the step and understating
    the creep beside it (reviewer M1 on #4364). A window that is nearest
    the disturbance is chosen by its BOUNDS, not by a count.
    """
    inrange = [(t, v) for t, v in pts if lo <= t < hi]
    if not inrange:
        return None, None
    return min(v for _, v in inrange), inrange[0][0]


def resets_row(series):
    """The manifest's line about counter resets, or None when there were
    none. A reset is not a fault — it is what restarting a node does — but
    a reader who does not know one happened cannot account for the floor
    stepping down, and every later loss of that size is masked. The line
    names what each one carried, because that number is added to every
    later reading and nothing else on the board shows it."""
    rs = series.get("resets") or []
    if not rs:
        return None
    where = ", ".join("%s/%s at %s carrying %d" % (n, p, t[11:16] + "Z", c)
                      for t, n, p, c in rs[:6])
    more = "" if len(rs) <= 6 else ", and %d more" % (len(rs) - 6)
    return ("%d (%s%s) — the carry is the floor of the pair's last samples, "
            "not its last reading, and the series does not step down"
            % (len(rs), where, more))
