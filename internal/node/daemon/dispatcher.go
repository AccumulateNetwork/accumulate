// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// dispatcher implements [execute.Dispatcher] with one outbound queue per
// destination partition, each owned by its own goroutine (#4222).
//
// The dispatcher is isolated from the data being dispatched: a block hands its
// envelopes over with Submit and is done; Send marks the block boundary and
// reports nothing. Each destination's queue sends on its own stream with its
// own per-attempt deadline, so one unreachable partition never delays another,
// and a write that fails is retried with back-off until the envelope's retry
// deadline. What the dispatcher drops — a destination unreachable past the
// deadline, or a queue past its block bound — is counted, and the cache and
// the requester recover it (executor.md, "Dispatch"; healing.md).
type dispatcher struct {
	network string
	router  routing.Router
	dialer  message.Dialer
	timing  dispatchTiming

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	block  uint64 // block generation; Send advances it
	queues map[string]*destQueue
}

// dispatchTiming is the retry schedule of a destination queue.
type dispatchTiming struct {
	attemptTimeout time.Duration // one stream round trip
	minBackoff     time.Duration // wait after the first failed attempt
	maxBackoff     time.Duration // wait never grows past this
	retryDeadline  time.Duration // an envelope older than this is dropped
}

// The retry schedule is a few blocks' worth. An attempt that has not answered
// in five block intervals is not going to; an envelope not delivered in
// sixteen is dropped, because by then the destination's healing has asked for
// it (healing.md), and holding it longer only holds memory.
var defaultDispatchTiming = dispatchTiming{
	attemptTimeout: 5 * time.Second,
	minBackoff:     250 * time.Millisecond,
	maxBackoff:     2 * time.Second,
	retryDeadline:  16 * time.Second,
}

// dispatchQueueBlocks bounds a destination's queue in blocks: envelopes from
// more than this many blocks ago are dropped, oldest first, when a new block
// begins. A count is not a bound on a block's output, but a block's output is
// bounded by the block, so blocks are the unit.
const dispatchQueueBlocks = 8

// Drop reasons, the reason label of dispatchDrops.
const (
	dropDeadline  = "deadline"
	dropQueueFull = "queue-full"
)

var (
	dispatchQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "accumulate", Subsystem: "dispatcher", Name: "queue_depth",
		Help: "Envelopes queued for a destination partition, waiting to be sent or retried",
	}, []string{"destination"})
	dispatchSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "dispatcher", Name: "sent_total",
		Help: "Envelopes the destination accepted",
	}, []string{"destination"})
	dispatchRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "dispatcher", Name: "retries_total",
		Help: "Envelope send attempts that failed and were retried",
	}, []string{"destination"})
	dispatchRefused = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "dispatcher", Name: "refused_total",
		Help: "Envelopes the destination refused as invalid; not retried",
	}, []string{"destination"})
	dispatchDrops = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "dispatcher", Name: "drops_total",
		Help: "Envelopes dropped undelivered, by reason: deadline (retries exhausted) or queue-full (block bound)",
	}, []string{"destination", "reason"})
)

var _ execute.Dispatcher = (*dispatcher)(nil)

// NewDispatcher creates a new dispatcher.
func NewDispatcher(network string, router routing.Router, dialer message.Dialer) *dispatcher {
	d := new(dispatcher)
	d.network = network
	d.router = router
	d.dialer = dialer
	d.timing = defaultDispatchTiming
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.queues = map[string]*destQueue{}
	return d
}

// Close stops every destination queue. Envelopes still queued are abandoned;
// the node is shutting down.
func (d *dispatcher) Close() {
	d.cancel()
	d.wg.Wait()
}

// Submit routes the account URL and hands the envelope to the destination's
// queue. It returns once the envelope is queued; delivery is the queue's.
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
	// A panic here takes down the node — observed twice from the conductor's
	// healing path during post-fast-sync replay (#4058). Report the problem
	// instead; the caller logs it and healing retries.
	if u == nil {
		return errors.InternalError.With("cannot submit: no destination")
	}
	if d.router == nil {
		return errors.InternalError.With("cannot submit: router not set")
	}

	// If there's something wrong with the envelope, it's better for that error
	// to be logged closer to the source, at the sending side instead of the
	// receiving side
	_, err := env.Normalize()
	if err != nil {
		return err
	}

	// Route the account
	partition, err := d.router.RouteAccount(u)
	if err != nil {
		return err
	}

	// Construct the multiaddr, /acc/{network}/acc-svc/submit:{partition}
	addr, err := api.ServiceTypeSubmit.AddressFor(partition).MultiaddrFor(d.network)
	if err != nil {
		return err
	}

	d.mu.Lock()
	q, ok := d.queues[partition]
	if !ok {
		q = d.newQueue(partition, addr)
		d.queues[partition] = q
	}
	block := d.block
	d.mu.Unlock()

	q.enqueue(&queued{
		req:      &message.Addressed{Address: addr, Message: &message.SubmitRequest{Envelope: env}},
		block:    block,
		enqueued: time.Now(),
	})
	return nil
}

