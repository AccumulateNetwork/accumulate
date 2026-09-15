#!/usr/bin/env python3
"""Read a stall out of the node log.

Every block logs, per stream it advanced or that is behind, a `Stream
position` line, and per destination it produced for, a `Stream produced`
line (executor spec, "What a stream logs"). This reads those lines back out
of `node-logs-live.txt` (or any docker-compose log) and says, per node and
stream: where it stands, when it last advanced, how long it has been
waiting and on what, whether any number ever went backwards, and whether
the producer's numbering has gaps.

    streamlog.py runs/<id>/node-logs-live.txt [--node acc-bvn1-val1]
        [--stream synthetic:BVN2] [--stall 60]

The lines are `<container> | <ts> INFO Stream position k=v ...`, possibly
ANSI-coloured. Anything else is ignored.
"""
import argparse, re, sys
from collections import OrderedDict

# A caught-up stream logs only every StreamLogEvery blocks (60, ~60s at a
# one-second block), so anything at or under that cadence is silence, not a
# stall. The default sits well clear of it.
DEFAULT_STALL = 180.0

ANSI = re.compile(r"\x1b\[[0-9;]*m")
LINE = re.compile(r"^(?P<node>\S+)\s*\|\s*(?P<ts>\S+)\s+INFO\s+Stream (?P<kind>position|produced)\s+(?P<kv>.*)$")
KV = re.compile(r"(\w+)=(\S+)")


def parse(lines):
    """Yield (node, ts, kind, fields) for every stream line."""
    for raw in lines:
        line = ANSI.sub("", raw.rstrip("\n"))
        m = LINE.match(line)
        if not m:
            continue
        f = {}
        for k, v in KV.findall(m.group("kv")):
            try:
                f[k] = int(v)
            except ValueError:
                f[k] = v
        yield m.group("node"), m.group("ts"), m.group("kind"), f


def num(f, key):
    """A field as a number. A log line is text and a value may be anything;
    one unexpected token in a multi-gigabyte log must not abort the whole
    analysis with a TypeError deep in a comparison."""
    v = f.get(key, 0)
    if isinstance(v, int):
        return v
    try:
        return int(str(v).replace(",", ""))
    except (TypeError, ValueError):
        return 0


class Stream:
    """One (node, ledger, source) stream's history."""

    def __init__(self):
        self.first = self.last = None
        self.samples = 0
        self.advances = 0
        self.delivered = self.sighted = self.reach = self.held = self.waiting = 0
        self.block = 0
        self.last_advance_ts = None   # seconds, when delivered last moved
        self.last_advance_at = None   # the stamp of that advance, as written
        self.last_advance_block = None
        self.waiting_since = None     # seconds, when the current hole appeared
        self.waiting_at = None        # the stamp of that sample
        self.regressions = []         # (ts, field, before, after)
        self.longest_stall = (0.0, None)  # seconds, ts it ended

    def take(self, ts, f):
        t = when(ts)
        if self.first is None:
            self.first = ts
            self.last_advance_ts, self.last_advance_at = t, ts
        for field in ("delivered", "sighted"):
            v = num(f, field)
            if self.samples and v < getattr(self, field):
                self.regressions.append((ts, field, getattr(self, field), v))
        if num(f, "advanced") > 0 or num(f, "delivered") > self.delivered:
            gap = t - self.last_advance_ts
            if gap > self.longest_stall[0]:
                self.longest_stall = (gap, ts)
            self.last_advance_ts, self.last_advance_at = t, ts
            self.last_advance_block = f.get("block")
            self.advances += 1
        w = num(f, "waiting")
        if w != self.waiting or not self.samples:
            self.waiting_since = t if w else None
            self.waiting_at = ts if w else None
        self.delivered, self.sighted = num(f, "delivered"), num(f, "sighted")
        self.reach, self.held, self.waiting = num(f, "reach"), num(f, "held"), w
        self.block = f.get("block", self.block)
        self.last = ts
        self.samples += 1


