// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// acceptAll is an entry classifier for tests that do not care what became of
// an answer's entries.
var acceptAll = entryOutcome(func(uint64) string { return "applied" })

// activateStream is one activation of a stream as the conductor runs it:
// every node observes the stream's stillness (observeStreams), then the
// selected pair asks (requestStream).
func activateStream(c *Conductor, staged *execute.StagingTxn, block uint64, backoff *url.URL, a streamAsk) {
	c.requester.observeStill(a.stream, a.delivered, block)
	c.requestStream(context.Background(), staged, block, backoff, a)
}

// A requester for tests: the partition it asks for, and nothing else.
func testConductor() *Conductor {
	return &Conductor{Partition: &protocol.PartitionInfo{ID: "BVN0", Type: protocol.PartitionTypeBlockValidator}}
}

// A destination whose Directory anchors run late holds every package's
// entries collected with the package's proof waiting in anchor staging. That
// is not a gap: the proof has arrived, the anchor is on its way, and asking
// the source again lands the entries twice (#4229; run 20260905T134346Z).
// Over a 30-block delay the requester makes no request. Once the proof is
// dropped, disproved, the entries are unproven and are asked for.
func TestRequester_LaggingDestination(t *testing.T) {
	const entries = 40
	s := execute.NewStaging()
	tx := s.Begin()
	list := merkle.NewReceiptList()
	list.MerkleState = &merkle.State{Count: 0}
	for n := uint64(1); n <= entries; n++ {
		h := reqHeld(n, true)
		tx.Hold(reqStream, n, h)
		list.Elements = append(list.Elements, h.Hash[:])
	}
	tx.StageProof(reqSource, 50, &protocol.AnnotatedReceipt{Anchor: &protocol.AnchorMetadata{SourceBlock: 50}, ReceiptList: list})
	tx.Commit()

	c := testConductor()
	asks := 0
	ask := streamAsk{stream: reqStream, what: "synthetics", healed: func(int) {},
		ask: func(first, last uint64, _ entryOutcome) (int, uint64, error) {
			asks++
			return int(last - first + 1), last, nil
		}}
	staged := s.Begin()
	defer staged.Discard()
	for block := uint64(healCadence); block <= 30; block += healCadence {
		activateStream(c, staged, block, reqSource, ask)
	}
	require.Zero(t, asks, "entries whose proof is waiting for its anchor are not gaps")

	// The anchor disproves the proof: now the entries are held unproven
	staged.DropProofs(reqSource, 50)
	activateStream(c, staged, 32, reqSource, ask)
	require.Equal(t, 1, asks, "one span for the whole run, once the proof is gone")
}

// A quiet stream is probed — the span above Delivered asked for whole — and
// the source answers "not yet". The probe is remembered like an answer, so it
// fires once per patience window, not every activation (#4229).
//
// It also does not fire until the stream has been empty and still for
// probeAfter activations (#4280), so the first probe is that many in and the
// count is one per patience window thereafter.
func TestRequester_ProbeOncePerPatience(t *testing.T) {
	s := execute.NewStaging()
	c := testConductor()
	asks := 0
	ask := streamAsk{stream: reqStream, what: "synthetics", healed: func(int) {},
		ask: func(first, last uint64, _ entryOutcome) (int, uint64, error) {
			asks++
			return 0, 0, errors.NotReady.With("not produced yet")
		}}
	staged := s.Begin()
	defer staged.Discard()
	const activations = 12
	for i := uint64(1); i <= activations; i++ {
		activateStream(c, staged, i*healCadence, reqSource, ask)
	}
	// Probes land at activations probeAfter, +healPatience, +2*healPatience...
	want := (activations-probeAfter)/healPatience + 1
	require.Equal(t, want, asks, "once per patience window, starting after the stillness")
	require.False(t, c.requester.backedOff(reqSource, activations*healCadence), "not yet is not a failure")
}

// fakeRanger answers a span request with fixed records.
type fakeRanger struct {
	records []*api.MessageRecord[messaging.Message]
}

func (f fakeRanger) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound
}

