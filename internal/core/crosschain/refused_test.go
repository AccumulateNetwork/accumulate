// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4426. A heal the destination refuses is a failed heal, not a landed one:
// the requester asks for the span again at its next activation instead of
// waiting out patience on an answer that will never arrive.
//
// The requester records a span as asked when the source answers, before the
// dispatcher has put the envelope in front of the destination; the refusal
// comes back later, on the dispatcher's queue. This drives it the way
// production does: the conductor started on the node's own dispatcher (which
// is how it registers for refusals), the activation's requestGaps asking the
// source and handing the answer to the dispatcher, the dispatcher's routed
// transport, and a destination that answers as the submit service answers a
// validation refusal (TestSubmitter_AValidationRefusalIsAnsweredAsTheRefusal
// pins that answer against the real service).
func TestRequester_ARefusedHealIsAskedAgain(t *testing.T) {
	// The destination: every submission refused at validation.
	submits := make(chan *messaging.Envelope, 16)
	refuse := func(ctx context.Context, s message.Stream) {
		for {
			m, err := s.Read()
			if err != nil {
				return
			}
			req := m.(*message.Addressed).Message.(*message.SubmitRequest)
			submits <- req.Envelope
			_ = s.Write(&message.SubmitResponse{Value: []*api.Submission{{
				Success: false,
				Message: "Transaction validation failed",
				Status: &protocol.TransactionStatus{Code: errors.Unauthorized,
					Error: errors.Unauthorized.With("key is not an active validator for BVN1")},
			}}})
		}
	}
	d := accumulated.NewDispatcher(t.Name(),
		refusedRouter(func(u *url.URL) (string, error) {
			id, _ := protocol.ParsePartitionUrl(u)
			return id, nil
		}),
		refusedDialer(func(ctx context.Context, _ multiaddr.Multiaddr) (message.Stream, error) {
			p, q := message.DuplexPipe(ctx)
			go refuse(ctx, p)
			return q, nil
		}))
	defer d.Close()

	// The source answers entries 1 and 2 of BVN1's synthetic stream to BVN0.
	var records []*api.MessageRecord[messaging.Message]
	for n := uint64(1); n <= 2; n++ {
		h := reqHeld(n, true)
		sig := &protocol.ED25519Signature{PublicKey: append(make([]byte, 31), byte(n)), Signer: reqSource.JoinPath(protocol.Network)}
		records = append(records, &api.MessageRecord[messaging.Message]{
			Sequence:          h.Message.(*messaging.SequencedMessage),
			SourceReceiptList: reqProof(n, h.Hash),
			SourceAnchorBlock: 50,
			Signatures: &api.RecordRange[*api.SignatureSetRecord]{Records: []*api.SignatureSetRecord{{
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{Records: []*api.MessageRecord[messaging.Message]{
					{Message: &messaging.SignatureMessage{Signature: sig}}}},
			}}},
		})
	}
	ranger := &refusedRanger{records: records}

	// BVN0 holds entry 2 collected and unproven, and is missing entry 1: a gap
	// of two kinds, asked for as one span.
	c := testConductor()
	c.Dispatcher = d
	c.Sequencer = ranger
	c.Staging = reqStaging(t, map[uint64]bool{2: true})
	c.Globals.Store(&network.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest,
		Network: &protocol.NetworkDefinition{Partitions: []*protocol.PartitionInfo{
			{ID: "BVN0", Type: protocol.PartitionTypeBlockValidator},
			{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}}}})
	require.NoError(t, c.Start(events.NewBus(nil)))

	db := database.OpenInMemory(nil)
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		if err := batch.Account(c.Url(protocol.Synthetic)).Main().Put(&protocol.SyntheticLedger{Url: c.Url(protocol.Synthetic)}); err != nil {
			return err
		}
		return batch.Account(c.Url(protocol.AnchorPool)).Main().Put(&protocol.AnchorLedger{Url: c.Url(protocol.AnchorPool)})
	}))
	activation := func(block uint64) {
		batch := db.Begin(false)
		defer batch.Discard()
		require.NoError(t, c.observeStreams(batch, block))
		require.NoError(t, c.requestGaps(context.Background(), batch, block))
	}
	refusedCount := func() float64 {
		return testutil.ToFloat64(mHealRequests.WithLabelValues("refused", "BVN0", "BVN1"))
	}

	// Activate until the requester asks for the span; its envelope goes out
	// and is refused.
	before := refusedCount()
	block := uint64(0)
	for ranger.asks == 0 && block < 100*healCadence {
		block += healCadence
		activation(block)
	}
	require.Equal(t, 1, ranger.asks, "the gap is asked for")
	select {
	case env := <-submits:
		require.NotEmpty(t, env.Messages)
	case <-time.After(5 * time.Second):
		t.Fatal("the heal never reached the destination")
	}
	require.Eventually(t, func() bool { return refusedCount()-before == 1 }, 5*time.Second, 10*time.Millisecond,
		"the requester is told of the refusal: heal_requests_total{outcome=refused}")

	// The next activation asks again: the refusal was a failed heal. Without
	// it the span stays "asked" for healPatience activations, waiting on an
	// answer that the destination has already refused.
	block += healCadence
	activation(block)
	require.Equal(t, 2, ranger.asks, "a refused heal is asked for again at the next activation, not after patience (%d activations)", healPatience)
}

