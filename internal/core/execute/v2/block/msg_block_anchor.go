// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[BlockAnchor](&messageExecutors, messaging.MessageTypeBlockAnchor)
}

// BlockAnchor executes the signature, queuing the transaction for processing
// when appropriate.
type BlockAnchor struct{}

// blockAnchorContext collects all the bits of data needed to process a block anchor.
type blockAnchorContext struct {
	*TransactionContext

	sequenced   *messaging.SequencedMessage
	blockAnchor *messaging.BlockAnchor
	signer      protocol.Signer2
}

func (x BlockAnchor) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	ctx2, err := x.check(ctx, batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Validate the transaction
	_, err = ctx.callMessageValidator(batch, ctx2.blockAnchor.Anchor)
	return nil, errors.UnknownError.Wrap(err)
}

func (x BlockAnchor) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message
	ctx2, err := x.check(ctx, batch)
	if err == nil {
		err = x.process(batch, ctx2)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x BlockAnchor) process(batch *database.Batch, ctx *blockAnchorContext) error {
	// Record the anchor signature
	err := batch.Account(ctx.transaction.Header.Principal).
		Transaction(ctx.transaction.ID().Hash()).
		ValidatorSignatures().
		Add(ctx.blockAnchor.Signature)
	if err != nil {
		// A system error occurred
		return errors.UnknownError.Wrap(err)
	}

	// Add the signature to the signature chain
	err = batch.Account(ctx.transaction.Header.Principal).
		Transaction(ctx.transaction.ID().Hash()).
		RecordHistory(ctx.message)
	if err != nil {
		return errors.UnknownError.WithFormat("record history: %w", err)
	}

	ready, err := x.txnIsReady(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !ready {
		// Mark the message as pending
		_, err = ctx.childWith(ctx.sequenced.Message).recordPending(batch)
		return errors.UnknownError.Wrap(err)
	}

	// Process the transaction
	_, err = ctx.callMessageExecutor(batch, ctx.sequenced)
	return errors.UnknownError.Wrap(err)
}

// check checks if the message is garbage or not.
func (x BlockAnchor) check(ctx *MessageContext, batch *database.Batch) (*blockAnchorContext, error) {
	anchor, ok := ctx.message.(*messaging.BlockAnchor)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBlockAnchor, ctx.message.Type())
	}

	// A block anchor is authorized either by a validator signature counted
	// toward the quorum, or by a collection proof under a root of the source
	// that this node already holds (#4087). The proof form matters for recovery:
	// a historical quorum can be impossible to re-gather after validator churn,
	// while the proof depends only on an anchor already received and validated.
	if anchor.Proof != nil {
		if !ctx.GetActiveGlobals().ExecutorVersion.V2KourouEnabled() {
			return nil, errors.BadRequest.With("anchor collection proofs are not enabled")
		}
		if anchor.Proof.ReceiptList == nil {
			return nil, errors.BadRequest.With("anchor proof must carry a receipt list")
		}
		if anchor.Proof.Receipt != nil {
			return nil, errors.BadRequest.With("anchor proof must carry a receipt or a receipt list, not both")
		}
		if len(anchor.Proof.ReceiptList.Elements) > protocol.MaxReceiptListElements {
			return nil, errors.BadRequest.WithFormat("collection proof carries %d elements, limit is %d",
				len(anchor.Proof.ReceiptList.Elements), protocol.MaxReceiptListElements)
		}
		if !anchor.Proof.ReceiptList.Validate(nil) {
			return nil, errors.BadRequest.With("anchor proof is invalid")
		}
	}

	if anchor.Signature == nil && anchor.Proof == nil {
		return nil, errors.BadRequest.With("missing signature")
	}
	if anchor.Anchor == nil {
		return nil, errors.BadRequest.With("missing anchor")
	}
	if anchor.Signature != nil && anchor.Signature.GetTransactionHash() == ([32]byte{}) {
		return nil, errors.BadRequest.With("missing transaction hash")
	}

	// Verify the anchor is a sequenced anchor transaction
	seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeSequenced, anchor.Anchor.Type())
	}
	txnMsg, ok := seq.Message.(*messaging.TransactionMessage)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeTransaction, seq.Message.Type())
	}

	// Resolve placeholders
	txn := txnMsg.Transaction
	if txn.Body.Type() == protocol.TransactionTypeRemote && ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		var err error
		txn, err = ctx.getTransaction(batch, txn.ID().Hash())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}
	}

	// Verify the transaction is an anchor
	if !txn.Body.Type().IsAnchor() {
		return nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", txn.Body.Type(), anchor.Type())
	}

	// Verify the destination and principal match
	if ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() {
		if seq.Destination == nil {
			return nil, errors.InternalError.WithFormat("sequence is missing destination")
		}
		if txn.Header.Principal == nil {
			return nil, errors.InternalError.WithFormat("transaction is missing principal")
		}
		if !seq.Destination.RootIdentity().Equal(txn.Header.Principal.RootIdentity()) {
			return nil, errors.BadRequest.WithFormat("sequence destination does not match transaction principal")
		}
	}

	// Verify the signer is a validator of this partition
	if seq.Source == nil {
		return nil, errors.InternalError.WithFormat("sequence is missing source")
	}

	// Verify the signer is a validator of this partition
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return nil, errors.BadRequest.WithFormat("signature source is not a partition")
	}

	// TODO: Consider checking the version. However this can get messy because
	// it takes some time for changes to propagate, so we'd need an activation
	// height or something.

	// The signer is still needed to build the context below, but the validator
	// check only applies when a signature is present — a proof-authorized anchor
	// carries none.
	signer := core.AnchorSigner(&ctx.Executor.globals.Active, partition)
	if anchor.Signature != nil {
		_, _, ok = signer.EntryByKeyHash(anchor.Signature.GetPublicKeyHash())
		if !ok {
			return nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
		}
	}

	// A collection proof must include the anchor transaction in its canonical
	// stored form: the anchor sequence chain records it without a principal, and
	// post-Vandenberg the body is identical for every destination.
	if anchor.Proof != nil {
		stored := new(protocol.Transaction)
		stored.Body = txn.Body
		if !anchor.Proof.ReceiptList.Included(stored.GetHash()) {
			return nil, errors.BadRequest.WithFormat("collection proof does not include anchor %x", stored.GetHash())
		}
	}

	// Basic validation
	ctx2 := &blockAnchorContext{
		TransactionContext: ctx.txnWith(txn),
		sequenced:          seq,
		blockAnchor:        anchor,
		signer:             signer,
	}
	err := x.checkSignature(ctx2)
	if err != nil {
		return nil, err
	}

	return ctx2, nil
}

