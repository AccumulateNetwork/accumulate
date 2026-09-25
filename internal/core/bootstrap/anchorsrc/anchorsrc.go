// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package anchorsrc is where a joining node's roots come from, and the reason
// they are worth anything.
//
// A Collector gathers a partition's own anchors from its validators, and
// records a StateTreeAnchor only after a quorum of that partition's
// validators is shown to have signed it. The join's one proof is its local
// root equal to one of those roots (executor spec, "Sync", "The algorithm",
// step 3), so this is where the chain of trust either terminates in a key the
// node already holds or terminates in a peer.
//
// # Trust
//
// The peer is not trusted for anything. The validator sets come from the
// node's own record (see Authority, FromTrusted) and never from the peer; a
// signature is checked against a key, so nothing has to exist on the network
// before this can work. An anchor is accepted only when valid signatures from
// DISTINCT members of the producing partition's set reach that partition's
// threshold; a second copy from one validator is no second signature.
package anchorsrc

// Stall is an anchor the collector is held at: none of the validators asked
// for it served it signed by a quorum, so the read stays in front of it and
// the roots from there on wait (#4419). The stall ends when one is served.
type Stall struct {
	// Entry is the anchor's sequence number.
	Entry uint64

	// Asked names the validators asked for it by the read that last held it.
	Asked []string

	// Err is why the last answer for it could not be taken.
	Err error
}
