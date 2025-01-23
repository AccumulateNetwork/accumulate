// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

// votes tracks votes by validators.
type votes struct {
	context    context.Context
	validators []*validator
	votes      map[[32]byte]struct{}
}

// newVotes returns a new vote tracker and marks the current node as having
// voted.
func (n *Node) newVotes() votes {
	return votes{n.context, n.validators, map[[32]byte]struct{}{
		n.self.PubKeyHash: {},
	}}
}

// add adds the vote of the given node, if it is a validator.
func (v *votes) add(k [32]byte) {
	if v.votes == nil {
		v.votes = map[[32]byte]struct{}{}
	}
	for _, val := range v.validators {
		if val.PubKeyHash == k {
			v.votes[k] = struct{}{}
			return
		}
	}
	slog.ErrorContext(v.context, "Vote by non-validator", "id", logging.AsHex(k).Slice(0, 4))
}

// reachThreshold returns true if 2/3 of the validators have voted.
func (v votes) reachedThreshold() bool {
	r := len(v.validators) - len(v.votes)
	return r*3 <= len(v.validators)
}
