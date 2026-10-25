// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// A BadSyntheticMessage without a receipt is refused, whether it is submitted
// or arrives in a proposed block (#4572), and naming it does not read through
// the missing proof.
func TestBadSyntheticWithoutReceiptIsRefused(t *testing.T) {
	for name, proof := range map[string]*AnnotatedReceipt{
		"no proof":   nil,
		"no receipt": {ReceiptList: &merkle.ReceiptList{}},
	} {
		t.Run(name, func(t *testing.T) {
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.Genesis(GenesisTime),
			)

			msg := func() messaging.Message {
				return &messaging.BadSyntheticMessage{
					Message: &messaging.SequencedMessage{
						Message: &messaging.TransactionMessage{Transaction: &Transaction{
							Header: TransactionHeader{Principal: AccountUrl("foo")},
							Body:   &SyntheticDepositTokens{},
						}},
						Source:      PartitionUrl("BVN1"),
						Destination: PartitionUrl("BVN0"),
						Number:      1,
					},
					Proof: proof,
				}
			}

			// Submitted
			statuses, err := sim.SubmitTo("BVN0", &messaging.Envelope{Messages: []messaging.Message{msg()}})
			require.NoError(t, err)
			require.Len(t, statuses, 1)
			require.NotEqual(t, errors.Delivered, statuses[0].Code)
			require.Error(t, statuses[0].AsError())

			// In a proposed block, past validation
			sim.S.SetNodeBlockHook("BVN0", func(_ int, _ execute.BlockParams, envs []*messaging.Envelope) ([]*messaging.Envelope, bool) {
				return append(envs, &messaging.Envelope{Messages: []messaging.Message{msg()}}), false
			})
			require.NoError(t, sim.S.Step())
			sim.StepN(2)
		})
	}
}
