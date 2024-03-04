// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

type Node struct {
	mu     sync.Locker
	app    App
	record Recorder
	gossip *Gossip

	context        context.Context
	network        string
	state          nodeState
	self           *validator
	validators     []*validator
	mempool        *mempool
	lastBlockIndex uint64
	lastBlockTime  time.Time

	executeHook ExecuteHookFunc

	// SkipProposalCheck skips checking the proposed block.
	SkipProposalCheck bool

	// IgnoreDeliverResults ignores inconsistencies in the result of DeliverTx
	// (the results of transactions and signatures).
	IgnoreDeliverResults bool

	// IgnoreCommitResults ignores inconsistencies in the result of Commit (the
	// root hash of the BPT).
	IgnoreCommitResults bool
}

type validator struct {
	PubKey     []byte
	PubKeyHash [32]byte
	Power      int64
}

func (v *validator) cmpKey(k []byte) int { return bytes.Compare(v.PubKey, k) }

type ExecuteHookFunc = func(*Node, execute.BlockParams, []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool)

type Recorder interface {
	DidInit(snapshot ioutil.SectionReader) error
	DidExecuteBlock(state execute.BlockState, submissions []*messaging.Envelope) error
}

type StatusRequest struct{}

type StatusResponse struct {
	BlockIndex uint64
	BlockTime  time.Time
}

type ExecuteRequest struct {
	Context context.Context
}

type ExecuteResponse struct {
}

func NewNode(ctx context.Context, network string, key ed25519.PrivateKey, app App, gossip *Gossip) *Node {
	n := new(Node)
	n.mu = new(sync.Mutex)
	n.self = &validator{
		PubKey:     key[32:],
		PubKeyHash: sha256.Sum256(key[32:]),
	}
	n.app = app
	n.network = network
	n.gossip = gossip
	gossip.adopt(n)
	n.context = logging.With(ctx, "module", "consensus")
	n.mempool = newMempool((*logging.Slogger)(slog.Default().With("module", "consensus")))
	return n
}

func (n *Node) SetRecorder(rec Recorder) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.record = rec
}

func (n *Node) SetExecuteHook(hook ExecuteHookFunc) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.executeHook = hook
}

func (n *Node) Info(req *InfoRequest) (*InfoResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.app.Info(req)
}

func (n *Node) Status(*StatusRequest) (*StatusResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	return &StatusResponse{
		BlockIndex: n.lastBlockIndex,
		BlockTime:  n.lastBlockTime,
	}, nil
}

func (n *Node) Check(req *CheckRequest) (*CheckResponse, error) {
	messages, err := req.Envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	for _, m := range n.gossip.nodes {
		m.mu.Lock()
		defer m.mu.Unlock()
	}

	res, err := n.app.Check(req)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if req.Pretend {
		return res, nil
	}

	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	errg, _ := errgroup.WithContext(ctx)
	for _, m := range n.gossip.nodes {
		if m == n {
			continue
		}

		m := m
		errg.Go(func() error {
			return m.recheck(req, res)
		})
	}

	err = errg.Wait()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	lup := map[[32]byte]messaging.Message{}
	for _, m := range messages {
		lup[m.Hash()] = m
	}

	logMessage := func(ok bool, id *url.TxID, msg messaging.Message, err error) {
		kv := []any{"id", id, "last-block", n.lastBlockIndex}
		var debug bool
		if msg != nil {

			kv = append(kv, "type", msg.Type())
		again:
			switch m := msg.(type) {
			case *messaging.TransactionMessage:
				kv = append(kv, "txn-type", m.Transaction.Body.Type())
				if m.Transaction.Body.Type().IsAnchor() {
					debug = true
				}
			case *messaging.SignatureMessage:
				kv = append(kv, "sig-type", m.Signature.Type())
				debug = true
			case *messaging.BlockAnchor:
				debug = true
			case *messaging.BadSyntheticMessage:
				msg = m.Message
				goto again
			case *messaging.SyntheticMessage:
				msg = m.Message
				goto again
			case *messaging.SequencedMessage:
				msg = m.Message
				goto again
			}
		}
		if err != nil {
			kv = append(kv, "error", err)
		}
		var s string
		if ok {
			s = "Message accepted"
		} else {
			s = "Message rejected"
		}
		if debug {
			slog.DebugContext(n.context, s, kv...)
		} else {
			slog.InfoContext(n.context, s, kv...)
		}
	}

	ok := true
	for _, st := range res.Results {
		if !st.Failed() {
			continue
		}

		ok = false
		logMessage(false, st.TxID, lup[st.TxID.Hash()], st.AsError())
		delete(lup, st.TxID.Hash())
	}

	for _, msg := range messages {
		if _, x := lup[msg.Hash()]; x {
			logMessage(ok, msg.ID(), msg, nil)
		}
	}

	if ok {
		for _, m := range n.gossip.nodes {
			m.mempool.Add(m.lastBlockIndex, req.Envelope)
		}
	}

	return res, nil
}

