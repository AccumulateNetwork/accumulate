// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"context"
	"crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A proof names the Directory anchor that proves it: the destination collects
// the proof in anchor staging under that block until the anchor executes
// (executor spec, "Anchor staging"). Both dispatch forms carry it.

func TestSynthPackageProof_NamesTheDirectoryAnchor(t *testing.T) {
	f := newStagingFixture(t, 3)
	rootChain, err := f.batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	require.NoError(t, rootChain.AddEntry(f.chain.Anchor(), false))
	rootReceipt, err := rootChain.Receipt(0, 0)
	require.NoError(t, err)

	seg, err := merkle.NewSegment(f.chain2.Inner(), 0)
	require.NoError(t, err)
	pkg := []*synthOutbound{{index: 0}, {index: 1}}
	proof, err := f.x.buildSynthPackageProof(pkg, seg, rootReceipt, nil, 2, 42)
	require.NoError(t, err)
	require.True(t, proof.ReceiptList.Validate(nil))
	require.NotNil(t, proof.Anchor)
	require.Equal(t, protocol.DnUrl(), proof.Anchor.Account)
	require.Equal(t, uint64(42), proof.Anchor.SourceBlock, "the Directory block whose anchor proves the package")
}

type captureDispatcher struct{ envelopes []*messaging.Envelope }

func (d *captureDispatcher) Submit(_ context.Context, _ *url.URL, env *messaging.Envelope) error {
	d.envelopes = append(d.envelopes, env)
	return nil
}
func (d *captureDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
func (d *captureDispatcher) Close() {}

func TestSynthOwnProof_NamesTheDirectoryAnchor(t *testing.T) {
	f := newStagingFixture(t, 3)
	f.x.globalsPtr.Store(&Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest, Network: &protocol.NetworkDefinition{Version: 1}}})
	_, f.x.Key, _ = ed25519.GenerateKey(nil)
	d := new(captureDispatcher)
	f.x.mainDispatcher = d

	rootChain, err := f.batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	require.NoError(t, rootChain.AddEntry(f.chain.Anchor(), false))
	rootReceipt, err := rootChain.Receipt(0, 0)
	require.NoError(t, err)

	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("alice", "tokens")
	txn.Body = &protocol.SyntheticDepositCredits{Amount: 1}
	o := &synthOutbound{index: 0, seq: &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      protocol.PartitionUrl("BVN1"),
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      1,
	}}
	seg, err := merkle.NewSegment(f.chain2.Inner(), 0)
	require.NoError(t, err)
	require.NoError(t, f.x.sendSynthWithOwnProof(o, seg, rootReceipt, nil, 2, 42))
	require.Len(t, d.envelopes, 1)
	syn, ok := d.envelopes[0].Messages[0].(*messaging.SyntheticMessage)
	require.True(t, ok)
	require.NotNil(t, syn.Proof.Anchor)
	require.Equal(t, uint64(42), syn.Proof.Anchor.SourceBlock)
}
