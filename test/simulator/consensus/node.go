// Copyright 2023 The Accumulate Authors
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
	stderr "errors"
	"sync"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

type Node struct {
	mu         sync.Locker
	pool       *mempool
	privKey    ed25519.PrivateKey
	pubKeyHash [32]byte
	app        App
	gossip     *Gossip
	blockIndex uint64
	blockTime  time.Time
	record     Recorder
	logger     logging.OptionalLogger
	validators []*validator

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

func NewNode(key ed25519.PrivateKey, app App, gossip *Gossip, logger log.Logger) *Node {
	n := new(Node)
	n.mu = new(sync.Mutex)
	n.privKey = key
	n.pubKeyHash = sha256.Sum256(key[32:])
	n.app = app
	n.gossip = gossip
	n.logger.Set(logger, "module", "consensus")
	gossip.adopt(n)
	n.pool = newMempool(n.logger.L)
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
		BlockIndex: n.blockIndex,
		BlockTime:  n.blockTime,
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
		kv := []any{"id", id, "last-block", n.blockIndex}
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
			n.logger.Debug(s, kv...)
		} else {
			n.logger.Info(s, kv...)
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
			m.pool.Add(m.blockIndex, req.Envelope)
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
		n.logger.Error("Consensus failure", "step", "check", "expected", expect.Results, "actual", res.Results)
		return errors.FatalError.WithFormat("consensus failure: different number of results")
	}
	for i, st := range res.Results {
		if !expect.Results[i].Equal(st) {
			n.logger.Error("Consensus failure", "step", "check", "id", st.TxID, "expected", expect.Results[i], "actual", st)
			return errors.FatalError.WithFormat("consensus failure: check message %v", st.TxID)
		}
	}
	return nil
}

func (n *Node) Init(req *InitRequest) (*InitResponse, error) {
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
	n.blockIndex = info.LastBlock.Index
	n.blockTime = info.LastBlock.Time

	// Record the snapshot
	if n.record != nil {
		err = n.record.DidInit(req.Snapshot)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("record snapshot: %w", err)
		}
	}

	return res, nil
}

var errAborted = stderr.New("aborted")

func (n *Node) Execute(req *ExecuteRequest) (*ExecuteResponse, error) {
	for _, m := range n.gossip.nodes {
		m.mu.Lock()
		defer m.mu.Unlock()
	}

	// Select the leader
	leader := n.selectLeader()
	var haveLeader bool
	for _, m := range n.gossip.nodes {
		if m.pubKeyHash == leader {
			haveLeader = true
		}
		if m.selectLeader() != leader {
			return nil, errors.FatalError.WithFormat("consensus error: select leader (1)")
		}
	}
	if !haveLeader {
		return nil, errors.FatalError.WithFormat("consensus error: select leader (2)")
	}
	n.logger.Debug("Selected a leader", "leader", logging.AsHex(leader).Slice(0, 4), "block", n.blockIndex+1)

	// Execute
	followers := make([]chan any, len(n.gossip.nodes)-1)
	for i := range followers {
		followers[i] = make(chan any, 1)
	}

	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	errg, ctx := errgroup.WithContext(ctx)
	var i int
	for _, m := range n.gossip.nodes {
		m := m
		if m.pubKeyHash == leader {
			// Force a panic if ch is used by the leader
			ch := make(chan any)
			close(ch)

			errg.Go(func() error {
				err := m.execute(ctx, true, followers, ch)
				if err != nil {
					m.logger.Error("Execute failed", "error", err)
				}
				return err
			})
		} else {
			ch := followers[i]
			i++
			errg.Go(func() error {
				err := m.execute(ctx, false, nil, ch)
				if err != nil {
					m.logger.Error("Execute failed", "error", err)
				}
				return err
			})
		}
	}

	err := errg.Wait()
	if err != nil {
		// If a consensus error occurs, err should be that error, because the
		// other nodes will only stop *after* the context is canceled by the
		// consensus error. The only other possibility is that the context was
		// canceled directly, in which case its error should be returned. There
		// should never be a scenario where err is errAborted and the context
		// error is nil.
		if errors.Is(err, errAborted) && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}

	n.logger.Debug("Block complete", "block", n.blockIndex)
	return &ExecuteResponse{}, nil
}

func (n *Node) selectLeader() [32]byte {
	return n.validators[int(n.blockIndex)%len(n.validators)].PubKeyHash
}

func get[T any](ch chan any) (T, bool) {
	v, ok := <-ch
	if !ok {
		var z T
		return z, false
	}
	return v.(T), true
}

func send(ctx context.Context, ch []chan any, v any) bool {
	for _, ch := range ch {
		select {
		case ch <- v:
			// Ok
		case <-ctx.Done():
			return false
		}
	}
	return true
}

func copyEnv(env []*messaging.Envelope) []*messaging.Envelope {
	// Use a copy to avoid weirdness
	c := make([]*messaging.Envelope, len(env))
	for i, env := range env {
		c[i] = env.Copy()
	}
	return c
}