// refusedRanger is a source that answers every synthetic span request with
// fixed records and holds no anchors.
type refusedRanger struct {
	records []*api.MessageRecord[messaging.Message]
	asks    int
}

func (f *refusedRanger) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound
}

func (f *refusedRanger) SequenceRange(_ context.Context, src, _ *url.URL, first, last uint64, _ private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	if !src.Equal(reqSource.JoinPath(protocol.Synthetic)) {
		return nil, errors.NotReady.With("no anchors in flight")
	}
	f.asks++
	return f.records, nil
}

type refusedRouter func(*url.URL) (string, error)

func (f refusedRouter) RouteAccount(u *url.URL) (string, error) { return f(u) }
func (f refusedRouter) Route(env ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(f, env...)
}

type refusedDialer func(context.Context, multiaddr.Multiaddr) (message.Stream, error)

func (fn refusedDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return fn(ctx, addr)
}

// #4426 review F1. Not every envelope the conductor sends to its own
// partition is a heal: the Directory anchors itself, and a validator just
// removed from the committee has its own anchor refused with exactly the
// #4424 text. A refusal of an envelope nothing asked for is the dispatcher's
// to count (refused_total); counted as a heal refusal it reports heals that
// never happened.
func TestRequester_ARefusedEnvelopeThatWasNoHealIsNotAHealRefusal(t *testing.T) {
	c := &Conductor{Partition: &protocol.PartitionInfo{ID: "Directory", Type: protocol.PartitionTypeDirectory}}
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: protocol.DnUrl().JoinPath(protocol.AnchorPool)},
		Body:   &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.DnUrl(), MinorBlockIndex: 9}},
	}
	own := &messaging.Envelope{Messages: []messaging.Message{&messaging.BlockAnchor{
		Signature: &protocol.ED25519Signature{PublicKey: make([]byte, 32), Signer: protocol.DnUrl().JoinPath(protocol.Network)},
		Anchor:    &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.DnUrl(), Destination: protocol.DnUrl(), Number: 9},
	}}}

	counter := mHealRequests.WithLabelValues("refused", "Directory", "Directory")
	before := testutil.ToFloat64(counter)
	c.refused("Directory", own, errors.Unauthorized.With("key is not an active validator for Directory"))
	require.Equal(t, before, testutil.ToFloat64(counter), "the Directory's own anchor was never asked for: not a heal refusal")

	// The same anchor, asked for as a heal, is one.
	stream := execute.StreamID{Ledger: c.Url(protocol.AnchorPool), Source: protocol.DnUrl()}
	c.requester.asked(stream, [2]uint64{9, 9}, 100)
	c.refused("Directory", own, errors.Unauthorized.With("key is not an active validator for Directory"))
	require.Equal(t, before+1, testutil.ToFloat64(counter), "a refused heal is a heal refusal")
}