class Producer:
    """One (node, destination) producer's numbering."""

    def __init__(self):
        self.first = self.last = None
        self.count = 0
        self.to = 0
        self.gaps = []        # (ts, expected_from, actual_from)
        self.regressions = []  # (ts, prev_to, from)

    def take(self, ts, f):
        lo, hi = num(f, "from"), num(f, "to")
        if self.first is None:
            self.first = ts
        elif lo < self.to + 1:
            self.regressions.append((ts, self.to, lo))
        elif lo > self.to + 1:
            self.gaps.append((ts, self.to + 1, lo))
        self.to = max(self.to, hi)
        self.count += num(f, "count")
        self.last = ts


def when(ts):
    """Seconds from an ISO-8601 UTC stamp, for differences only."""
    import datetime
    try:
        return datetime.datetime.strptime(ts[:19], "%Y-%m-%dT%H:%M:%S").timestamp()
    except ValueError:
        return 0.0


def read(lines, node=None, stream=None):
    streams, producers = OrderedDict(), OrderedDict()
    for n, ts, kind, f in parse(lines):
        if node and n != node:
            continue
        if kind == "position":
            key = (n, f.get("ledger", "?"), f.get("source", "?"))
            if stream and "%s:%s" % (key[1], key[2]) != stream:
                continue
            streams.setdefault(key, Stream()).take(ts, f)
        else:
            key = (n, f.get("destination", "?"))
            producers.setdefault(key, Producer()).take(ts, f)
    return streams, producers


def report(streams, producers, stall=DEFAULT_STALL, out=sys.stdout):
    w = out.write
    w("STREAMS  (node, ledger <- source): delivered / sighted, held, waiting on, last advance\n")
    for (n, ledger, src), s in streams.items():
        end = when(s.last)
        since = end - s.last_advance_ts if s.last_advance_ts else 0
        flags = []
        if s.sighted > s.delivered and since >= stall:
            flags.append("STALLED %ds, waiting on %d since %s"
                         % (since, s.waiting, s.waiting_at or s.first))
        for ts, field, a, b in s.regressions:
            flags.append("%s WENT BACKWARDS %s %d->%d" % (field, ts, a, b))
        w("  %-14s %-9s <- %-9s  %9d / %-9d held %-5d waiting %-8d last advance %s (block %s)  %s\n" % (
            n, ledger, src, s.delivered, s.sighted, s.held, s.waiting,
            s.last_advance_at or "-", s.last_advance_block, "; ".join(flags)))
        if s.longest_stall[0] >= stall and s.advances:
            w("  %-14s   longest wait between advances: %ds, ending %s\n" % ("", s.longest_stall[0], s.longest_stall[1]))
    w("PRODUCERS  (node -> destination): last number, count, numbering\n")
    for (n, dst), p in producers.items():
        flags = ["GAP %s: expected %d, got %d" % g for g in p.gaps]
        flags += ["BACKWARDS %s: after %d came %d" % r for r in p.regressions]
        w("  %-14s -> %-9s  to %-9d count %-9d %s\n" % (n, dst, p.to, p.count, "; ".join(flags) or "contiguous"))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("log")
    ap.add_argument("--node")
    ap.add_argument("--stream", help="ledger:source, e.g. synthetic:BVN2")
    ap.add_argument("--stall", type=float, default=DEFAULT_STALL,
                    help="seconds without an advance, while behind, that counts as a stall; "
                         "must exceed the emitter's cadence (StreamLogEvery blocks) or a quiet "
                         "healthy stream reads as stalled")
    a = ap.parse_args(argv)
    with open(a.log, errors="replace") as f:
        streams, producers = read(f, a.node, a.stream)
    if not streams and not producers:
        sys.stderr.write("no Stream lines in %s -- a build before #4279 logs none\n" % a.log)
        return 2
    report(streams, producers, a.stall)
    return 0


if __name__ == "__main__":
    sys.exit(main())
