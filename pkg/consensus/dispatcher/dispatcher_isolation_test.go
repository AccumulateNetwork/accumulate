// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dispatcher

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// These tests run the REAL dispatch path — Submit → Send → gossipsub →
// handleInbound → Subscribe — over real libp2p hosts. The existing tests
// declared that path untestable and skipped it; the wrong-DAG episode
// (dn-destined anchors committing inside BVN3's blocks, run 20260820T100912Z)
// is what it costs when the only isolation between partitions is asserted by
// nothing (#4117, #4116).

// isoRouter routes accounts by a fixed map and envelopes by the destination
// of their first sequenced message — the shape every conductor envelope has.
type isoRouter struct{ accounts map[string]string }

func (r *isoRouter) RouteAccount(u *url.URL) (string, error) {
	if p, ok := r.accounts[u.String()]; ok {
		return p, nil
	}
	return "", fmt.Errorf("no route for %v", u)
}

func (r *isoRouter) Route(envs ...*messaging.Envelope) (string, error) {
	for _, env := range envs {
		for _, msg := range env.Messages {
			if seq, ok := msg.(*messaging.SequencedMessage); ok && seq.Destination != nil {
				if part, ok := protocol.ParsePartitionUrl(seq.Destination); ok {
					return part, nil
				}
			}
		}
	}
	return "", fmt.Errorf("unroutable envelope")
}

// isoNode is one partition's dispatcher on its own libp2p host.
type isoNode struct {
	part string
	ps   *pubsub.PubSub
	d    *Dispatcher
}

// newIsoNetwork builds one dispatcher per partition, each on its own host,
// with every host connected to every other.
func newIsoNetwork(t *testing.T, ctx context.Context, router *isoRouter, partitions ...string) map[string]*isoNode {
	t.Helper()

	nodes := make(map[string]*isoNode, len(partitions))
	var hosts []*isoNode
	for _, part := range partitions {
		h := bhost.NewBlankHost(swarmt.GenSwarm(t))
		t.Cleanup(func() { _ = h.Close() })

		ps, err := pubsub.NewGossipSub(ctx, h)
		require.NoError(t, err)

		d, err := NewDispatcherWithOptions(h, ps, router, part, DispatcherOptions{
			SendTimeout: 5 * time.Second,
		})
		require.NoError(t, err)

		n := &isoNode{part: strings.ToLower(part), ps: ps, d: d}
		nodes[n.part] = n
		hosts = append(hosts, n)
	}

	// Full mesh
	for i, a := range hosts {
		ha := a.d.host
		for _, b := range hosts[i+1:] {
			hb := b.d.host
			ha.Peerstore().AddAddrs(hb.ID(), hb.Addrs(), time.Hour)
			require.NoError(t, ha.Connect(ctx, hb.Peerstore().PeerInfo(hb.ID())))
		}
	}

	for _, n := range nodes {
		require.NoError(t, n.d.Start(ctx))
		t.Cleanup(n.d.Close)
	}
	return nodes
}

// seqEnvelope builds a normalizable envelope whose sequenced message is
// destined for the given partition — the shape every conductor dispatch has.
func seqEnvelope(dest string, n uint64) *messaging.Envelope {
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.PartitionUrl(dest).JoinPath("ledger")
	txn.Body = &protocol.SyntheticDepositCredits{}
	return &messaging.Envelope{Messages: []messaging.Message{
		&messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      protocol.PartitionUrl("bvn1"),
			Destination: protocol.PartitionUrl(dest),
			Number:      n,
		},
	}}
}

// drain empties an inbound channel, returning how many envelopes it held.
func drain(ch <-chan *messaging.Envelope) int {
	n := 0
	for {
		select {
		case <-ch:
			n++
		default:
			return n
		}
	}
}

