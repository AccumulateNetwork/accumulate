// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// JoinedAt is what a node that joined attaches to the NotReady it answers for
// a sequenced item produced before it joined: the block it joined at.
//
// The block was already in the message — "%v joined at block %d and produced
// nothing for %v at or before it" — and being in the message is the same as
// not being there at all. Nothing parses it, so a requester cannot tell "I
// will never have this, ask a node that executed it" from "I am busy, ask
// again", and the two want opposite responses: the first wants a different
// node, the second wants the same node later (#4295, threat review finding
// 4).
//
// It travels in errors.Error's Data, which survives the JSON-RPC and the
// message transports, so a requester on the other side of either can read it.
type JoinedAt struct {
	// Block is the block the answering node joined at. It executed no block
	// at or below it and never will: there is no backfill on this line.
	Block uint64 `json:"joinedAtBlock"`
}

// MarkJoinedAt attaches the block to a NotReady so a requester can act on it.
// The error is returned unchanged if it is not an *errors.Error.
func MarkJoinedAt(err error, block uint64) error {
	e := new(errors.Error)
	if !errors.As(err, &e) {
		return err
	}
	b, merr := json.Marshal(JoinedAt{Block: block})
	if merr != nil {
		return err
	}
	e.Data = b
	return e
}

// JoinedAtBlock reports the block a peer says it joined at, when its answer
// says so. A requester that gets one knows the peer will never hold what it
// asked for and should ask a node that executed it, rather than asking this
// one again.
func JoinedAtBlock(err error) (uint64, bool) {
	e := new(errors.Error)
	if !errors.As(err, &e) || len(e.Data) == 0 {
		return 0, false
	}
	var j JoinedAt
	if json.Unmarshal(e.Data, &j) != nil || j.Block == 0 {
		return 0, false
	}
	return j.Block, true
}