func (n *Node) execute(ctx context.Context, isLeader bool, followers []chan any, ch chan any) error {
	if isLeader {
		defer func() {
			for _, ch := range followers {
				close(ch)
			}
		}()
	}

	var envelopes []*messaging.Envelope
	if isLeader {
		// Send out the proposal
		n.logger.Debug("Proposing a block", "block", n.blockIndex+1)
		envelopes = n.pool.Propose(n.blockIndex + 1)
		envelopes = copyEnv(envelopes)
		if !send(ctx, followers, envelopes) {
			return errAborted
		}

	} else {
		// Receive the proposal
		var ok bool
		envelopes, ok = get[[]*messaging.Envelope](ch)
		if !ok {
			return errAborted
		}
		envelopes = copyEnv(envelopes)

		// FIXME Fix this and reenable it. It is unreliable and causes
		// intermittent failures.
		if false && !n.SkipProposalCheck {
			for _, env := range envelopes {
				err := n.pool.CheckProposed(n.blockIndex+1, env)
				if err != nil {
					n.logger.Error("Consensus failure", "step", "propose", "error", err, "envelope", env, "block", n.blockIndex+1)
					return errors.FatalError.With("consensus error: propose block (1)")
				}
			}
		}
	}

	n.blockIndex++
	n.blockTime = n.blockTime.Add(time.Second)
	n.pool.AcceptProposed(n.blockIndex, envelopes)

	params := execute.BlockParams{
		Context:  ctx,
		Index:    n.blockIndex,
		Time:     n.blockTime,
		IsLeader: isLeader,
	}

	// Apply the block hook
	if n.executeHook != nil {
		var keep bool
		envelopes, keep = n.executeHook(n, params, envelopes)
		if !keep {
			n.executeHook = nil
		}
	}

	// Begin the block
	block, err := n.begin(params)
	if err != nil {
		return errors.FatalError.WithFormat("begin block: %w", err)
	}
	if isLeader {
		n.logger.Debug("Block begin", "block", params.Index, "time", params.Time)
		if !send(ctx, followers, params) {
			return errAborted
		}
	} else {
		p, ok := get[execute.BlockParams](ch)
		if !ok {
			return errAborted
		}
		if params.Index != p.Index {
			n.logger.Error("Consensus failure", "step", "begin", "error", "block parameters don't match", "leader-hash", p, "our-hash", params)
			return errors.FatalError.With("consensus error: begin block")
		}
	}

	// Deliver the envelopes
	results, err := n.deliver(block, envelopes)
	if err != nil {
		return errors.FatalError.WithFormat("deliver: %w", err)
	}
	if isLeader {
		for _, r := range results {
			n.logger.Debug("Delivered", "block", params.Index, "result", r)
		}
		if !send(ctx, followers, results) {
			return errAborted
		}
	} else {
		r, ok := get[[]*protocol.TransactionStatus](ch)
		if !ok {
			return errAborted
		}
		if !n.IgnoreDeliverResults {
			if len(results) != len(r) {
				n.logger.Error("Consensus failure", "step", "deliver", "expected", r, "actual", results)
				return errors.FatalError.WithFormat("consensus failure: different number of results")
			}
			for i, st := range results {
				if !r[i].Equal(st) {
					n.logger.Error("Consensus failure", "step", "deliver", "id", st.TxID, "expected", r[i], "actual", st)
					return errors.FatalError.WithFormat("consensus failure: deliver message %v", st.TxID)
				}
			}
		}
	}

	// End the block
	state, err := n.endBlock(block)
	if err != nil {
		return errors.FatalError.WithFormat("end block: %w", err)
	}

	// Record before checking consensus
	if n.record != nil {
		err = n.record.DidExecuteBlock(state, envelopes)
		if err != nil {
			return errors.UnknownError.WithFormat("record block: %w", err)
		}
	}

	type valUpOk struct {
		V  []*execute.ValidatorUpdate
		Ok bool
	}
	valUp, didUpdate := state.DidUpdateValidators()
	if isLeader {
		n.logger.Debug("End block", "block", params.Index, "validator-updates", valUp)
		if !send(ctx, followers, valUpOk{valUp, didUpdate}) {
			return errAborted
		}
	} else {
		v, ok := get[valUpOk](ch)
		if !ok {
			return errAborted
		}
		if v.Ok != didUpdate {
			n.logger.Error("Consensus failure", "step", "end", "expected", v.Ok, "actual", didUpdate)
			return errors.FatalError.WithFormat("consensus failure: end block (1)")
		}
		if len(v.V) != len(valUp) {
			n.logger.Error("Consensus failure", "step", "end", "expected", v.V, "actual", valUp)
			return errors.FatalError.WithFormat("consensus failure: end block (2)")
		}
		for i, v := range v.V {
			if !bytes.Equal(v.PublicKey, valUp[i].PublicKey) || v.Power != valUp[i].Power {
				n.logger.Error("Consensus failure", "step", "end", "expected", v, "actual", valUp[i])
				return errors.FatalError.WithFormat("consensus failure: end block (3)")
			}
		}
	}
	n.applyValUp(valUp)

	// Commit the block
	hash, err := n.commit(state)
	if err != nil {
		return errors.FatalError.WithFormat("commit: %w", err)
	}
	if isLeader {
		n.logger.Debug("Commit", "block", params.Index, "hash", logging.AsHex(hash))
		if !send(ctx, followers, hash) {
			return errAborted
		}
	} else {
		expect, ok := get[[]byte](ch)
		if !ok {
			return errAborted
		}
		if !n.IgnoreCommitResults {
			err := CommitConsensusError{hash, expect}
			if err.isErr() {
				n.logger.Error("Consensus failure", "step", "commit", "error", "state hashes don't match", "leader", logging.AsHex(expect).Slice(0, 4), "ours", logging.AsHex(hash).Slice(0, 4))
				return err
			}
		}
	}
	return nil
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

type CommitConsensusError [][]byte

func (c CommitConsensusError) isErr() bool {
	for _, v := range c[1:] {
		if !bytes.Equal(c[0], v) {
			return true
		}
	}
	return false
}

func (CommitConsensusError) Error() string { return "consensus failure during commit" }