// TestDispatcher_PartitionIsolation is the wrong-DAG pin: an envelope
// submitted for partition X is delivered to X's dispatcher and NEVER to any
// other partition's — and once the gossip mesh is warm, exactly once per
// Send, with the queue fully drained.
func TestDispatcher_PartitionIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	destURL := protocol.PartitionUrl("bvn2").JoinPath("ledger")
	router := &isoRouter{accounts: map[string]string{destURL.String(): "bvn2"}}
	nodes := newIsoNetwork(t, ctx, router, "bvn1", "bvn2", "bvn3")
	a, b, c := nodes["bvn1"], nodes["bvn2"], nodes["bvn3"]

	// Warm the mesh: gossipsub needs time to graft the topic, and dispatch is
	// one-shot by design (a dropped pre-mesh publish is healing's job, not the
	// dispatcher's). Submit+Send until the first envelope lands.
	var seq uint64
	require.Eventually(t, func() bool {
		seq++
		require.NoError(t, a.d.Submit(ctx, destURL, seqEnvelope("bvn2", seq)))
		for err := range a.d.Send(ctx) {
			t.Logf("warmup send error: %v", err)
		}
		select {
		case <-b.d.Subscribe():
			return true
		case <-time.After(250 * time.Millisecond):
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "the envelope never reached its destination partition")

	// Steady state: one Submit, one Send, exactly one delivery.
	drain(b.d.Subscribe())
	require.NoError(t, a.d.Submit(ctx, destURL, seqEnvelope("bvn2", 1000)))
	for err := range a.d.Send(ctx) {
		require.NoError(t, err)
	}
	select {
	case env := <-b.d.Subscribe():
		require.Len(t, env.Messages, 1)
	case <-time.After(5 * time.Second):
		t.Fatal("steady-state envelope not delivered")
	}

	// No duplicate delivery, and the queue is drained: a second Send moves
	// nothing.
	time.Sleep(300 * time.Millisecond)
	require.Zero(t, drain(b.d.Subscribe()), "an envelope must be delivered exactly once per Send")
	for err := range a.d.Send(ctx) {
		require.NoError(t, err)
	}
	time.Sleep(300 * time.Millisecond)
	require.Zero(t, drain(b.d.Subscribe()), "Send must fully drain the queue — nothing to re-send")

	// THE isolation assertion: nothing destined for bvn2 ever surfaced on any
	// other partition, across the entire test including the warmup storm.
	require.Zero(t, drain(c.d.Subscribe()), "an envelope for X must NEVER reach partition Y")
	require.Zero(t, drain(a.d.Subscribe()), "an envelope for X must NEVER reach its sender's own partition")
}

// TestDispatcher_MisroutedInboundDropped: topic membership must not be the
// only isolation. An envelope published directly onto partition X's dispatch
// topic that does not ROUTE to X is dropped before it can reach consensus —
// the defense-in-depth check added after run 20260820T100912Z.
func TestDispatcher_MisroutedInboundDropped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	destURL := protocol.PartitionUrl("bvn2").JoinPath("ledger")
	router := &isoRouter{accounts: map[string]string{destURL.String(): "bvn2"}}
	nodes := newIsoNetwork(t, ctx, router, "bvn1", "bvn2", "bvn9")
	a, b, attacker := nodes["bvn1"], nodes["bvn2"], nodes["bvn9"]

	// A malicious or buggy publisher pushes an envelope destined for bvn3
	// straight onto bvn2's dispatch topic, wrapped exactly like a real
	// dispatch. Use a separate node's pubsub — joining the topic on A's own
	// pubsub would collide with A's dispatcher joining it to send.
	topic, err := attacker.ps.Join(fmt.Sprintf(TopicDispatch, "bvn2"))
	require.NoError(t, err)
	misrouted := seqEnvelope("bvn3", 1)
	data, err := misrouted.MarshalBinary()
	require.NoError(t, err)
	wrapped := attacker.d.wrapMessage(data)

	// Interleave misrouted garbage with legitimate traffic until the
	// legitimate envelope arrives — proving the topic was live while the
	// misrouted envelopes were being dropped.
	var seq uint64
	require.Eventually(t, func() bool {
		require.NoError(t, topic.Publish(ctx, wrapped))
		seq++
		require.NoError(t, a.d.Submit(ctx, destURL, seqEnvelope("bvn2", seq)))
		for range a.d.Send(ctx) {
		}
		select {
		case env := <-b.d.Subscribe():
			// Whatever arrives must route to bvn2 — never the bvn3 envelope.
			part, err := router.Route(env)
			require.NoError(t, err)
			require.Equal(t, "bvn2", part, "a misrouted envelope crossed into the partition")
			return true
		case <-time.After(250 * time.Millisecond):
			return false
		}
	}, 30*time.Second, 10*time.Millisecond)

	// Grace period: any misrouted envelope in flight would surface now.
	time.Sleep(500 * time.Millisecond)
	for {
		select {
		case env := <-b.d.Subscribe():
			part, err := router.Route(env)
			require.NoError(t, err)
			require.Equal(t, "bvn2", part, "a misrouted envelope crossed into the partition")
			continue
		default:
		}
		break
	}
}

