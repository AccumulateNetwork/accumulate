// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SyntheticMessage](&messageExecutors, messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic)
}

// SyntheticMessage records the synthetic transaction but does not execute
// it.
type SyntheticMessage struct{}

func (x SyntheticMessage) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// Check the wrapper
	syn, err := x.check(batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Validate the inner message
	_, err = ctx.callMessageValidator(batch, syn.Message)
	return nil, errors.UnknownError.Wrap(err)
}

// synthDropped surfaces a synthetic message that was rejected with a client
// error at delivery. Such a rejection is otherwise recorded only as a terminal
// failed status with no other trace — which is what made the #4070 wedge so hard
// to find. A synthetic whose proof anchor is not yet known is now HELD, not
// dropped (see process), so this fires only for genuinely terminal rejections.
func synthDropped(ctx *MessageContext, cause error) {
	var syn *messaging.SynthFields
	switch m := ctx.message.(type) {
	case *messaging.SyntheticMessage:
		syn = m.Data()
	case *messaging.BadSyntheticMessage:
		syn = m.Data()
	}
	if syn == nil {
		return
	}
	seq, ok := syn.Message.(*messaging.SequencedMessage)
	if !ok {
		return
	}
	ctx.Executor.logger.Info("Synthetic message rejected",
		"module", "synthetic", "source", seq.Source,
		"destination", seq.Destination, "number", seq.Number, "error", cause)
}

// holdsAnchorRoot reports whether the given root is on our anchor chain for the
// named partition — that is, whether it arrived on an anchor we have already
// received, validated and executed.
//
// This is the trust question every proof reduces to. What differs between a
// directory root and a source's own root is only which chain to look on, not how
// much is being trusted: both got there by an anchor meeting its partition's
// validator quorum.
func sourcePartition(seq *messaging.SequencedMessage) (string, bool) {
	if seq == nil || seq.Source == nil {
		return "", false
	}
	return protocol.ParsePartitionUrl(seq.Source)
}

func holdsAnchorRoot(batch *database.Batch, anchorPool *url.URL, partition string, root []byte) (bool, error) {
	_, err := batch.Account(anchorPool).AnchorChain(partition).Root().IndexOf(root)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errors.NotFound):
		return false, nil
	default:
		return false, errors.UnknownError.WithFormat("search for %s anchor %x: %w", partition, root, err)
	}
}

