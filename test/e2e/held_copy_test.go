// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multi "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4423: BVN1→BVN3 froze at 3857 with an entry held and validated at every
// number above it (waiting=0). The entry at the head had been collected — it
// arrived before the Directory anchor its proof names — and after the anchor
// validated it, a byte-identical second copy was committed while the stream
// still had a hole below it. The copy was recorded Delivered; the collected
// original has the same hash, so when its number came up it answered "already
// delivered" and executed nothing, every block, on every node.
//
// These tests drive that through the wiring a node uses. Every node dispatches
// through the daemon's dispatcher (internal/node/daemon), over the simulator's
// routing and service dialer, with the network between them made to lose,
// delay and repeat what the dispatcher sends. The one thing a test does by
// hand is decide what the network does to a request.

// TestHeldCopy_DispatcherRetryAfterABadDial_DoesNotWedgeTheStream is the run's
// shape: the destination takes an envelope, the answer is lost (a bad dial
// after the send), and the dispatcher sends the same envelope again. The
// network delays that retry until the anchor has validated the collected
// entry, and the stream has a hole below it throughout.
func TestHeldCopy_DispatcherRetryAfterABadDial_DoesNotWedgeTheStream(t *testing.T) {
	x := newHeldCopyNet(t)

	// k+1 and k+2, one package: its first send is taken and its answer lost;
	// the dispatcher's retry is held in the network.
	x.net.setNext(func(n uint64) { x.net.loseFirstAnswer, x.net.holdRepeats = n, n })
	x.send(2)
	x.until(100, "the source dispatches k+1 and the answer is lost", func() bool {
		x.net.mu.Lock()
		defer x.net.mu.Unlock()
		return x.net.lostAnswer
	})
	// The dispatcher backs off before it retries (250ms), in real time. The
	// blocks wait for it, as a node's do not have to: stepping on would carry
	// the envelope past the dispatcher's block bound, and it would be dropped
	// rather than retried.
	next := x.net.nextNum()
	require.Eventually(t, func() bool { return x.net.writtenAlone(next) >= 2 }, 10*time.Second, 10*time.Millisecond,
		"the dispatcher retries k+1 after the lost answer")
	x.requireCollected()

	x.releaseAnchors()

	// The retry reaches the destination now, while k is still missing.
	retries := x.net.release(x.net.dispatchedCopies(next))
	require.NotEmpty(t, retries, "the retry was held")
	for _, r := range retries {
		requireAccepted(t, r)
	}
	x.stepN(3)

	x.releaseHoleAndRequireDelivered()
}

// TestHeldCopy_HealAnswerFromTheOriginalSigner_DoesNotWedgeTheStream is the
// heal-answer shape. A heal answer carries the source validator's signed
// message as the source holds it, and ed25519 is deterministic, so two answers
// for the same entry from the same signer are byte-identical. The first is
// collected ahead of its anchor; the second lands after the anchor validated
// it, while the hole below stays open.
func TestHeldCopy_HealAnswerFromTheOriginalSigner_DoesNotWedgeTheStream(t *testing.T) {
	x := newHeldCopyNet(t)

	// k+1 and k+2 reach the destination only as a heal answer: every
	// dispatched copy is held in the network.
	x.net.setNext(func(n uint64) { x.net.holdNumbers[n] = true })
	x.send(2)
	x.until(100, "the source dispatches k+1", func() bool { n := x.net.nextNum(); return n > x.k && x.net.heldCount(n) >= 1 })
	next := x.net.nextNum()

	answer := x.healAnswer(next, next+1)
	x.submit(answer)
	x.stepN(2)
	x.requireCollected()

	x.releaseAnchors()

	// The same answer again. The source signs each answer when it gives it
	// (sequencer_cache.go signRecord, SetTimestampToNow), so two answers are
	// byte-identical only within one millisecond; what repeats an answer
	// byte for byte is its submission being sent twice — the requester
	// submits through the dispatcher, which resends after a bad dial.
	again := x.healAnswer(next, next+1)
	require.NotEqual(t, answer.Messages[1].Hash(), again.Messages[1].Hash(), "a fresh answer is signed afresh")
	x.submit(answer)
	x.stepN(3)

	x.releaseHoleAndRequireDelivered()
}

type heldCopyNet struct {
	t          *testing.T
	sim        *Sim
	net        *lossyNet
	alice      []byte
	aliceUrl   *url.URL
	bobUrl     *url.URL
	src, dst   *url.URL
	stream     execute.StreamID
	timestamp  uint64
	k          uint64
	dispatched []interface{ Close() }
}

