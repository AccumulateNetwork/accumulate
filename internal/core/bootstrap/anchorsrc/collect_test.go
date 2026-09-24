// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// answering is one validator's sequencer: anchor n is answers[n], whatever
// number was asked for when lie is set.
type answering struct {
	answers map[uint64]*api.MessageRecord[messaging.Message]
	lie     *api.MessageRecord[messaging.Message]
}

func (a answering) Sequence(_ context.Context, _, _ *url.URL, n uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	if a.lie != nil {
		return a.lie, nil
	}
	rec, ok := a.answers[n]
	if !ok {
		return nil, errors.NotFound.WithFormat("anchor %d is not in the cache", n)
	}
	return rec, nil
}

type validatorList []Validator

func (v validatorList) ValidatorsOf(context.Context, *url.URL) ([]Validator, error) { return v, nil }

// lastAnchor answers a producer's anchor ledger at a sequence number.
type lastAnchor uint64

func (n lastAnchor) Query(_ context.Context, scope *url.URL, _ api.Query) (api.Record, error) {
	return &api.AccountRecord{Account: &protocol.AnchorLedger{Url: scope, MinorBlockSequenceNumber: uint64(n)}}, nil
}

// signedBy is anchor number n of BVN0, for block n, signed by one validator.
func (f *netFixture) signedBy(t *testing.T, n uint64, root [32]byte, signer int) *api.MessageRecord[messaging.Message] {
	return f.anchor(t, anchorOpts{source: bvn0(), destination: dn(), block: n, root: root, signers: []int{signer}})
}

type observed map[uint64][32]byte

func (o observed) record(_ *url.URL, block uint64, root [32]byte) { o[block] = root }

func collectorOver(t *testing.T, f *netFixture, vals validatorList, last uint64) (*Collector, observed) {
	t.Helper()
	c, err := NewCollector(bvn0(), f.authority(t), vals, lastAnchor(last))
	require.NoError(t, err)
	got := observed{}
	c.OnAnchor = got.record
	return c, got
}

// TestTheCollectorTakesAnAnchorItsValidatorsSignTogether — the anchor of a
// block is a partition's own, and each validator answers it signed with its
// own key alone (Sequencer.anchorRecord, #4424). No one answer carries a
// quorum; the collector counts the distinct members across the answers, to
// the partition's threshold (executor spec, "Sync", "The algorithm", step 3).
func TestTheCollectorTakesAnAnchorItsValidatorsSignTogether(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1) // threshold 3 of 4

	vals := func(signers ...int) validatorList {
		var out validatorList
		for _, i := range signers {
			out = append(out, Validator{Name: "v", Sequencer: answering{answers: map[uint64]*api.MessageRecord[messaging.Message]{
				7: f.signedBy(t, 7, root(7), i),
			}}})
		}
		return out
	}

	t.Run("three of four validators", func(t *testing.T) {
		c, got := collectorOver(t, f, vals(0, 1, 2), 7)
		require.NoError(t, c.Read(ctx))
		require.Equal(t, observed{7: root(7)}, got)
		_, held := c.Stalled()
		require.False(t, held)
	})

	t.Run("two of four validators", func(t *testing.T) {
		c, got := collectorOver(t, f, vals(0, 1), 7)
		require.NoError(t, c.Read(ctx))
		require.Empty(t, got, "an anchor two of four signed was taken where three are required")
		st, held := c.Stalled()
		require.True(t, held, "an anchor produced and not signed by a quorum is where the collector is held")
		require.Equal(t, uint64(7), st.Entry)
	})

	t.Run("one validator answering three times", func(t *testing.T) {
		c, got := collectorOver(t, f, vals(0, 0, 0), 7)
		require.NoError(t, c.Read(ctx))
		require.Empty(t, got, "a second answer from one validator is no second signature")
	})
}

// TestTheCollectorCountsSignaturesWithinOneAnchor: a validator that answers
// another body under the same number -- another root for the block -- signs
// that body, and its signature is not counted toward the honest one; nor do
// the honest signatures carry the lie.
func TestTheCollectorCountsSignaturesWithinOneAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)

	honest := func(i int) Validator {
		return Validator{Name: "honest", Sequencer: answering{answers: map[uint64]*api.MessageRecord[messaging.Message]{
			7: f.signedBy(t, 7, root(7), i),
		}}}
	}
	liar := Validator{Name: "liar", Sequencer: answering{answers: map[uint64]*api.MessageRecord[messaging.Message]{
		7: f.signedBy(t, 7, root(0x66), 2),
	}}}

	c, got := collectorOver(t, f, validatorList{honest(0), honest(1), liar}, 7)
	require.NoError(t, c.Read(ctx))
	require.Empty(t, got, "two honest signatures and one over another root were counted as three")

	c, got = collectorOver(t, f, validatorList{honest(0), honest(1), liar, honest(3)}, 7)
	require.NoError(t, c.Read(ctx))
	require.Equal(t, observed{7: root(7)}, got, "three honest signatures beside a liar did not take the anchor")
}

// TestTheCollectorRefusesAnAnswerToAnotherNumber: an anchor is asked for by
// its number, and an answer under another number is not the anchor asked for,
// however well signed.
func TestTheCollectorRefusesAnAnswerToAnotherNumber(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)

	var vals validatorList
	for i := 0; i < 3; i++ {
		vals = append(vals, Validator{Name: "v", Sequencer: answering{lie: f.signedBy(t, 5, root(5), i)}})
	}
	c, got := collectorOver(t, f, vals, 7)
	require.NoError(t, c.Read(ctx))
	require.Empty(t, got, "an anchor answered under number 5 was taken for number 7")
}

// TestTheCollectorReadsForwardFromTheNewestAnchor: the first read starts at
// the newest anchor the partition's ledger names, and each read takes every
// anchor produced since, in order, and stops at the first not produced yet.
func TestTheCollectorReadsForwardFromTheNewestAnchor(t *testing.T) {
	ctx := context.Background()
	f := newNet(t, 4, 1)

	answers := make([]map[uint64]*api.MessageRecord[messaging.Message], 3)
	for i := range answers {
		answers[i] = map[uint64]*api.MessageRecord[messaging.Message]{}
		for n := uint64(1); n <= 9; n++ {
			answers[i][n] = f.signedBy(t, n, root(byte(n)), i)
		}
	}
	var vals validatorList
	for i := range answers {
		vals = append(vals, Validator{Name: "v", Sequencer: answering{answers: answers[i]}})
	}

	c, got := collectorOver(t, f, vals, 6)
	require.NoError(t, c.Read(ctx))
	require.Equal(t, observed{6: root(6), 7: root(7), 8: root(8), 9: root(9)}, got,
		"the first read starts at the newest anchor and takes what follows")
	_, held := c.Stalled()
	require.False(t, held, "a number no validator has produced yet is not a stall")

	for i := range answers {
		answers[i][10] = f.signedBy(t, 10, root(10), i)
	}
	require.NoError(t, c.Read(ctx))
	require.Equal(t, root(10), got[10], "the next read did not go on from where the last stopped")
}