func (SyntheticMessage) check(batch *database.Batch, ctx *MessageContext) (*messaging.SynthFields, error) {
	// Using messaging.SynthFields is safer than converting one message type
	// into the other because that could lead to issues with the different Hash
	// method implementations
	var syn *messaging.SynthFields
	if !ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		msg, ok := ctx.message.(*messaging.BadSyntheticMessage)
		if !ok {
			return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBadSynthetic, ctx.message.Type())
		}
		syn = msg.Data()
	} else {
		switch msg := ctx.message.(type) {
		case *messaging.BadSyntheticMessage:
			syn = msg.Data()
		case *messaging.SyntheticMessage:
			syn = msg.Data()
		default:
			return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSynthetic, ctx.message.Type())
		}
	}

	// Basic validation
	if syn.Message == nil {
		return nil, errors.BadRequest.With("missing message")
	}
	if syn.Signature == nil {
		return nil, errors.BadRequest.With("missing signature")
	}
	if syn.Proof == nil {
		return nil, errors.BadRequest.With("missing proof")
	}
	if syn.Proof.Anchor == nil || syn.Proof.Anchor.Account == nil {
		return nil, errors.BadRequest.With("missing proof metadata")
	}

	// Accept either proof form. An individual receipt proves one message; a
	// collection proof proves a contiguous range with a single proof, which is
	// what makes range recovery cheap (#4087). Both terminate at a DN anchor, so
	// the trust root below is unchanged.
	switch {
	case syn.Proof.ReceiptList != nil:
		// Gated: what a node accepts is consensus-critical. If one node took a
		// collection proof while another rejected it, they would disagree about
		// the state. Nothing emits one before this activates network-wide.
		if !ctx.GetActiveGlobals().ExecutorVersion.V2KourouEnabled() {
			return nil, errors.BadRequest.With("collection proofs are not enabled")
		}
		if syn.Proof.Receipt != nil {
			return nil, errors.BadRequest.With("proof carries both a receipt and a receipt list")
		}
		// Bound the work before doing any: an unbounded list is an invitation to
		// make a validator allocate and hash arbitrarily much.
		if len(syn.Proof.ReceiptList.Elements) > protocol.MaxReceiptListElements {
			return nil, errors.BadRequest.WithFormat("collection proof carries %d elements, limit is %d",
				len(syn.Proof.ReceiptList.Elements), protocol.MaxReceiptListElements)
		}
		if !syn.Proof.ReceiptList.Validate(nil) {
			return nil, errors.BadRequest.With("proof is invalid")
		}

	case syn.Proof.Receipt != nil:
		if !syn.Proof.Receipt.Validate(nil) {
			return nil, errors.BadRequest.With("proof is invalid")
		}

	default:
		return nil, errors.BadRequest.With("missing proof receipt")
	}

	// A synthetic message must be sequenced (may change in the future)
	seq, ok := syn.Message.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.BadRequest.With("a synthetic message must be sequenced")
	}

	// Verify the signature
	h := syn.Message.Hash()
	if !syn.Signature.Verify(nil, syn.Message) {
		return nil, errors.BadRequest.With("invalid signature")
	}

	// Verify the signer is a validator of this partition
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return nil, errors.BadRequest.WithFormat("signature source is not a partition")
	}

	// TODO: Consider checking the version. However this can get messy because
	// it takes some time for changes to propagate, so we'd need an activation
	// height or something.

	signer := core.AnchorSigner(&ctx.Executor.globals.Active, partition)
	_, _, ok = signer.EntryByKeyHash(syn.Signature.GetPublicKeyHash())
	if !ok {
		return nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
	}

	// Verify the proof covers this message. An individual receipt must start at
	// the message hash; a collection proof must contain it. Included also binds
	// the element's absolute index, because the list carries the counted merkle
	// state at its start — so a collection proof pins the sequence number
	// without any additional machinery.
	if syn.Proof.ReceiptList != nil {
		if !syn.Proof.ReceiptList.Included(h[:]) {
			return nil, errors.BadRequest.WithFormat("message %x is not included in the collection proof", h)
		}
	} else if !bytes.Equal(h[:], syn.Proof.Receipt.Start) {
		return nil, errors.BadRequest.WithFormat("invalid proof start: expected %x, got %x", h, syn.Proof.Receipt.Start)
	}

	// Don't check the anchor during validation. If we check the anchor during
	// validation, there is a race condition: partition X may receive a DN
	// anchor and submit synthetic messages to partition Y before that partition
	// receives and processes that anchor, which could cause partition Y to
	// reject the message during CheckTx. Waiting until DeliverTx to check the
	// anchor does not eliminate the race but it does significantly reduce the
	// likelihood it will strike, since partition Y will almost process the DN
	// anchor before it processes the synthetic message.

	// Verify the message within the sequenced message is an allowed type
	switch seq.Message.Type() {
	case messaging.MessageTypeTransaction,
		messaging.MessageTypeSignature,
		messaging.MessageTypeSignatureRequest,
		messaging.MessageTypeCreditPayment,
		messaging.MessageTypeNetworkUpdate,
		messaging.MessageTypeMakeMajorBlock,
		messaging.MessageTypeDidUpdateExecutorVersion:
		// Allowed

	default:
		return nil, errors.BadRequest.WithFormat("a synthetic message cannot carry a %v message", seq.Message.Type())
	}

	return syn, nil
}