// newHeldCopyNet builds a network whose nodes dispatch through the daemon's
// dispatcher over a lossy network, sends one deposit from Alice's partition to
// Bob's, and holds it in the network: k, the hole that stays open until the
// end.
func newHeldCopyNet(t *testing.T) *heldCopyNet {
	x := &heldCopyNet{t: t}
	x.net = &lossyNet{holdNumbers: map[uint64]bool{}, writes: map[uint64]int{}}
	var mu sync.Mutex
	x.sim = NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
		simulator.DispatchWith(func(network string, router routing.Router, dialer message.Dialer) multi.Dispatcher {
			x.net.mu.Lock()
			x.net.dialer = dialer
			x.net.mu.Unlock()
			d := accumulated.NewDispatcher(network, router, x.net)
			mu.Lock()
			x.dispatched = append(x.dispatched, d)
			mu.Unlock()
			return d
		}),
	)
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, d := range x.dispatched {
			d.Close()
		}
	})

	alice := acctesting.GenerateKey("Alice")
	x.alice = alice
	x.aliceUrl = acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := x.sim.Router().RouteAccount(x.aliceUrl)
	require.NoError(t, err)
	for i := 0; ; i++ {
		x.bobUrl = acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Bob", i))
		bobPart, err := x.sim.Router().RouteAccount(x.bobUrl)
		require.NoError(t, err)
		if bobPart != alicePart {
			x.dst = protocol.PartitionUrl(bobPart)
			break
		}
	}
	x.src = protocol.PartitionUrl(alicePart)
	x.stream = execute.StreamID{Ledger: x.dst.JoinPath(protocol.Synthetic), Source: x.src}
	x.net.mu.Lock()
	x.net.src, x.net.dst = x.src, x.dst
	x.net.mu.Unlock()
	MakeLiteTokenAccount(t, x.sim.DatabaseFor(x.aliceUrl), alice[32:], protocol.AcmeUrl())
	x.stepN(5)

	// k: the first deposit is held in the network, dispatched copies and heal
	// answers alike, so the stream has a hole below everything after it.
	x.net.setNext(func(n uint64) { x.net.holdNumbers[n] = true })
	x.send(1)
	x.until(100, "the source dispatches k", func() bool { return x.net.nextNum() != 0 && x.net.heldCount(x.net.nextNum()) >= 1 })
	x.k = x.net.nextNum()

	// From here the destination gets no Directory anchors, so what follows
	// arrives ahead of the anchor that proves it and is collected.
	x.net.mu.Lock()
	x.net.holdAnchors = true
	x.net.mu.Unlock()
	return x
}

// send submits n deposits from Alice to Bob in one block. One travels with
// its own receipt; two or more travel as a package under one collection
// proof, which is what staging keeps for a collected entry.
func (x *heldCopyNet) send(n int) {
	x.t.Helper()
	for i := 0; i < n; i++ {
		x.sim.SubmitTxnSuccessfully(MustBuild(x.t,
			build.Transaction().For(x.aliceUrl).
				SendTokens(1, protocol.AcmePrecisionPower).To(x.bobUrl).
				SignWith(x.aliceUrl).Version(1).Timestamp(&x.timestamp).PrivateKey(x.alice)))
	}
}

// step lets the dispatchers' goroutines run between blocks: they send on
// their own schedule, as a node's do, and a test that steps faster than they
// send outruns their block bound.
func (x *heldCopyNet) step() {
	x.sim.Step()
	time.Sleep(5 * time.Millisecond)
}

func (x *heldCopyNet) stepN(n int) {
	for i := 0; i < n; i++ {
		x.step()
	}
}

func (x *heldCopyNet) until(n int, what string, cond func() bool) {
	x.t.Helper()
	for i := 0; i < n && !cond(); i++ {
		x.step()
	}
	require.True(x.t, cond(), what)
}

func (x *heldCopyNet) staging(fn func(*execute.StagingTxn)) {
	tx := x.sim.S.StagingFor(x.dst).Begin()
	defer tx.Discard()
	fn(tx)
}

