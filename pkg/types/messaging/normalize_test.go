// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An envelope's messages normalize in one order on every node and every
// run. The placeholders for signed-but-absent transactions were appended by
// ranging a map, so their order was whatever the map felt like; a
// consumer that happened not to care is not a guarantee (#4279 review).
func TestNormalize_PlaceholdersKeepTheSignaturesOrder(t *testing.T) {
	var want [][32]byte
	env := new(Envelope)
	for i := byte(1); i <= 6; i++ {
		h := [32]byte{i, 0xa0 ^ i, 0x5c}
		want = append(want, h)
		env.Messages = append(env.Messages, &SignatureMessage{
			Signature: &protocol.ED25519Signature{},
			TxID:      protocol.UnknownUrl().WithTxID(h),
		})
	}
	// A repeated signature for one transaction is one placeholder, in
	// the position of its first sighting.
	env.Messages = append(env.Messages, &SignatureMessage{
		Signature: &protocol.ED25519Signature{},
		TxID:      protocol.UnknownUrl().WithTxID(want[2]),
	})

	for run := 0; run < 100; run++ {
		msgs, err := env.Normalize()
		require.NoError(t, err)
		var got [][32]byte
		for _, m := range msgs {
			if txn, ok := m.(*TransactionMessage); ok {
				got = append(got, txn.Transaction.Body.(*protocol.RemoteTransaction).Hash)
			}
		}
		require.Equal(t, want, got, "run %d: placeholders in signature order, once each", run)
	}
}
