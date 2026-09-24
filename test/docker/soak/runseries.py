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

**A sample is a reading only when every (node, partition) reported or is
joining.** A container mid-restart answers no scrape, its rows are blank,
and summing what is left dips the total by that node's real count on
exactly the sample it was unreachable. The window minimum then takes the
dip as the settled level: a loss masked at the restart, or a negative
step. But an empty row BESIDE a reported one on the same node is not an
unreachable node — one scrape answers for both partitions — it is a
counter the process has not created: the node is joining that partition
and has counted 0 there (#4414). Reading it as incomplete blanked the
headline of run 20260924T074702Z for its last sixteen minutes.

**Prometheus counters are process-local, so a restarted or re-added node
starts again at 0.** Its pre-restart cumulative loss leaves the fleet
series, the floor steps DOWN, and every later loss of that size is masked
— while "the figure does not climb between disturbances" is satisfied by
construction at every re-add. Since #4364's own run removes and re-adds
the follower, that is not an edge case; it is the run. A decrease in a
pair's `accepted` is the reset signal (counters are monotone within a
process), and from then on that pair carries an offset of everything it
had stranded before.

**A paused node is a known state, not an unreachable one (#4425).** A
paused container answers no scrape, so every row of it is blank exactly as
a restarting one's is — but `chaos.log` names the node and the window, and
a paused process's counters cannot move. Its pairs are read at their last
answer before the pause, and the sample is complete. Reading it as
unreachable skipped the final row of run 20260924T093936Z, whose manifest
then said `FINAL ROW MISSING` over a row that had landed. Without a
`chaos.log` line covering the sample, a blank node is still unreachable.

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


# A pause is logged in whole seconds before `docker pause` runs, and the
# un-pause follows `chaos_wait` by the time `docker unpause` takes, so a
# sample stamped a second or two past the logged end can still have found
# the container paused. The four blank samples at a pause's end in run
# 20260924T093936Z were stamped 0-1 s after it.
PAUSE_SLACK_SECS = 5


def pauses(chaos_path):
    """The pauses `chaos.log` records: ``[(node, start, end, start_text,
    seconds)]`` with epochs. `soak.sh` logs `<t> pause <node> <p>s` when a
    pause starts and never logs its end; the end is `t + p`. An absent or
    unreadable file is no pauses — never an assumed one."""
    import re
    out = []
    try:
        with open(chaos_path) as f:
            lines = f.read().splitlines()
    except OSError:
        return out
    for l in lines:
        m = re.match(r"^(\S+Z) pause (\S+) (\d+)s", l)
        if m:
            t = _epoch(m.group(1))
            out.append((m.group(2), t, t + int(m.group(3)), m.group(1),
                        int(m.group(3))))
    return out


def load(path, role="", window=120, pauses=()):
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
                  ``{time, epoch, complete, byPair, total, reported,
                  joining, uncounted}`` where `byPair` and `total` are
                  RESET-CORRECTED and are present only on a complete
                  sample; `joining` and `uncounted` list the pairs read as
                  0 because their node answered and their counter does not
                  exist (`no_counter_pairs`) — joining if the pair counted
                  earlier in the run, uncounted if it never has
    ``resets``    ``[(time, node, partition, carried)]``, when each was
                  detected and what it carried forward
    ``dropped``   how many samples were not complete
    ``pausedSamples`` how many samples were completed by reading a paused
                  node at its last answer (#4425)

    Each sample also carries ``paused``: ``{(node, partition): (time of
    the reading used, the raw figure)}`` for the pairs of a node blank
    because `pauses` (from `pauses()`) covers it, and ``pausedNodes``:
    ``{node: (pause start text, seconds)}``. A paused pair's figure is its
    last answer — a stopped process's counter does not move — with any
    carried offset added, and it is never taken as a new reading for
    reset detection.
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
                "resets": [], "dropped": 0, "pausedSamples": 0}
    if not rows:
        return {"error": "no rows for this role", "pairs": set(),
                "samples": [], "resets": [], "dropped": 0, "pausedSamples": 0}

    pairs = {(r.get("node"), r.get("partition")) for r in rows}
    at = {}
    for r in rows:
        at.setdefault(r["time"], {})[(r.get("node"), r.get("partition"))] = r

    # Per pair, across the run: the offset its resets have accumulated, the
    # last raw `accepted` seen (to spot the next reset) and the pair's
    # recent raw stranded readings (whose FLOOR the offset takes on when a
    # reset happens — one reading could be a jitter spike).
    off, prev_acc, recent = {}, {}, {}
    # A joining carry not yet confirmed: pair -> (the `accepted` it had,
    # what was carried, the reset entry, its readings before the gap).
    unconfirmed = {}
    resets, samples, dropped = [], [], 0
    ever = set()  # pairs that have reported a count at some sample
    # pair -> (time, raw figure) at its last answer in a complete sample;
    # 0 for a pair that answered with no counter (joining / never counted)
    last_known = {}
    paused_samples = 0
    nodes = {n for n, _ in pairs}

    for t in sorted(at):
        got = at[t]
        reported = {k for k, r in got.items()
                    if _int(r.get(STRANDED)) is not None}
        empty = no_counter_pairs(got, reported)
        joining = {k for k in empty if k in ever}
        uncounted = empty - joining
        ever |= reported
        # A node with no answer on any partition, inside a pause chaos.log
        # records for it, with an answer on record for every pair: read at
        # that answer. Anything short of all three stays unreachable.
        now = _epoch(t)
        answered = {k for k in reported | empty}
        paused, paused_nodes = {}, {}
        for n in nodes:
            mine = {k for k in pairs if k[0] == n}
            if mine & answered:
                continue
            win = [p for p in pauses if p[0] == n
                   and p[1] <= now <= p[2] + PAUSE_SLACK_SECS]
            if not win or not all(k in last_known for k in mine):
                continue
            for k in mine:
                paused[k] = last_known[k]
            paused_nodes[n] = (win[-1][3], win[-1][4])
        if reported | empty | set(paused) != pairs:
            dropped += 1
            samples.append({"time": t, "epoch": _epoch(t), "complete": False,
                            "byPair": {}, "total": None, "reported": reported,
                            "joining": sorted(joining),
                            "uncounted": sorted(uncounted),
                            "paused": {}, "pausedNodes": {}})
            continue
        if paused:
            paused_samples += 1
        by_pair = {}
        for k, (_, raw) in paused.items():
            by_pair[k] = raw + off.get(k, 0)
        for k in sorted(empty):
            # A counter that existed and is now uncreated belongs to a new
            # process: carry what the old one had settled at, exactly as a
            # counter going backwards does, and forget its `accepted` so
            # that the counter's reappearance — lower, as a new process's
            # must be — is not carried a second time.
            if prev_acc.get(k) is not None:
                hist = [v for tt, v in recent.get(k, ())
                        if tt >= now - window] or \
                    [v for _, v in recent.get(k, ())][-1:]
                carried = min(hist) if hist else 0
                off[k] = off.get(k, 0) + carried
                resets.append((t, k[0], k[1], carried))
                unconfirmed[k] = (prev_acc[k], carried, resets[-1],
                                  recent.get(k, []))
                recent[k] = []
                prev_acc[k] = None
            by_pair[k] = off.get(k, 0)
        for k in empty:
            last_known[k] = (t, 0)
        for k, r in got.items():
            if k in empty or k in paused:
                continue
            raw = _int(r.get(STRANDED))
            acc = _int(r.get("accepted"))
            # The counter is back after a joining carry. HIGHER than it was
            # is the same process — a new one starts at 0 and cannot be —
            # so the row was lost, not the process: `_scrape_one` parses a
            # curl body cut by its timeout, and a cut between two
            # partitions' label sets empties one of them (reviewer F2 on
            # #4414). Unwind the carry and the reset it recorded; LOWER, it
            # stands.
            u = unconfirmed.pop(k, None)
            if u is not None and acc is not None and acc >= u[0]:
                off[k] -= u[1]
                resets.remove(u[2])
                prev_acc[k], recent[k] = u[0], u[3]
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
            last_known[k] = (t, raw)
            recent.setdefault(k, []).append((now, raw))
            recent[k] = [(tt, v) for tt, v in recent[k] if tt >= now - window]
            by_pair[k] = raw + off.get(k, 0)
        samples.append({"time": t, "epoch": _epoch(t), "complete": True,
                        "byPair": by_pair, "total": sum(by_pair.values()),
                        "reported": reported, "joining": sorted(joining),
                        "uncounted": sorted(uncounted), "paused": paused,
                        "pausedNodes": paused_nodes})
    return {"error": None, "pairs": pairs, "samples": samples,
            "resets": resets, "dropped": dropped,
            "pausedSamples": paused_samples}


def no_counter_pairs(got, reported):
    """The (node, partition) rows of one sample whose counter does not exist
    in the node's process (#4414): the row is present and empty, and the
    same node reported a count on another partition at this sample.

    One scrape of a container answers for every partition it runs, so a
    node that reported anywhere was reachable, and an empty row beside it
    is a counter that does not exist in that process: nothing has been
    submitted to that partition since the process started. A restarted
    node whose Directory has not rejoined is the case that exists — its
    BVN relays everything it accepts while its Directory row stays empty
    for the rest of the run — and it has counted 0 there, which is a
    reading. A node whose rows are ALL empty answered no scrape at all;
    that is neither, and the sample stays incomplete.

    `load` names the two kinds apart: JOINING when the pair counted
    earlier in the run (its node restarted and has not created the counter
    again), UNCOUNTED when it never has (nothing was ever submitted to that
    partition on that node — the follower's Directory, on every run)."""
    answered = {n for n, _ in reported}
    return {k for k, r in got.items()
            if k not in reported and k[0] in answered
            and _int(r.get(STRANDED)) is None}


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