func (n *Node) recheck(req *CheckRequest, expect *CheckResponse) error {
	res, err := n.app.Check(req)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if len(res.Results) != len(expect.Results) {
		slog.ErrorContext(n.context, "Consensus failure", "step", "check", "expected", expect.Results, "actual", res.Results)
		return errors.FatalError.WithFormat("consensus failure: different number of results")
	}
	for i, st := range res.Results {
		if !expect.Results[i].Equal(st) {
			slog.ErrorContext(n.context, "Consensus failure", "step", "check", "id", st.TxID, "expected", expect.Results[i], "actual", st)
			return errors.FatalError.WithFormat("consensus failure: check message %v", st.TxID)
		}
	}
	return nil
}

func (n *Node) Init(req *InitRequest) (*InitResponse, error) {
	// Prevent races with methods such as SetRecorder
	n.mu.Lock()
	defer n.mu.Unlock()

	// Add the initial validators
	n.applyValUp(req.Validators)

	// Initialize the app
	res, err := n.app.Init(req)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Process validator updates
	n.applyValUp(res.Validators)

	// Initialize the block index and time
	info, err := n.app.Info(&InfoRequest{})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	n.lastBlockIndex = info.LastBlock.Index
	n.lastBlockTime = info.LastBlock.Time

	// Record the snapshot
	if n.record != nil {
		err = n.record.DidInit(req.Snapshot)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("record snapshot: %w", err)
		}
	}

	return res, nil
}

func copyEnv(env []*messaging.Envelope) []*messaging.Envelope {
	// Use a copy to avoid weirdness
	c := make([]*messaging.Envelope, len(env))
	for i, env := range env {
		c[i] = env.Copy()
	}
	return c
}

func (n *Node) begin(params execute.BlockParams) (execute.Block, error) {
	res, err := n.app.Begin(&BeginRequest{Params: params})
	if err != nil {
		return nil, errors.FatalError.Wrap(err)
	}
	return res.Block, nil
}

func (n *Node) deliver(block execute.Block, envelopes []*messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	var results []*protocol.TransactionStatus
	for _, envelope := range envelopes {
		s, err := block.Process(envelope)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("deliver envelope: %w", err)
		}

		results = append(results, s...)
	}
	return results, nil
}

func (n *Node) endBlock(block execute.Block) (execute.BlockState, error) {
	state, err := block.Close()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("end block: %w", err)
	}

	// Apply validator updates
	valUp, ok := state.DidUpdateValidators()
	if ok {
		n.applyValUp(valUp)
	}

	return state, nil
}

func (n *Node) commit(state execute.BlockState) ([]byte, error) {
	// Commit (let the app handle empty blocks and notifications)
	err := state.Commit()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("commit: %w", err)
	}

	// Get the old root
	res, err := n.app.Info(&InfoRequest{})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return res.LastHash[:], errors.UnknownError.Wrap(err)
}

func (n *Node) applyValUp(valUp []*execute.ValidatorUpdate) {
	for _, val := range valUp {
		if val.Power > 0 {
			ptr, _ := sortutil.BinaryInsert(&n.validators, func(v *validator) int { return v.cmpKey(val.PublicKey) })
			*ptr = &validator{
				PubKey:     val.PublicKey,
				PubKeyHash: sha256.Sum256(val.PublicKey),
				Power:      val.Power,
			}
		} else {
			sortutil.Remove(&n.validators, func(v *validator) int { return v.cmpKey(val.PublicKey) })
		}
	}
}