// requireCollected: k+1 is held at the destination as a collected entry, its
// proof not yet validated, and k is still missing.
func (x *heldCopyNet) requireCollected() {
	x.t.Helper()
	next := x.net.nextNum()
	x.until(20, "k+1 is collected at the destination", func() bool {
		var ok bool
		x.staging(func(tx *execute.StagingTxn) {
			h, held := tx.IDOf(x.stream, next)
			ok = held && h.Collected
		})
		return ok
	})
	x.staging(func(tx *execute.StagingTxn) {
		require.False(x.t, tx.IsValidated(x.stream, next, x.net.seq(next).Hash()), "k+1 is not validated: its anchor is held")
		_, held := tx.IDOf(x.stream, x.k)
		require.False(x.t, held, "k is missing")
	})
}

// releaseAnchors delivers the Directory anchors the network held, and waits
// for the destination to validate k+1 with them.
func (x *heldCopyNet) releaseAnchors() {
	x.t.Helper()
	x.net.mu.Lock()
	x.net.holdAnchors = false
	x.net.mu.Unlock()
	for _, r := range x.net.release(func(e *messaging.Envelope) bool { return x.net.anchorToDst(e) }) {
		requireAccepted(x.t, r)
	}
	next := x.net.nextNum()
	x.until(100, "the anchor validates k+1", func() bool {
		var ok bool
		x.staging(func(tx *execute.StagingTxn) { ok = tx.IsValidated(x.stream, next, x.net.seq(next).Hash()) })
		return ok
	})
	delivered, _ := x.delivered()
	require.Less(x.t, delivered, x.k, "k is still missing")
}

// releaseHoleAndRequireDelivered delivers k, and requires the stream to move
// through k+1 and k+2: the collected entries must execute.
//
// Only k's own dispatched envelope is delivered. The requester's heal answers
// for the hole stay held: they were asked while k+1 was not yet held, so they
// carry fresh copies of k+1 and k+2, signed at answer time, which would
// execute in the collected entries' place and hide the wedge. In the run the
// requester never asked for a number it held (accountedFor), so no such copy
// came.
func (x *heldCopyNet) releaseHoleAndRequireDelivered() {
	x.t.Helper()
	only := func(e *messaging.Envelope) bool {
		nums := x.net.numbers(e)
		return len(nums) == 1 && nums[0] == x.k
	}
	released := x.net.release(only)
	require.NotEmpty(x.t, released, "k's dispatched envelope was held")
	for _, r := range released {
		requireAccepted(x.t, r)
	}
	x.until(50, "k delivers", func() bool { d, _ := x.delivered(); return d >= x.k })

	next := x.net.nextNum()
	x.until(50, "k+1 and k+2 deliver: a held copy must not wedge the stream", func() bool {
		d, _ := x.delivered()
		return d >= next+1
	})
}

func (x *heldCopyNet) delivered() (uint64, uint64) {
	received, delivered := streamLag(x.t, x.sim, x.dst, x.src)
	return delivered, received
}

// healAnswer is what the destination's requester submits for entries
// [first, last]: the source's answer through the private sequencer API,
// wrapped as requestSpanTo wraps it — the collection proof, then each signed
// message as the source holds it, with its transaction.
func (x *heldCopyNet) healAnswer(first, last uint64) *messaging.Envelope {
	x.t.Helper()
	// The source answers only once the entries are out of its in-flight
	// window (#4248), as it would the requester.
	var records []*api.MessageRecord[messaging.Message]
	x.until(30, "the source answers for k+1", func() bool {
		var err error
		records, err = x.sim.S.Services().Private().(private.SequenceRanger).SequenceRange(context.Background(), x.src.JoinPath(protocol.Synthetic), x.dst, first, last, private.SequenceOptions{})
		return err == nil && len(records) == int(last-first+1)
	})
	tail := records[len(records)-1]
	require.NotNil(x.t, tail.SourceReceiptList)
	proof := &protocol.AnnotatedReceipt{
		ReceiptList: tail.SourceReceiptList,
		Anchor:      &protocol.AnchorMetadata{Account: protocol.DnUrl(), SourceBlock: tail.SourceAnchorBlock},
	}
	msgs := []messaging.Message{&messaging.SyntheticProof{Proof: proof}}
	for _, r := range records {
		require.NotNil(x.t, r.Sequence)
		var sig protocol.KeySignature
		for _, set := range r.Signatures.Records {
			for _, s := range set.Signatures.Records {
				if sm, ok := s.Message.(*messaging.SignatureMessage); ok {
					if ks, ok := sm.Signature.(protocol.KeySignature); ok && sig == nil {
						sig = ks
					}
				}
			}
		}
		require.NotNil(x.t, sig, "the source holds its validator's signature")
		msgs = append(msgs, &messaging.SyntheticMessage{Message: r.Sequence, Signature: sig})
		if r.Companion != nil {
			msgs = append(msgs, r.Companion)
		}
	}
	return &messaging.Envelope{Messages: msgs}
}

