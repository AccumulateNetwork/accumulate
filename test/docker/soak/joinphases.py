#!/usr/bin/env python3
"""joinphases.py: for each late follower a run added, how long it took to go
ACTIVE and how long until every section of its tree was loaded.

The walk's add -> ACTIVE time ends when the overlap holds; the fill loads the
rest of the tree after that, while the node executes on its own. A follower is
synced when both are done. Read from the node-logs-<follower>-<n>.txt files a
run saves, one per join.

Usage: joinphases.py <run dir>
"""
import datetime, glob, os, re, sys

ANSI = re.compile(r"\x1b\[[0-9;]*m")
STAMP = re.compile(r"(20\d\d-\d\d-\d\dT\d\d:\d\d:\d\dZ)")


def ts(s):
    return datetime.datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ")


def ct(t):
    return (t - datetime.timedelta(hours=5)).strftime("%H:%M")


def phases(path):
    first = active = filled = booting = None
    with open(path, errors="replace") as f:
        for line in f:
            line = ANSI.sub("", line)
            m = STAMP.search(line)
            if not m:
                continue
            t = ts(m.group(1))
            first = first or t
            if "dn.acme" in line or "partition=Directory" in line:
                continue  # the BVN's join, not the directory's
            if active is None and "it is ACTIVE" in line:
                active = t
            elif filled is None and "Every section is loaded" in line:
                filled = t
            elif booting is None and "it is BOOTING" in line:
                booting = t
    return first, active, filled, booting


def main():
    run = sys.argv[1]
    rows = []
    for p in glob.glob(os.path.join(run, "node-logs-acc-*-fol*-*.txt")):
        m = re.search(r"node-logs-acc-(.+)-(\d+)\.txt$", p)
        if not m:
            continue
        rows.append((phases(p), m.group(1)))
    rows.sort(key=lambda r: r[0][0] or datetime.datetime.max)

    def mins(start, t):
        return f"{(t - start).total_seconds() / 60:5.1f}" if t else "    -"

    print("join  added(CT)  follower    ACTIVE  filled  left-ACTIVE   (minutes after added; - = not in the log)")
    for i, ((first, active, filled, booting), name) in enumerate(rows, 1):
        print(f"{i:4}  {ct(first):9}  {name:10}  {mins(first, active)}  {mins(first, filled)}  {mins(first, booting)}")


if __name__ == "__main__":
    main()
