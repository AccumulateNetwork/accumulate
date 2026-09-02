// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// streamOf is the single statement of "which ordered stream governs this
// message" (#4169 step 1). It replaced two copies that could disagree:
// SequencedMessage.isAnchor, which resolves a remote stub, and the readiness
// pre-pass's isAnchorBody, which did not and never opened a BlockAnchor.

func streamTestExec(t *testing.T) *Executor {
	t.Helper()
	x := new(Executor)
	x.Staging = execute.NewStaging()
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	x.globalsPtr.Store(new(Globals))
	x.globals().Active = core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionLatest,
	}
	return x
}

func streamSeq(body protocol.TransactionBody, principal *url.URL) *messaging.SequencedMessage {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = body
	return &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      protocol.PartitionUrl("BVN1"),
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      1,
	}
}

func TestStreamOf(t *testing.T) {
	x := streamTestExec(t)
	alice := protocol.AccountUrl("alice", "tokens")
	pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	synth := streamSeq(&protocol.SyntheticDepositCredits{Amount: 1}, alice)
	anchor := streamSeq(new(protocol.BlockValidatorAnchor), pool)

	cases := []struct {
		name   string
		msg    messaging.Message
		kind   streamKind
		ledger *url.URL
	}{
		{"synthetic in its wrapper", &messaging.SyntheticMessage{Message: synth}, streamSynthetic, x.Describe.Synthetic()},
		{"bad synthetic wrapper", &messaging.BadSyntheticMessage{Message: synth}, streamSynthetic, x.Describe.Synthetic()},
		{"bare sequenced", synth, streamSynthetic, x.Describe.Synthetic()},
		{"anchor in a BlockAnchor", &messaging.BlockAnchor{Anchor: anchor}, streamAnchor, x.Describe.AnchorPool()},
		{"bare sequenced anchor", anchor, streamAnchor, x.Describe.AnchorPool()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, seq, err := x.streamOf(c.msg, nil)
			require.NoError(t, err)
			require.True(t, s.ok(), "must be recognised as a stream message")
			assert.Equal(t, c.kind, s.kind)
			assert.Equal(t, c.ledger.String(), s.ledger.String(),
				"anchors and synthetics keep SEPARATE ledgers — sharing one would let an anchor's position gate a synthetic's")
			assert.Equal(t, protocol.PartitionUrl("BVN1").String(), s.source.String())
			require.NotNil(t, seq)
		})
	}
}

// A BlockAnchor is how an anchor actually travels. The pre-pass's copy did not
// open one, so it saw no anchors at all and silently declined to speak for
// every anchor stream.
func TestStreamOf_OpensABlockAnchor(t *testing.T) {
	x := streamTestExec(t)
	pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	anchor := streamSeq(new(protocol.BlockValidatorAnchor), pool)

	s, seq, err := x.streamOf(&messaging.BlockAnchor{Anchor: anchor}, nil)
	require.NoError(t, err)
	require.True(t, s.ok(), "an anchor travels inside a BlockAnchor — not opening it hides every anchor")
	assert.Equal(t, streamAnchor, s.kind)
	assert.Equal(t, anchor, seq)
}

// A sequenced message can carry a REMOTE placeholder whose real body decides
// whether it is an anchor. Only the supplied lookup can see it.
func TestStreamOf_ResolvesARemoteStub(t *testing.T) {
	x := streamTestExec(t)
	pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	stub := streamSeq(&protocol.RemoteTransaction{}, pool)

	real := new(protocol.Transaction)
	real.Header.Principal = pool
	real.Body = new(protocol.BlockValidatorAnchor)
	resolve := func([32]byte) (*protocol.Transaction, error) { return real, nil }

	s, _, err := x.streamOf(stub, resolve)
	require.NoError(t, err)
	assert.Equal(t, streamAnchor, s.kind, "the real body is an anchor, so the anchor pool governs it")

	// Without a lookup the stub cannot be classified as an anchor. That is the
	// executor's own answer for anything it cannot resolve, and it is why the
	// lookup is a parameter rather than an assumption.
	s, _, err = x.streamOf(stub, nil)
	require.NoError(t, err)
	assert.Equal(t, streamSynthetic, s.kind)
}

func TestStreamOf_NotAStream(t *testing.T) {
	x := streamTestExec(t)
	alice := protocol.AccountUrl("alice", "tokens")

	txn := new(protocol.Transaction)
	txn.Header.Principal = alice
	txn.Body = &protocol.SendTokens{}

	// A user transaction carries no position, so nothing orders it.
	s, seq, err := x.streamOf(&messaging.TransactionMessage{Transaction: txn}, nil)
	require.NoError(t, err)
	assert.False(t, s.ok(), "a user transaction belongs to no stream")
	assert.Nil(t, seq)

	// A sequenced message with no source names no stream either.
	sourceless := streamSeq(&protocol.SyntheticDepositCredits{Amount: 1}, alice)
	sourceless.Source = nil
	s, _, err = x.streamOf(sourceless, nil)
	require.NoError(t, err)
	assert.False(t, s.ok(), "without a source there is no stream to order it by")
}