func (x BlockAnchor) checkSignature(ctx *blockAnchorContext) error {
	// A proof-authorized anchor carries no signature to check. Its
	// authorization was established by the collection proof, which was
	// validated and shown to include this anchor before we got here.
	if ctx.blockAnchor.Signature == nil {
		return nil
	}

	// Recalculate the hash in case the transaction was originally a remote
	// transaction
	txn := &messaging.TransactionMessage{Transaction: ctx.transaction}
	seq := *ctx.sequenced
	seq.Message = txn
	if ctx.blockAnchor.Signature.Verify(nil, &seq) {
		return nil
	}

	// Allow reusing signatures from the DN
	part, _ := protocol.ParsePartitionUrl(ctx.transaction.Header.Principal)
	if ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() &&
		ctx.transaction.Body.Type() == protocol.TransactionTypeDirectoryAnchor &&
		!strings.EqualFold(part, protocol.Directory) {

		seq.Destination = protocol.DnUrl()
		txn.Transaction = txn.Transaction.Copy()
		txn.Transaction.Header.Principal = protocol.DnUrl().JoinPath(ctx.transaction.Header.Principal.Path)
		if ctx.blockAnchor.Signature.Verify(nil, &seq) {
			return nil
		}
	}

	return errors.Unauthenticated.WithFormat("invalid signature")
}

