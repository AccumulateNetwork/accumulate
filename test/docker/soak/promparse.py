# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""Parse a Prometheus text exposition into (name, labels, value) triples.

One copy, imported by every tool that scrapes a node. soakmon.py carried
this; nodewatch.py needs exactly the same reading of exactly the same
endpoint, and two copies of a parser are two chances to disagree about what
a node said -- which is the failure this directory has already paid for with
six copies of the topology (see topology.py).

Histogram buckets, _sum and _count come through as ordinary samples with
their labels; callers select what they want by name.
"""

import re

PROM_LINE = re.compile(r'^([a-zA-Z_:][\w:]*)(?:\{([^}]*)\})?\s+([-0-9.eE+]+)')
_LABEL = re.compile(r'(\w+)="([^"]*)"')


def parse_prom(text):
    """Yield (name, labels_dict, float_value) for each sample line.

    Comment lines (# HELP / # TYPE), blank lines and values that are not
    floats (NaN text, truncated reads) are skipped rather than raised: a
    partial scrape must degrade to fewer readings, never to an exception
    that takes the whole collection cycle down.
    """
    for line in text.splitlines():
        if not line or line[0] == "#":
            continue
        m = PROM_LINE.match(line)
        if not m:
            continue
        name, lbls, val = m.group(1), m.group(2) or "", m.group(3)
        labels = dict(_LABEL.findall(lbls))
        try:
            yield name, labels, float(val)
        except ValueError:
            continue
