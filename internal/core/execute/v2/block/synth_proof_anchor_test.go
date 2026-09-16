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

// A proof names what it terminates in, and the destination stages it under
// that (executor spec, "Anchor staging"). Dispatch at begin(N+1) ends at the
// SOURCE's own root and names the source and the source block, because the
// Directory has not receipted it yet (#4253); a proof carrying the Directory
// receipt ends at a Directory root and names the Directory block. Both
// dispatch forms carry the annotation.

func TestSynthPackageProof_NamesWhatItTerminatesIn(t *testing.T) {
	f := newStagingFixture(t, 3)
	rootChain, err := f.batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	require.NoError(t, rootChain.AddEntry(f.chain.Anchor(), false))
	rootReceipt, err := rootChain.Receipt(0, 0)
	require.NoError(t, err)

	seg, err := merkle.NewSegment(f.chain2.Inner(), 0)
	require.NoError(t, err)
	pkg := []*synthOutbound{{index: 0}, {index: 1}}
	// Sent at begin(N+1): no Directory receipt yet, so the proof stops at
	// this partition's own root and says so.
	proof, err := f.x.buildSynthPackageProof(pkg, seg, rootReceipt, nil, 2, 0, 7)
	require.NoError(t, err)
	require.True(t, proof.ReceiptList.Validate(nil))
	require.NotNil(t, proof.Anchor)
	require.Equal(t, protocol.PartitionUrl("BVN0"), proof.Anchor.Account, "the source partition, whose root the proof ends in")
	require.Equal(t, uint64(7), proof.Anchor.SourceBlock, "the source block the Directory must receipt to extend it")
	require.NotEqual(t, protocol.DnUrl(), proof.Anchor.Account, "a source-root proof must not claim to be Directory-anchored")
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
	require.NoError(t, f.x.sendSynthWithOwnProof(o, seg, rootReceipt, nil, 2, 0, 7, 0))
	require.Len(t, d.envelopes, 1)
	syn, ok := d.envelopes[0].Messages[0].(*messaging.SyntheticMessage)
	require.True(t, ok)
	require.NotNil(t, syn.Proof.Anchor)
	require.Equal(t, protocol.PartitionUrl("BVN0"), syn.Proof.Anchor.Account)
	require.Equal(t, uint64(7), syn.Proof.Anchor.SourceBlock)
}