func (f fakeRanger) SequenceRange(context.Context, *url.URL, *url.URL, uint64, uint64, private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	return f.records, nil
}

// An anchor's signatures travel in one envelope, and anchors share it under
// the budget: two anchors with three signatures each are one envelope of six
// BlockAnchors, not six envelopes (#4229).
func TestRequestAnchorSpan_OneEnvelope(t *testing.T) {
	c := testConductor()
	var envelopes []*messaging.Envelope
	c.Intercept = func(_ context.Context, env *messaging.Envelope) (bool, error) {
		envelopes = append(envelopes, env)
		return false, nil
	}
	var records []*api.MessageRecord[messaging.Message]
	for n := uint64(1); n <= 2; n++ {
		txn := &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)},
			Body: &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.DnUrl(), MinorBlockIndex: n}}}
		r := &api.MessageRecord[messaging.Message]{
			Sequence:   &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.DnUrl(), Destination: protocol.PartitionUrl("BVN0"), Number: n},
			Signatures: &api.RecordRange[*api.SignatureSetRecord]{},
		}
		for i := byte(0); i < 3; i++ {
			sig := &protocol.ED25519Signature{PublicKey: append(make([]byte, 31), i+1), Signer: protocol.DnUrl().JoinPath(protocol.Network), TransactionHash: txn.ID().Hash()}
			sm := &messaging.SignatureMessage{Signature: sig, TxID: txn.ID()}
			r.Signatures.Records = append(r.Signatures.Records, &api.SignatureSetRecord{
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{Records: []*api.MessageRecord[messaging.Message]{{Message: sm}}},
			})
		}
		records = append(records, r)
	}
	n, served, err := c.requestAnchorSpan(context.Background(), fakeRanger{records}, protocol.DnUrl(), 1, 2, acceptAll)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, uint64(2), served)
	require.Len(t, envelopes, 1, "one envelope for the span")
	require.Len(t, envelopes[0].Messages, 6, "every signature of every anchor")
	for _, m := range envelopes[0].Messages {
		require.IsType(t, &messaging.BlockAnchor{}, m)
	}
}

// A stream whose oldest gap the source answers NotFound for, activation after
// activation through the back-off's cap, is stranded: the requester stops
// asking, says so once, and shows it on a gauge. It leaves the state when
// Delivered moves past where it was stranded — sync filled the hole (#4242).
func TestRequester_StrandedStream(t *testing.T) {
	s := execute.NewStaging()
	c := testConductor()
	asks := 0
	ask := streamAsk{stream: reqStream, what: "synthetics", healed: func(int) {},
		ask: func(first, last uint64, _ entryOutcome) (int, uint64, error) {
			asks++
			return 0, 0, errors.NotFound.With("not in the cache")
		}}
	staged := s.Begin()
	defer staged.Discard()

	// Enough activations for the back-off to reach its cap and stay there
	block := uint64(0)
	for i := 0; i < 400; i++ {
		block += healCadence
		activateStream(c, staged, block, reqSource, ask)
	}
	require.Equal(t, strandedAfter, asks, "asked through the back-off, then never again")
	require.Equal(t, []string{streamKey(reqStream)}, c.requester.Stranded())
	g, err := mStrandedStreams.GetMetricWithLabelValues("BVN0", "BVN1")
	require.NoError(t, err)
	require.Equal(t, 1.0, gaugeValue(t, g))

	// The stream moves: sync delivered past the hole
	ask.delivered = 100
	activateStream(c, staged, block+healCadence, reqSource, ask)
	require.Empty(t, c.requester.Stranded(), "Delivered moved past the stranded point")
	require.Equal(t, 0.0, gaugeValue(t, g))

	// It does not ask on that activation. Delivered having just moved is
	// what a draining stream looks like, and the catch-up probe is for the
	// opposite case; it waits for the stream to go still again (probeAfter,
	// #4280). Recovery is not lost, only deferred by a few activations.
	require.Equal(t, strandedAfter, asks, "Delivered just moved: draining, not stuck")
	for i := uint64(0); i < probeAfter; i++ {
		block += healCadence
		activateStream(c, staged, block+healCadence, reqSource, ask)
	}
	require.Equal(t, strandedAfter+1, asks, "still again at the new Delivered: asked")
}

