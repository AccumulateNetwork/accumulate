// Copyright 2025 The Accumulate Authors
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
	"log/slog"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Node struct {
	mu     sync.Locker
	app    App
	record Recorder

	context        context.Context
	network        string
	blockState     blockState
	submitState    map[*messaging.Envelope]submitState
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

type StatusRequest struct{}

type StatusResponse struct {
	BlockIndex uint64
	BlockTime  time.Time
}

func NewNode(ctx context.Context, network string, key ed25519.PrivateKey, app App) *Node {
	n := new(Node)
	n.mu = new(sync.Mutex)
	n.self = &validator{
		PubKey:     key[32:],
		PubKeyHash: sha256.Sum256(key[32:]),
	}
	n.submitState = map[*messaging.Envelope]submitState{}
	n.app = app
	n.network = network
	n.context = logging.With(ctx, "module", "consensus")
	n.mempool = newMempool((*logging.Slogger)(slog.Default().With("module", "consensus")))
	return n
}

func (n *Node) SetRecorder(rec Recorder) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.record = rec
	if app, ok := n.app.(interface{ SetRecorder(Recorder) }); ok {
		app.SetRecorder(rec)
	}
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

func (n *Node) check(ctx context.Context, env *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	messages, err := env.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	res, err := n.app.Check(&CheckRequest{
		Context:  ctx,
		Envelope: env,
		New:      true,
	})
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
		again:
			switch m := msg.(type) {
			case *messaging.TransactionMessage:
				kv = append(kv, "txn-type", m.Transaction.Body.Type())
				// if m.Transaction.Body.Type().IsAnchor() {
				debug = true
				// }
			case *messaging.SignatureMessage:
				kv = append(kv, "sig-type", m.Signature.Type())
				debug = true
			case *messaging.BlockAnchor,
				*messaging.NetworkUpdate,
				*messaging.MakeMajorBlock,
				*messaging.DidUpdateExecutorVersion:
				debug = true
			case *messaging.BadSyntheticMessage:
				msg = m.Message
				goto again
			case *messaging.SyntheticMessage:
				msg = m.Message
				kv = append(kv, "synthetic", true)
				goto again
			case *messaging.SequencedMessage:
				msg = m.Message
				kv = append(kv,
					"from", m.Source,
					"to", m.Destination,
					"sequence", m.Number,
				)
				goto again
			}
			kv = append(kv, "type", msg.Type())
		}
		if err != nil {
			kv = append(kv, "error", err)
		}
		var s string
		if ok {
			s = "Message accepted"
		} else {
			s, debug = "Message rejected", false
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

	return res.Results, nil
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

func (n *Node) execute(params execute.BlockParams, envelopes []*messaging.Envelope) (*ExecuteResponse, error) {
	return n.app.Execute(&ExecuteRequest{params, envelopes})
}

func (n *Node) commit(block any) ([]byte, error) {
	// Commit (let the app handle empty blocks and notifications)
	res, err := n.app.Commit(&CommitRequest{Block: block})
	return res.Hash[:], errors.UnknownError.Wrap(err)
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