// submit sends an envelope to the destination through the routed client, as
// the requester's does.
func (x *heldCopyNet) submit(env *messaging.Envelope) {
	x.t.Helper()
	subs, err := x.sim.S.Services().Submit(context.Background(), env, api.SubmitOptions{})
	require.NoError(x.t, err)
	for _, s := range subs {
		require.NoError(x.t, s.Status.AsError())
	}
}

func requireAccepted(t *testing.T, r message.Message) {
	t.Helper()
	res, ok := r.(*message.SubmitResponse)
	require.True(t, ok, "the destination answered %T", r)
	for _, s := range res.Value {
		if err := s.Status.AsError(); err != nil {
			require.Equal(t, errors.Delivered, errors.Code(err), "the destination took it: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------

// lossyNet is the network between the dispatchers and the destination's
// submit service. It forwards what it is not told to touch; it holds a
// request (answers it as taken, delivers it when released); and it can lose
// the answer to one request after the destination took it, which is a bad
// dial after a send.
type lossyNet struct {
	mu     sync.Mutex
	dialer message.Dialer

	src, dst    *url.URL
	holdAnchors bool
	holdNumbers map[uint64]bool

	// next is the first src→dst number seen after setNext armed it, and
	// onNext what to do about it.
	armed  bool
	next   uint64
	onNext func(uint64)
	seqs   map[uint64]*messaging.SequencedMessage

	loseFirstAnswer uint64
	lostAnswer      bool
	holdRepeats     uint64

	writes map[uint64]int
	alone  map[uint64]int
	held   []*message.Addressed
}

func (n *lossyNet) setNext(fn func(uint64)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.armed, n.onNext = true, fn
}

func (n *lossyNet) nextNum() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.next
}

func (n *lossyNet) seq(num uint64) *messaging.SequencedMessage {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.seqs[num]
}

func (n *lossyNet) written(num uint64) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.writes[num]
}

// writtenAlone counts the envelopes written that carry num and nothing below
// it: the dispatched package, not a heal answer that starts at the hole.
func (n *lossyNet) writtenAlone(num uint64) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.alone[num]
}

// dispatchedCopies matches held envelopes carrying num and nothing below it.
func (n *lossyNet) dispatchedCopies(num uint64) func(*messaging.Envelope) bool {
	return func(e *messaging.Envelope) bool {
		nums := n.numbers(e)
		return len(nums) > 0 && nums[0] == num
	}
}

func (n *lossyNet) heldCount(num uint64) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, r := range n.held {
		for _, m := range n.numbers(envelopeOf(r)) {
			if m == num {
				c++
			}
		}
	}
	return c
}

func (n *lossyNet) carrying(num uint64) func(*messaging.Envelope) bool {
	return func(e *messaging.Envelope) bool {
		for _, m := range n.numbers(e) {
			if m == num {
				return true
			}
		}
		return false
	}
}

func envelopeOf(r *message.Addressed) *messaging.Envelope {
	if sub, ok := r.Message.(*message.SubmitRequest); ok {
		return sub.Envelope
	}
	return nil
}

// numbers are the src→dst synthetic numbers an envelope carries. Called with
// or without the lock; it reads only src and dst, which are set once.
func (n *lossyNet) numbers(e *messaging.Envelope) []uint64 {
	if e == nil || n.src == nil {
		return nil
	}
	var nums []uint64
	for _, m := range e.Messages {
		var inner messaging.Message
		switch m := m.(type) {
		case *messaging.SyntheticMessage:
			inner = m.Message
		case *messaging.BadSyntheticMessage:
			inner = m.Message
		default:
			continue
		}
		seq, ok := inner.(*messaging.SequencedMessage)
		if ok && seq.Source.Equal(n.src) && seq.Destination.Equal(n.dst) {
			nums = append(nums, seq.Number)
		}
	}
	return nums
}

func (n *lossyNet) anchorToDst(e *messaging.Envelope) bool {
	if e == nil || n.dst == nil {
		return false
	}
	for _, m := range e.Messages {
		a, ok := m.(*messaging.BlockAnchor)
		if !ok {
			continue
		}
		seq, ok := a.Anchor.(*messaging.SequencedMessage)
		if ok && seq.Source.Equal(protocol.DnUrl()) && seq.Destination.Equal(n.dst) {
			return true
		}
	}
	return false
}