func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	m := new(dto.Metric)
	require.NoError(t, g.Write(m))
	return m.GetGauge().GetValue()
}

// A node lagging consensus -- more than MaxExecutionLag groups behind, the
// same test the primary makes -- asks for nothing: what it lacks is in its own
// unexecuted blocks, and a source would answer NotFound for it, a miss that
// strands the stream for entries that were never missing (#4260). Within the
// window it asks: healing runs at the block-begin hook, where a working
// executor is one behind by construction, and a threshold of zero refused
// every request on a working network and left a real hole permanent (#4284).
func TestRequester_LaggingNodeDoesNotAsk(t *testing.T) {
	anchorStream := execute.StreamID{Ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool), Source: reqSource}
	const max = 8
	// One activation on a fresh conductor, so the patience window from an
	// earlier answer cannot hide an ask: how many requests a node at this lag
	// makes on sight of a HOLE in an anchor stream -- anchor 3 held, anchor 2
	// missing. A hole is asked on sight whatever else is true; only the guard
	// under test can stop it. (An empty anchor stream's probe for the next
	// anchor waits until it is overdue, #4288, so it is not the vehicle.)
	asksAt := func(lag int) int {
		c := testConductor()
		c.SetExecutionLagSource(func() int { return lag }, max)
		asks := 0
		ask := streamAsk{stream: anchorStream, what: "anchors", delivered: 1, healed: func(int) {},
			ask: func(first, last uint64, _ entryOutcome) (int, uint64, error) {
				asks++
				return int(last - first + 1), last, nil
			}}
		staged := execute.NewStaging().Begin()
		defer staged.Discard()
		staged.Hold(anchorStream, 3, reqHeld(3, false))
		activateStream(c, staged, healCadence, reqSource, ask)
		return asks
	}
	require.Zero(t, asksAt(max+1), "beyond MaxExecutionLag: the gap is in our own backlog")
	require.Equal(t, 1, asksAt(1), "one behind is what a working node looks like at the hook: asked")
	require.Equal(t, 1, asksAt(max), "at the threshold, not beyond it: asked")
	require.Equal(t, 1, asksAt(0), "caught up: a hole is asked on sight")
}

// A ranger that answers by node: unaddressed requests come from signer A;
// each peer answers with its own signer. Records the nodes it was asked.
type byNodeRanger struct {
	asked   []string
	signers map[string]byte // node id -> signer index; "" is unaddressed
}

func (r *byNodeRanger) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound
}

func (r *byNodeRanger) SequenceRange(_ context.Context, _, _ *url.URL, first, last uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	r.asked = append(r.asked, string(opts.NodeID))
	who := r.signers[string(opts.NodeID)]
	var out []*api.MessageRecord[messaging.Message]
	for n := first; n <= last; n++ {
		txn := &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.DnUrl().JoinPath(protocol.AnchorPool)},
			Body: &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN1"), MinorBlockIndex: n}}}
		sig := &protocol.ED25519Signature{PublicKey: append(make([]byte, 31), who), Signer: protocol.PartitionUrl("BVN1").JoinPath(protocol.Network), TransactionHash: txn.ID().Hash()}
		out = append(out, &api.MessageRecord[messaging.Message]{
			Sequence: &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.PartitionUrl("BVN1"), Destination: protocol.DnUrl(), Number: n},
			Signatures: &api.RecordRange[*api.SignatureSetRecord]{Total: 1, Records: []*api.SignatureSetRecord{{
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{Records: []*api.MessageRecord[messaging.Message]{{Message: &messaging.SignatureMessage{Signature: sig, TxID: txn.ID()}}}},
			}}},
		})
	}
	return out, nil
}

type fakePeers []string

func (p fakePeers) NodeInfo(context.Context, api.NodeInfoOptions) (*api.NodeInfo, error) {
	return nil, errors.NotAllowed
}