// TestDispatcher_MalformedInboundDoesNotKill: garbage on the dispatch topic —
// wrong version, truncated frames, non-envelope payloads — must be dropped
// without killing the inbound handler; legitimate traffic still flows after.
func TestDispatcher_MalformedInboundDoesNotKill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	destURL := protocol.PartitionUrl("bvn2").JoinPath("ledger")
	router := &isoRouter{accounts: map[string]string{destURL.String(): "bvn2"}}
	nodes := newIsoNetwork(t, ctx, router, "bvn1", "bvn2", "bvn9")
	a, b, attacker := nodes["bvn1"], nodes["bvn2"], nodes["bvn9"]

	topic, err := attacker.ps.Join(fmt.Sprintf(TopicDispatch, "bvn2"))
	require.NoError(t, err)
	garbage := [][]byte{
		{},                            // empty
		{0x02, 0x01, 'x', 0, 0, 0, 0}, // wrong version
		{0x01, 0xFF},                  // partition length overruns
		attacker.d.wrapMessage([]byte("not an envelope")), // valid frame, junk payload
	}

	// Interleave garbage with legitimate envelopes until one arrives; the
	// handler surviving the garbage IS the assertion.
	var seq uint64
	require.Eventually(t, func() bool {
		for _, g := range garbage {
			require.NoError(t, topic.Publish(ctx, g))
		}
		seq++
		require.NoError(t, a.d.Submit(ctx, destURL, seqEnvelope("bvn2", seq)))
		for range a.d.Send(ctx) {
		}
		select {
		case <-b.d.Subscribe():
			return true
		case <-time.After(250 * time.Millisecond):
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "the inbound handler did not survive malformed traffic")
}

// TestDispatcher_PublishFailureSurfacesError pins the one-shot dispatch
// contract: a failed publish surfaces on the error channel and the envelope
// is NOT silently retried or requeued — recovery of lost dispatches is
// healing's job (#4105), and a silent drop here is how streams wedge.
func TestDispatcher_PublishFailureSurfacesError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	destURL := protocol.PartitionUrl("bvn2").JoinPath("ledger")
	router := &isoRouter{accounts: map[string]string{destURL.String(): "bvn2"}}
	nodes := newIsoNetwork(t, ctx, router, "bvn1")
	a := nodes["bvn1"]

	require.NoError(t, a.d.Submit(ctx, destURL, seqEnvelope("bvn2", 1)))

	// Sabotage the send: occupy the destination topic on the dispatcher's own
	// pubsub, so its getOrCreateTopic fails with "topic already exists".
	// (A cancelled context does NOT fail a gossipsub publish — publishing is
	// an async enqueue; that non-obvious fact is itself pinned by this test's
	// history, see #4116's assumption ledger.)
	_, err := a.ps.Join(fmt.Sprintf(TopicDispatch, "bvn2"))
	require.NoError(t, err)
	var errs []error
	for err := range a.d.Send(ctx) {
		errs = append(errs, err)
	}
	require.NotEmpty(t, errs, "a failed publish must surface on the error channel, never vanish")

	// One-shot: the failed envelope is gone; the next Send has nothing.
	for err := range a.d.Send(ctx) {
		require.NoError(t, err)
	}
	a.d.queueMu.Lock()
	pending := len(a.d.queue)
	a.d.queueMu.Unlock()
	require.Zero(t, pending, "dispatch is one-shot by contract: failures surface and healing recovers, the queue does not grow")
}
