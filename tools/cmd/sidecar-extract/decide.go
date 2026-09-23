// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import "bytes"

// held is one archive's value for a key; ok is false when the archive holds
// the key but its value could not be read.
type held struct {
	archive *archive
	value   []byte
	ok      bool
}

// decision is what becomes of a key.
type decision struct {
	value      []byte // for the sidecar, when every partition agrees
	conflicts  []held // one per partition, when they do not
	lost       bool   // no archive could produce a value
	superseded int    // older copies of a partition whose different value lost to a newer one
}

// decide picks, for each partition, the value of its first readable copy — the
// copies of one partition are given newest first — and then puts the key in the
// sidecar if the partitions agree, or in conflicts if they do not. The holders
// are in command-line order.
func decide(holders []held) decision {
	var picks []held
	pick := map[string]int{}
	var d decision
	for _, h := range holders {
		if !h.ok {
			continue
		}
		if i, ok := pick[h.archive.group]; ok {
			if !bytes.Equal(picks[i].value, h.value) {
				d.superseded++
			}
			continue
		}
		pick[h.archive.group] = len(picks)
		picks = append(picks, h)
	}

	if len(picks) == 0 {
		d.lost = true
		return d
	}
	for _, p := range picks[1:] {
		if !bytes.Equal(p.value, picks[0].value) {
			d.conflicts = picks
			return d
		}
	}
	d.value = picks[0].value
	return d
}