func (x SyntheticMessage) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message. `held` means the proof anchor has not yet arrived.
	held, err := x.process(batch, ctx)

	// A synthetic whose proof anchor is not yet known is HELD — recorded pending,
	// not failed — so it re-runs in place when the anchor is delivered (see
	// processDirAnchor). Before this, the anchor race under network disruption
	// recorded a terminal failure that wedged the stream permanently, because the
	// healer's byte-identical re-submission is deduplicated and never re-processed
	// (#4070). Any real failure still records a failed status, and is surfaced —
	// recordMessageAndStatus otherwise swallows a client error into a status with
	// no other trace.
	s := errors.Delivered
	if held {
		s = errors.Pending
	} else if err != nil {
		synthDropped(ctx, err)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, s, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

// process delivers the synthetic message. The returned bool reports that the
// message was HELD awaiting its proof anchor rather than delivered or failed.
func (x SyntheticMessage) process(batch *database.Batch, ctx *MessageContext) (bool, error) {
	// Validate
	syn, err := x.check(batch, ctx)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	// Verify the proof ends at an anchor root we hold. TerminalAnchor resolves to
	// the receipt's anchor for an individual proof and to the continuation's (or
	// the list's own) for a collection proof, so both forms are checked the same
	// way.
	//
	// Two roots qualify. A directory root is what the ordinary outbound path
	// produces, and is unchanged. A root of the SOURCE — committed by an anchor
	// we already received from it and validated — is what recovery uses (#4087),
	// and it is the one that works while the directory is behind: an anchor from
	// S commits to S's root chain, which commits to S's synthetic chains, so one
	// anchor we already hold proves S's earlier synthetic messages by replay.
	// Nothing newer than what we have is needed, and the directory is not asked
	// for anything — which is what dissolves #4086 rather than mitigating it.
	terminal := syn.Proof.TerminalAnchor()
	if len(terminal) != 32 {
		return false, errors.BadRequest.With("proof has no terminal anchor")
	}
	held, err := holdsAnchorRoot(batch, ctx.Executor.Describe.AnchorPool(), protocol.Directory, terminal)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	// Gated: what a node is willing to accept is consensus-critical. If one node
	// took a source-rooted proof while another rejected it, they would disagree
	// about state. Nothing produces one before this activates network-wide.
	if !held && ctx.GetActiveGlobals().ExecutorVersion.V2KourouEnabled() {
		// check has already verified the message is sequenced
		seq, _ := syn.Message.(*messaging.SequencedMessage)
		if partition, ok := sourcePartition(seq); ok {
			held, err = holdsAnchorRoot(batch, ctx.Executor.Describe.AnchorPool(), partition, terminal)
			if err != nil {
				return false, errors.UnknownError.Wrap(err)
			}
		}
	}
	if !held {
		// The proof anchor is not (yet) known. From V2Jiuquan, HOLD the synthetic
		// keyed by the anchor it is waiting for and record it pending, so that when
		// an anchor carrying that root is delivered, releaseSyntheticsHeldFor
		// re-attempts it IN PLACE — no re-submission. This restores the V1 behavior
		// that V2 dropped; before Jiuquan the anchor race was a terminal failure
		// that wedged the receiver's stream permanently (#4070). Both directory and
		// partition anchors release what they carry, so either root can be waited on.
		if ctx.GetActiveGlobals().ExecutorVersion.V2JiuquanEnabled() {
			err = batch.Account(ctx.Executor.Describe.Ledger()).
				SyntheticForAnchor(*(*[32]byte)(terminal)).
				Add(ctx.message.ID())
			if err != nil {
				return false, errors.UnknownError.WithFormat("hold synthetic for anchor: %w", err)
			}
			return true, nil
		}
		return false, errors.BadRequest.WithFormat("invalid proof anchor: %x is not a known anchor", terminal)
	}

	// Execute the inner message
	_, err = ctx.callMessageExecutor(batch, syn.Message)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	// Record the signature (must not fail)
	err = batch.Account(syn.Signature.GetSigner()).
		Transaction(syn.Message.Hash()).
		ValidatorSignatures().
		Add(syn.Signature)
	return false, errors.InternalError.Wrap(err)
}