func (p fakePeers) FindService(context.Context, api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	var out []*api.FindServiceResult
	for _, id := range p {
		out = append(out, &api.FindServiceResult{PeerID: peer.ID(id)})
	}
	return out, nil
}

// An anchor answer is one signature from one node, and the anchor needs a
// quorum. The requester asks the source's validators by node until the first
// anchor of the span has enough distinct signers, and no further -- and
// submits them as one envelope. A completely lost block validator anchor
// used to stay lost when the transport kept dialing the same node.
func TestRequestAnchorSpan_GathersAQuorumByNode(t *testing.T) {
	c := testConductor()
	c.Peers = fakePeers{"n1", "n2", "n3"}
	g := new(network.GlobalValues)
	g.Network = &protocol.NetworkDefinition{Partitions: []*protocol.PartitionInfo{{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}}}
	for i := byte(1); i <= 3; i++ {
		g.Network.Validators = append(g.Network.Validators, &protocol.ValidatorInfo{
			PublicKey:  append(make([]byte, 31), i),
			Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}},
		})
	}
	g.Globals = &protocol.NetworkGlobals{ValidatorAcceptThreshold: protocol.Rational{Numerator: 2, Denominator: 3}}
	c.Globals.Store(g)
	require.Equal(t, uint64(2), g.ValidatorThreshold("BVN1"))

	var envelopes []*messaging.Envelope
	c.Intercept = func(_ context.Context, env *messaging.Envelope) (bool, error) {
		envelopes = append(envelopes, env)
		return false, nil
	}
	// Unaddressed and n1 both answer as signer 1 -- the "same node again"
	// case; n2 is signer 2, n3 signer 3.
	r := &byNodeRanger{signers: map[string]byte{"": 1, "n1": 1, "n2": 2, "n3": 3}}
	n, served, err := c.requestAnchorSpan(context.Background(), r, protocol.PartitionUrl("BVN1"), 1, 2, acceptAll)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, uint64(2), served)
	require.Equal(t, []string{"", "n1", "n2"}, r.asked, "asks by node until the quorum, then stops")

	require.Len(t, envelopes, 1, "one envelope")
	signers := map[uint64]map[string]bool{}
	for _, m := range envelopes[0].Messages {
		ba := m.(*messaging.BlockAnchor)
		num := ba.Anchor.(*messaging.SequencedMessage).Number
		if signers[num] == nil {
			signers[num] = map[string]bool{}
		}
		signers[num][string(ba.Signature.GetPublicKey())] = true
	}
	require.Len(t, signers[1], 2, "anchor 1 carries a quorum of distinct signers")
	require.Len(t, signers[2], 2, "and so does anchor 2, from the same answers")
}

// A NotFound taken while the node is behind consensus is not a miss: the span
// may be in its own unexecuted blocks, released at the source on the
// partition's Delivered (#4260). It is remembered like a not-yet answer and
// does not count toward stranding. Caught up, a NotFound means what it says
// and strands the stream as before (#4284).
func TestRequester_MissWhileLaggingIsNotAMiss(t *testing.T) {
	s := execute.NewStaging()
	c := testConductor()
	lag := 1
	c.SetExecutionLagSource(func() int { return lag }, 8)
	asks := 0
	ask := streamAsk{stream: reqStream, what: "synthetics", healed: func(int) {},
		ask: func(first, last uint64, _ entryOutcome) (int, uint64, error) {
			asks++
			return 0, 0, errors.NotFound.With("not in the cache")
		}}
	staged := s.Begin()
	defer staged.Discard()

	block := uint64(0)
	for i := 0; i < 400; i++ {
		block += healCadence
		activateStream(c, staged, block, reqSource, ask)
	}
	require.NotZero(t, asks, "one behind is a working node: it asks")
	require.Empty(t, c.requester.Stranded(), "NotFound while behind consensus does not strand")

	lag = 0
	before := asks
	for i := 0; i < 400; i++ {
		block += healCadence
		activateStream(c, staged, block, reqSource, ask)
	}
	require.Equal(t, strandedAfter, asks-before, "caught up: misses count, and the stream strands")
	require.Equal(t, []string{streamKey(reqStream)}, c.requester.Stranded())
}
