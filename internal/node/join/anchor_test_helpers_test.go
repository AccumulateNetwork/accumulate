// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// signedAnchor is the API record of one anchor BVN0 produced for block
// `block` carrying `root` as its StateTreeAnchor, in the Directory's pool,
// signed by a quorum of the genesis keys the way crosschain signs one. It is
// what anchorsrc verifies, and so the one thing that proves a root to the
// join.
func signedAnchor(t *testing.T, values *core.GlobalValues, keys []ed25519.PrivateKey, block uint64, root [32]byte) *api.MessageRecord[messaging.Message] {
	t.Helper()
	source, destination := protocol.PartitionUrl("BVN0"), protocol.DnUrl()

	txn := new(protocol.Transaction)
	txn.Header.Principal = destination.JoinPath(protocol.AnchorPool)
	txn.Body = &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{
		Source:          source,
		MinorBlockIndex: block,
		StateTreeAnchor: root,
	}}
	txnMsg := &messaging.TransactionMessage{Transaction: txn}
	seq := &messaging.SequencedMessage{
		Message:     txnMsg,
		Source:      source,
		Destination: destination,
		Number:      block,
	}
	h := seq.Hash()

	set := &api.SignatureSetRecord{
		Account:    &protocol.UnknownAccount{Url: txn.Header.Principal},
		Signatures: new(api.RecordRange[*api.MessageRecord[messaging.Message]]),
	}
	// A quorum: two thirds of the keys, rounded up.
	quorum := (2*len(keys) + 2) / 3
	for _, key := range keys[:quorum] {
		sig, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(key).
			SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
			SetVersion(values.Network.Version).
			SetTimestamp(1).
			Sign(h[:])
		require.NoError(t, err)
		set.Signatures.Records = append(set.Signatures.Records, &api.MessageRecord[messaging.Message]{
			Message: &messaging.BlockAnchor{Anchor: seq, Signature: sig.(protocol.KeySignature)},
		})
	}
	set.Signatures.Total = uint64(len(set.Signatures.Records))

	return &api.MessageRecord[messaging.Message]{
		ID:         txn.ID(),
		Message:    txnMsg,
		Sequence:   seq,
		Signatures: &api.RecordRange[*api.SignatureSetRecord]{Records: []*api.SignatureSetRecord{set}, Total: 1},
	}
}
