// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"testing"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An envelope carrying a message with a field left out is refused when it is
// decoded and normalized, before the executor sees it (#4572). These are the
// bytes CheckTx and block delivery decode.
func TestExecuteTransactions_RefusesMalformedMessages(t *testing.T) {
	cases := map[string]messaging.Message{
		"transaction without transaction": &messaging.TransactionMessage{},
		"transaction without body": &messaging.TransactionMessage{Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
		}},
		"signature without signature":     &messaging.SignatureMessage{},
		"block anchor without anchor":     &messaging.BlockAnchor{},
		"sequenced without message":       &messaging.SequencedMessage{},
		"synthetic without message":       &messaging.SyntheticMessage{},
		"bad synthetic without message":   &messaging.BadSyntheticMessage{},
		"credit payment without txid":     &messaging.CreditPayment{},
		"signature request without txid":  &messaging.SignatureRequest{},
		"anchor of an empty sequence":     &messaging.BlockAnchor{Anchor: &messaging.SequencedMessage{}},
		"synthetic of an empty sequence":  &messaging.SyntheticMessage{Message: &messaging.SequencedMessage{}},
		"sequenced txn without txn":       &messaging.SequencedMessage{Message: &messaging.TransactionMessage{}},
		"sequenced sig without signature": &messaging.SequencedMessage{Message: &messaging.SignatureMessage{}},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := (&messaging.Envelope{Messages: []messaging.Message{msg}}).MarshalBinary()
			require.NoError(t, err)

			executed := false
			require.NotPanics(t, func() {
				_, _, _, err = executeTransactions(log.NewNopLogger(), func(*messaging.Envelope) ([]*protocol.TransactionStatus, error) {
					executed = true
					return nil, nil
				}, raw)
			})
			require.Error(t, err)
			require.False(t, executed, "the executor was reached")
		})
	}
}
