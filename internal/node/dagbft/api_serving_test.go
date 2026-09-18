// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestSubmitter_AJoiningNodeTakesNoTraffic — #4307.
//
// A joining node validates against a store its pull has half filled, so every
// user transaction fails on an account it does not have yet. Run
// 20260918T131713Z: 15,035 "validation failed ... load signer:
// Account.acc://<lite>.Main not found", each one a transaction the sender was
// told was invalid when it was not.
//
// NotReady, so the sender asks another node. A validation failure would be a
// lie about the transaction.
func TestSubmitter_AJoiningNodeTakesNoTraffic(t *testing.T) {
	svc, _, _ := newJoiningService(t)
	machine := nodestate.New(protocol.PartitionUrl("bvn1"))

	sub := NewSubmitterService(SubmitterServiceParams{Service: svc, NodeState: machine})
	val := NewValidatorService(ValidatorServiceParams{Service: svc, NodeState: machine})

	env := new(messaging.Envelope)
	ctx := context.Background()

	_, err := sub.Submit(ctx, env, api.SubmitOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "Submit: got %v", err)
	require.Contains(t, err.Error(), "is joining", "it must say why it refused")

	_, err = val.Validate(ctx, env, api.ValidateOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "Validate: got %v", err)
}

// TestSubmitter_ANodeThatNeverJoinedTakesTraffic — the gate is about a node
// catching up, not a new precondition for every node.
func TestSubmitter_ANodeThatNeverJoinedTakesTraffic(t *testing.T) {
	svc, _, _ := newJoiningService(t)
	ctx := context.Background()
	env := new(messaging.Envelope)

	for _, s := range []*SubmitterService{
		NewSubmitterService(SubmitterServiceParams{Service: svc}),
		NewSubmitterService(SubmitterServiceParams{Service: svc, NodeState: nodestate.Always{}}),
	} {
		_, err := s.Submit(ctx, env, api.SubmitOptions{})
		if err != nil {
			require.NotContains(t, err.Error(), "is joining",
				"an ungated node refused for joining")
		}
	}
}