func (x BlockAnchor) txnIsReady(batch *database.Batch, ctx *blockAnchorContext) (bool, error) {
	// A collection proof under a root of the SOURCE that we already hold
	// authorizes the anchor by itself — no signature quorum (#4087). If we do
	// not hold that root we fall through to the quorum check rather than
	// failing, and recovery resubmits once a later anchor extends what we know.
	if ctx.blockAnchor.Proof != nil {
		held, err := x.proofAnchorIsHeld(batch, ctx)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if held {
			return true, nil
		}
	}

	sigs, err := batch.Account(ctx.transaction.Header.Principal).
		Transaction(ctx.transaction.ID().Hash()).
		ValidatorSignatures().
		Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load anchor signatures: %w", err)
	}

	// Have we received enough signatures?
	partition, ok := protocol.ParsePartitionUrl(ctx.sequenced.Source)
	if !ok {
		return false, errors.BadRequest.WithFormat("source %v is not a partition", ctx.sequenced.Source)
	}
	if uint64(len(sigs)) < ctx.Executor.globals.Active.ValidatorThreshold(partition) {
		return false, nil
	}

	return true, nil
}

// proofAnchorIsHeld reports whether the collection proof terminates at a root of
// the source partition that this node already holds and has already validated.
//
// This is what lets recovery work precisely when the network is behind, and it
// rests on a property of merkle chains rather than anything about anchors:
// holding a validated state of a chain proves every entry added before it. An
// anchor from S commits to S's root chain, which commits to S's anchor sequence
// chain — so one anchor we already trust proves any earlier run of anchors by
// replay. Nothing newer is needed, and the directory is not involved.
//
// Involving the directory is what deadlocked #4087: it made recovering an anchor
// depend on that anchor having been anchored. It is the same coupling behind
// #4086, where the source could not build a receipt because its producing block
// was not yet DN-anchored — 1,492 errors in ten minutes. A proof that only ever
// looks BACKWARD from something already held cannot be blocked by lag.
//
// Two forms of "already holds", checked in that order:
//
//   - An anchor from S we have executed, whose root is on our anchor chain for S.
//   - The next anchor from S we hold but have not executed, kept in the ledger's
//     pending list because it arrived out of order. That anchor is in the ledger
//     only because it already met S's validator quorum, so its root is trusted by
//     exactly the standard an executed one is — and it is the bound that made the
//     gap visible, so it is the one the source proves against.
//
// Every validator of this partition runs this same check against the same
// consensus state, so they agree on whether an anchor is authorized.
func (x BlockAnchor) proofAnchorIsHeld(batch *database.Batch, ctx *blockAnchorContext) (bool, error) {
	terminal := ctx.blockAnchor.Proof.TerminalAnchor()
	if len(terminal) != 32 {
		return false, nil
	}
	partition, ok := protocol.ParsePartitionUrl(ctx.sequenced.Source)
	if !ok {
		return false, nil
	}
	anchorPool := batch.Account(ctx.Executor.Describe.AnchorPool())

	// A root from an anchor we have already executed
	_, err := anchorPool.AnchorChain(partition).Root().IndexOf(terminal)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errors.NotFound):
		// Keep looking
	default:
		return false, errors.UnknownError.WithFormat("search for %s anchor %x: %w", partition, terminal, err)
	}

	// A root from the next anchor we hold but have not executed
	var ledger *protocol.AnchorLedger
	err = anchorPool.Main().GetAs(&ledger)
	if err != nil {
		return false, errors.UnknownError.WithFormat("load anchor ledger: %w", err)
	}
	part := ledger.Partition(ctx.sequenced.Source)

	var next *url.TxID
	for i, txid := range part.Pending {
		if txid == nil || part.Delivered+uint64(i)+1 <= ctx.sequenced.Number {
			continue
		}
		next = txid
		break
	}
	if next == nil {
		return false, nil
	}

	var seq *messaging.SequencedMessage
	err = batch.Message(next.Hash()).Main().GetAs(&seq)
	if err != nil {
		return false, errors.UnknownError.WithFormat("load pending anchor %v: %w", next, err)
	}
	txnMsg, ok := seq.Message.(messaging.MessageWithTransaction)
	if !ok {
		return false, nil
	}
	txn := txnMsg.GetTransaction()
	if txn.Body.Type() == protocol.TransactionTypeRemote {
		txn, err = ctx.getTransaction(batch, txnMsg.ID().Hash())
		if err != nil {
			return false, errors.UnknownError.WithFormat("load pending anchor transaction: %w", err)
		}
	}
	body, ok := txn.Body.(protocol.AnchorBody)
	if !ok {
		return false, nil
	}

	root := body.GetPartitionAnchor().RootChainAnchor
	return bytes.Equal(root[:], terminal), nil
}
