// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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

// anchorPool answers the two queries a Source makes against the Directory's
// anchor pool -- the chain's count and a range of its entries -- out of a list
// of anchors, the way the API answers them. It answers nothing else.
type anchorPool struct {
	entries []*api.MessageRecord[messaging.Message]
}

func (p *anchorPool) Query(_ context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	cq, ok := q.(*api.ChainQuery)
	if !ok || !scope.Equal(protocol.DnUrl().JoinPath(protocol.AnchorPool)) {
		return nil, errors.NotFound.WithFormat("no such record")
	}
	if cq.Range == nil {
		return &api.ChainRecord{Name: "main", Count: uint64(len(p.entries))}, nil
	}
	rr := new(api.RecordRange[api.Record])
	for i := cq.Range.Start; i < uint64(len(p.entries)); i++ {
		rr.Records = append(rr.Records, &api.ChainEntryRecord[api.Record]{
			Name: "main", Index: i, Value: p.entries[i],
		})
	}
	rr.Start = cq.Range.Start
	rr.Total = uint64(len(p.entries))
	return rr, nil
}

// anchoredSource is a real anchor source over a Directory pool holding the
// given anchors, so the roots those anchors carry are the ones it proves.
func anchoredSource(t *testing.T, values *core.GlobalValues, anchors ...*api.MessageRecord[messaging.Message]) *anchorsrc.Source {
	t.Helper()
	a, err := anchorsrc.FromValues(values)
	require.NoError(t, err)
	here := protocol.PartitionUrl("BVN0")
	pool, err := anchorsrc.PoolFor(here, a.BvnNames())
	require.NoError(t, err)
	s, err := anchorsrc.New(&anchorPool{entries: anchors}, pool, here, a)
	require.NoError(t, err)
	return s
}