type verdict int

const (
	forward verdict = iota
	hold
	forwardLoseAnswer
)

func (n *lossyNet) decide(req *message.Addressed) verdict {
	n.mu.Lock()
	defer n.mu.Unlock()
	env := envelopeOf(req)
	if env == nil {
		return forward
	}
	if n.holdAnchors && n.anchorToDst(env) {
		n.held = append(n.held, req)
		return hold
	}
	nums := n.numbers(env)
	for _, m := range env.Messages {
		if syn, ok := m.(*messaging.SyntheticMessage); ok {
			if seq, ok := syn.Message.(*messaging.SequencedMessage); ok && seq.Source.Equal(n.src) && seq.Destination.Equal(n.dst) {
				if n.seqs == nil {
					n.seqs = map[uint64]*messaging.SequencedMessage{}
				}
				n.seqs[seq.Number] = seq
			}
		}
	}
	if len(nums) > 0 {
		if n.alone == nil {
			n.alone = map[uint64]int{}
		}
		n.alone[nums[0]]++
	}
	for _, num := range nums {
		n.writes[num]++
		if n.armed && num > n.next {
			n.armed, n.next = false, num
			n.onNext(num)
		}
	}
	for _, num := range nums {
		if n.holdNumbers[num] {
			n.held = append(n.held, req)
			return hold
		}
		if num == n.loseFirstAnswer && !n.lostAnswer {
			n.lostAnswer = true
			return forwardLoseAnswer
		}
		if num == n.holdRepeats && n.lostAnswer {
			n.held = append(n.held, req)
			return hold
		}
	}
	return forward
}

// release delivers, in the order they were held, the held requests whose
// envelopes match, and returns the destination's answers.
func (n *lossyNet) release(match func(*messaging.Envelope) bool) []message.Message {
	n.mu.Lock()
	var out []*message.Addressed
	kept := n.held[:0]
	for _, r := range n.held {
		if match(envelopeOf(r)) {
			out = append(out, r)
		} else {
			kept = append(kept, r)
		}
	}
	n.held = kept
	dialer := n.dialer
	n.mu.Unlock()

	var answers []message.Message
	for _, r := range out {
		s, err := dialer.Dial(context.Background(), r.Address)
		if err != nil {
			panic(err)
		}
		if err := s.Write(r); err != nil {
			panic(err)
		}
		res, err := s.Read()
		if err != nil {
			panic(err)
		}
		answers = append(answers, res)
		_ = closeStream(s)
	}
	return answers
}

func (n *lossyNet) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	n.mu.Lock()
	dialer := n.dialer
	n.mu.Unlock()
	return &lossyStream{net: n, ctx: ctx, addr: addr, dialer: dialer}, nil
}

type lossyStream struct {
	net     *lossyNet
	ctx     context.Context
	addr    multiaddr.Multiaddr
	dialer  message.Dialer
	under   message.Stream
	pending message.Message
	broken  bool
}

func (s *lossyStream) Close() error { return closeStream(s.under) }

func closeStream(s message.Stream) error {
	if c, ok := s.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (s *lossyStream) forward(m message.Message) error {
	if s.under == nil {
		u, err := s.dialer.Dial(s.ctx, s.addr)
		if err != nil {
			return err
		}
		s.under = u
	}
	return s.under.Write(m)
}

func (s *lossyStream) Write(m message.Message) error {
	if s.broken {
		return io.EOF
	}
	req, ok := m.(*message.Addressed)
	if !ok {
		return s.forward(m)
	}
	switch s.net.decide(req) {
	case hold:
		s.pending = new(message.SubmitResponse)
		return nil
	case forwardLoseAnswer:
		if err := s.forward(m); err != nil {
			return err
		}
		// The destination takes it and answers; the answer never arrives.
		if _, err := s.under.Read(); err != nil {
			return err
		}
		s.broken = true
		return nil
	default:
		return s.forward(m)
	}
}

func (s *lossyStream) Read() (message.Message, error) {
	if s.broken {
		return nil, io.EOF
	}
	if s.pending != nil {
		m := s.pending
		s.pending = nil
		return m, nil
	}
	if s.under == nil {
		return nil, io.EOF
	}
	return s.under.Read()
}