// Send marks a block boundary: it advances the block generation every queue
// is bounded by, wakes the queues, and returns a closed channel. Failures are
// the queues' to retry, count and log; nothing waits on a dispatch
// (executor.md, "Dispatch").
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	d.mu.Lock()
	d.block++
	block := d.block
	queues := make([]*destQueue, 0, len(d.queues))
	for _, q := range d.queues {
		queues = append(queues, q)
	}
	d.mu.Unlock()

	for _, q := range queues {
		q.newBlock(block)
	}

	errs := make(chan error)
	close(errs)
	return errs
}

// queueDepth is the number of envelopes waiting for a destination.
func (d *dispatcher) queueDepth(partition string) int {
	d.mu.Lock()
	q := d.queues[partition]
	d.mu.Unlock()
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// A queued envelope: the addressed request, the block that produced it, and
// its retry history.
type queued struct {
	req      *message.Addressed
	block    uint64
	enqueued time.Time
	attempts int
}

// destQueue is the outbound queue of one destination partition, drained by
// one goroutine.
type destQueue struct {
	d         *dispatcher
	partition string
	addr      multiaddr.Multiaddr
	wake      chan struct{}

	mu    sync.Mutex
	items []*queued
}

func (d *dispatcher) newQueue(partition string, addr multiaddr.Multiaddr) *destQueue {
	q := &destQueue{d: d, partition: partition, addr: addr, wake: make(chan struct{}, 1)}
	d.wg.Add(1)
	go q.run()
	return q
}

func (q *destQueue) enqueue(item *queued) {
	q.mu.Lock()
	q.items = append(q.items, item)
	dispatchQueueDepth.WithLabelValues(q.partition).Set(float64(len(q.items)))
	q.mu.Unlock()
	q.signal()
}

func (q *destQueue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// newBlock applies the block bound: envelopes from blocks older than
// dispatchQueueBlocks are dropped, oldest first, and counted.
func (q *destQueue) newBlock(block uint64) {
	if block > dispatchQueueBlocks {
		q.mu.Lock()
		cutoff := block - dispatchQueueBlocks
		n := 0
		for n < len(q.items) && q.items[n].block < cutoff {
			n++
		}
		if n > 0 {
			dropped := q.items[:n]
			q.items = append([]*queued(nil), q.items[n:]...)
			dispatchQueueDepth.WithLabelValues(q.partition).Set(float64(len(q.items)))
			dispatchDrops.WithLabelValues(q.partition, dropQueueFull).Add(float64(n))
			slog.Warn("Dispatch queue past its block bound, dropping oldest",
				"module", "dispatcher", "destination", q.partition, "dropped", n,
				"oldestBlock", dropped[0].block, "block", block)
		}
		q.mu.Unlock()
	}
	q.signal()
}

// run drains the queue: an attempt sends everything queued on one stream,
// settles what the destination answered, and keeps the rest for a retry after
// a back-off. It exits when the dispatcher closes.
func (q *destQueue) run() {
	defer q.d.wg.Done()
	backoff := q.d.timing.minBackoff
	for {
		select {
		case <-q.d.ctx.Done():
			return
		case <-q.wake:
		}

		for {
			batch := q.snapshot()
			if len(batch) == 0 {
				break
			}
			if q.attempt(batch) {
				backoff = q.d.timing.minBackoff
				continue
			}
			// Something is still queued and the destination did not take
			// it: wait, then try again — unless the dispatcher closes.
			select {
			case <-q.d.ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff *= 2; backoff > q.d.timing.maxBackoff {
				backoff = q.d.timing.maxBackoff
			}
		}
	}
}

func (q *destQueue) snapshot() []*queued {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]*queued(nil), q.items...)
}

// attempt sends the batch on one stream. It returns true when every envelope
// of the batch was settled — accepted, refused, or dropped — and false when
// some remain queued for a retry.
func (q *destQueue) attempt(batch []*queued) bool {
	ctx, cancel := context.WithTimeout(q.d.ctx, q.d.timing.attemptTimeout)
	defer cancel()

	byReq := make(map[message.Message]*queued, len(batch))
	reqs := make([]message.Message, 0, len(batch))
	for _, item := range batch {
		item.attempts++
		byReq[item.req] = item
		reqs = append(reqs, item.req)
	}

	settled := make(map[*queued]bool, len(batch))
	settle := func(item *queued, outcome string, err error) {
		settled[item] = true
		switch outcome {
		case "sent":
			dispatchSent.WithLabelValues(q.partition).Inc()
		case "refused":
			dispatchRefused.WithLabelValues(q.partition).Inc()
			slog.Warn("Destination refused a dispatched envelope",
				"module", "dispatcher", "destination", q.partition, "error", err,
				"kind", envelopeKind(item.req.Message.(*message.SubmitRequest).Envelope))
		}
	}

	// Create a client using a batch dialer, but DO NOT set the router - all
	// the messages are already addressed
	tr := new(message.RoutedTransport)
	tr.Dialer = message.BatchDialer(ctx, q.d.dialer)

	err := tr.RoundTrip(ctx, reqs, func(res, req message.Message) error {
		item := byReq[req]
		switch res := res.(type) {
		case *message.ErrorResponse:
			if retryable(res.Error) {
				return nil // keep it queued
			}
			settle(item, "refused", res.Error)
			return nil

		case *message.SubmitResponse:
			var refused error
			for _, sub := range res.Value {
				err := sub.Status.AsError()
				if err == nil || errors.Code(err) == errors.Delivered {
					continue
				}
				if retryable(err) {
					return nil // keep the envelope queued
				}
				refused = err
			}
			if refused != nil {
				settle(item, "refused", refused)
			} else {
				settle(item, "sent", nil)
			}
			return nil

		default:
			return errors.Conflict.WithFormat("invalid response: want %T, got %T", (*message.SubmitResponse)(nil), res)
		}
	})

	// Remove what settled; count and drop what is past its deadline; the rest
	// waits for the retry.
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := q.items[:0]
	retried, dropped := 0, 0
	for _, item := range q.items {
		switch {
		case settled[item]:
		case byReq[item.req] == nil:
			kept = append(kept, item) // enqueued after the snapshot
		case now.Sub(item.enqueued) > q.d.timing.retryDeadline:
			dropped++
			slog.Warn("Dropping an envelope the destination never took",
				"module", "dispatcher", "destination", q.partition, "attempts", item.attempts,
				"age", now.Sub(item.enqueued).Round(time.Millisecond), "error", err,
				"kind", envelopeKind(item.req.Message.(*message.SubmitRequest).Envelope))
		default:
			retried++
			kept = append(kept, item)
		}
	}
	for i := len(kept); i < len(q.items); i++ {
		q.items[i] = nil
	}
	q.items = kept
	dispatchQueueDepth.WithLabelValues(q.partition).Set(float64(len(q.items)))
	if dropped > 0 {
		dispatchDrops.WithLabelValues(q.partition, dropDeadline).Add(float64(dropped))
	}
	if retried > 0 {
		dispatchRetries.WithLabelValues(q.partition).Add(float64(retried))
		if err != nil {
			slog.Debug("Dispatch attempt failed, will retry",
				"module", "dispatcher", "destination", q.partition, "queued", retried, "error", err)
		}
	}
	return retried == 0
}

// retryable decides whether a destination's answer means "not now" (retry) or
// "not ever" (refused). A client error is the destination saying the envelope
// is invalid; everything else — back-pressure, a full store, an internal
// error, an unknown failure — is the destination's state, not the envelope's.
func retryable(err error) bool {
	if err == nil {
		return false
	}
	// The worker's queue-full refusal crosses the wire as text (#4115).
	if strings.Contains(err.Error(), "worker backpressure") {
		return true
	}
	return !errors.Code(err).IsClientError()
}

// envelopeKind names what an envelope carries, for a log line.
func envelopeKind(env *messaging.Envelope) string {
	if env == nil || len(env.Messages) == 0 {
		return "envelope"
	}
	return env.Messages[0].Type().String()
}
