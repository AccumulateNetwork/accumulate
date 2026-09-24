// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	stderrors "errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The refusal run 20260924T093936Z logged 141 times at the destination: the
// executor's own text for an anchor signed by a follower (#4424).
var errFollowerKey = errors.Unauthorized.With("key is not an active validator for Directory")

// refusingValidator is the executor's pre-batch validation, refusing.
type refusingValidator struct{ err error }

func (v refusingValidator) ValidateTransaction([]byte) error { return v.err }

// newRefusingService is a started single-worker node whose pre-batch
// validation refuses everything with err, behind the production submit
// service.
func newRefusingService(t *testing.T, err error) *SubmitterService {
	t.Helper()
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, e)
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 1}}, 1)
	nodeCfg := consensus.NodeConfig{Partition: "bvn1", KeyPair: priv, NumWorkers: 1}
	nodeCfg.WorkerConfig.Validator = refusingValidator{err}
	node, e := consensus.NewNode(nodeCfg, committee, nil, nil)
	require.NoError(t, e)
	svc, e := NewService(ServiceConfig{
		Partition:  &protocol.PartitionInfo{ID: "bvn1", Type: protocol.PartitionTypeBlockValidator},
		NodeConfig: nodeCfg,
		Adapter:    &commitAdapter{hash: [32]byte{0xAA}},
		EventBus:   events.NewBus(nil),
		Database:   database.OpenInMemory(nil),
	})
	require.NoError(t, e)
	svc.node, svc.committee, svc.ctx = node, committee, context.Background()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = node.Start(ctx) }()
	require.Eventually(t, func() bool {
		return !stderrors.Is(node.SubmitTransaction([]byte("probe")), consensus.ErrNodeNotStarted)
	}, 5*time.Second, 10*time.Millisecond, "node starts")
	return NewSubmitterService(SubmitterServiceParams{Service: svc})
}

// healEnvelope is an anchor heal as the requester builds it: a BlockAnchor
// over the Directory's anchor #7 to BVN1.
func healEnvelope() *messaging.Envelope {
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: protocol.PartitionUrl("BVN1").JoinPath(protocol.AnchorPool)},
		Body:   &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.DnUrl(), MinorBlockIndex: 7}},
	}
	sig := &protocol.ED25519Signature{PublicKey: make([]byte, 32), Signer: protocol.DnUrl().JoinPath(protocol.Network), TransactionHash: txn.ID().Hash()}
	return &messaging.Envelope{Messages: []messaging.Message{&messaging.BlockAnchor{
		Signature: sig,
		Anchor:    &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.DnUrl(), Destination: protocol.PartitionUrl("BVN1"), Number: 7},
	}}}
}

// #4426. A submission the destination's validation refuses is answered as the
// refusal: its code and its message, never Pending with no error. Answered as
// Pending, the dispatcher settled a refused heal as sent and the requester
// believed it landed — 141 refusals, 0 "Destination refused" lines on run
// 20260924T093936Z.
func TestSubmitter_AValidationRefusalIsAnsweredAsTheRefusal(t *testing.T) {
	sub := newRefusingService(t, errFollowerKey)

	res, err := sub.Submit(context.Background(), healEnvelope(), api.SubmitOptions{})
	require.NoError(t, err, "the refusal is an answer, per submission")
	require.Len(t, res, 1)
	require.False(t, res[0].Success)
	require.NotNil(t, res[0].Status)
	require.NotEqual(t, errors.Pending, res[0].Status.Code, "a refusal is not Pending")
	require.Equal(t, errors.Unauthorized, res[0].Status.Code, "the executor's code survives")
	refusal := res[0].Status.AsError()
	require.Error(t, refusal, "a client reading the status must see the refusal")
	require.Contains(t, refusal.Error(), "key is not an active validator for Directory")
}

// #4426, through the production wiring: the node's dispatcher, the routed
// transport, and the submit service registered as production registers it
// (message.Submitter over SubmitterService). The destination's refusal must
// reach the dispatcher as a refusal — logged "Destination refused", counted
// refused, and handed to whoever registered for refusals (the conductor) —
// and not as sent.
func TestDispatcher_ADestinationRefusalIsARefusal(t *testing.T) {
	sub := newRefusingService(t, errFollowerKey)
	handler, err := message.NewHandler(message.Submitter{Submitter: sub})
	require.NoError(t, err)

	var logs syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	const dest = "refusal4426"
	d := accumulated.NewDispatcher(t.Name(),
		refusalRouter(func(*url.URL) (string, error) { return dest, nil }),
		refusalDialer(func(ctx context.Context, _ multiaddr.Multiaddr) (message.Stream, error) {
			p, q := message.DuplexPipe(ctx)
			go handler.Handle(p)
			return q, nil
		}))
	defer d.Close()

	type refusal struct {
		partition string
		env       *messaging.Envelope
		err       error
	}
	got := make(chan refusal, 1)
	d.OnRefused(func(partition string, env *messaging.Envelope, err error) {
		got <- refusal{partition, env, err}
	})

	sent0, refused0 := dispatcherCount(t, "sent_total", dest), dispatcherCount(t, "refused_total", dest)
	env := healEnvelope()
	require.NoError(t, d.Submit(context.Background(), protocol.PartitionUrl("BVN1"), env))
	for range d.Send(context.Background()) { //nolint:revive // drain
	}

	var r refusal
	select {
	case r = <-got:
	case <-time.After(5 * time.Second):
		t.Fatalf("the refusal never reached the dispatcher's refusal hook; logs:\n%s", logs.String())
	}
	require.Equal(t, dest, r.partition)
	require.True(t, env.Equal(r.env), "the hook is handed the envelope that was refused")
	require.Equal(t, errors.Unauthorized, errors.Code(r.err))
	require.Contains(t, r.err.Error(), "key is not an active validator for Directory")

	require.Equal(t, float64(1), dispatcherCount(t, "refused_total", dest)-refused0, "counted refused")
	require.Equal(t, float64(0), dispatcherCount(t, "sent_total", dest)-sent0, "not counted sent")
	require.Contains(t, logs.String(), "Destination refused a dispatched envelope")
}

// dispatcherCount reads accumulate_dispatcher_<name>{destination=dest} from
// the default registry: the dispatcher's own counters, as the soak scrapes
// them.
func dispatcherCount(t *testing.T, name, dest string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != "accumulate_dispatcher_"+name {
			continue
		}
		for _, m := range f.Metric {
			for _, l := range m.Label {
				if l.GetName() == "destination" && l.GetValue() == dest {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Clone(s.b.String())
}

type refusalRouter func(*url.URL) (string, error)

func (f refusalRouter) RouteAccount(u *url.URL) (string, error) { return f(u) }
func (f refusalRouter) Route(env ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(f, env...)
}

type refusalDialer func(context.Context, multiaddr.Multiaddr) (message.Stream, error)

func (fn refusalDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return fn(ctx, addr)
}
