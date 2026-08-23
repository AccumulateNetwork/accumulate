// Copyright 2026 The Accumulate Authors
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

// findProofInBundle returns the collection proof carried alongside this message
// in the same envelope, if one covers the message's hash — the package form of
// #4090, where one SyntheticProof proves every synthetic message it travels
// with.
func findProofInBundle(ctx *MessageContext, h [32]byte) *protocol.AnnotatedReceipt {
	for _, msg := range ctx.messages {
		p, ok := msg.(*messaging.SyntheticProof)
		if !ok || p.Proof == nil || p.Proof.ReceiptList == nil {
			continue
		}
		// Bound before hashing: Included walks the list, and an unbounded list
		// from an untrusted envelope is an invitation to burn CPU. The executor
		// for the proof rejects these too, but a sibling must not depend on
		// having been reached first.
		if len(p.Proof.ReceiptList.Elements) > protocol.MaxReceiptListElements {
			continue
		}
		if p.Proof.ReceiptList.Included(h[:]) {
			return p.Proof
		}
	}
	return nil
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

	// A synthetic message must be sequenced (may change in the future)
	seq, ok := syn.Message.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.BadRequest.With("a synthetic message must be sequenced")
	}

	// A message the destination's replica already contains needs no proof and
	// no signature of its own (#4140): the replica was seeded from a proof
	// this partition already accepted and anchored, and hashes cannot be
	// forged. Skip straight to the type check. Tried BEFORE bundle
	// resolution (#4152): a replica-covered, signature-less message must be
	// accepted no matter what else its envelope carries — resolving a
	// sibling proof first sent it to the missing-signature refusal below.
	if syn.Proof == nil && syn.Signature == nil {
		h := syn.Message.Hash()
		if ctx.Executor.replicaIncludes(batch, seq.Source, h[:]) {
			err := checkSyntheticInnerType(seq)
			if err != nil {
				return nil, err
			}
			return syn, nil
		}
	}

	// A synthetic message may omit its own proof when a SyntheticProof travels
	// with it in the same envelope (#4090). One proof then covers every message
	// in the package, instead of each message carrying its own receipt plus a
	// duplicate of the shared continuation.
	//
	// Resolution is confined to THIS bundle — one envelope, delivered atomically.
	// A proof from another envelope is not consulted even if it would verify,
	// because that would make a message's acceptance depend on what else arrived
	// and in what order, which is the coupling collection proofs exist to remove.
	//
	// syn is a copy (SyntheticMessage.Data builds a fresh SynthFields), so
	// filling it in here does not mutate the message or its hash.
	if syn.Proof == nil && ctx.GetActiveGlobals().ExecutorVersion.V2KourouEnabled() {
		syn.Proof = findProofInBundle(ctx, syn.Message.Hash())
	}

	if syn.Proof == nil && syn.Signature == nil {
		return nil, errors.BadRequest.With("missing proof")
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

	// A proof carries either an individual receipt or a collection proof
	// (#4048). Collection proofs are gated so that acceptance activates
	// atomically across the network — until then only individual receipts are
	// valid.
	switch {
	case syn.Proof.ReceiptList != nil:
		if !ctx.GetActiveGlobals().ExecutorVersion.V2KourouEnabled() {
			return nil, errors.BadRequest.With("collection proofs are not enabled")
		}
		if syn.Proof.Receipt != nil {
			return nil, errors.BadRequest.With("proof must carry a receipt or a receipt list, not both")
		}
		if len(syn.Proof.ReceiptList.Elements) > protocol.MaxReceiptListElements {
			return nil, errors.BadRequest.WithFormat("collection proof exceeds %d elements", protocol.MaxReceiptListElements)
		}
		// Validated once per envelope (#4152) — a package's members share
		// ONE proof, and rehashing it per member is a CheckTx DoS.
		if !ctx.bundle.listIsValid(syn.Proof.ReceiptList) {
			return nil, errors.BadRequest.With("proof is invalid")
		}
	case syn.Proof.Receipt != nil:
		if !syn.Proof.Receipt.Validate(nil) {
			return nil, errors.BadRequest.With("proof is invalid")
		}
	default:
		return nil, errors.BadRequest.With("missing proof receipt")
	}

	// Verify the signature — but only when the proof is an individual receipt.
	// A collection proof is the authorization by itself: it proves this exact
	// message hash under an anchor the destination checks against its own
	// directory root at delivery. Once we have a collection proof no other
	// signature is required — the hashes cannot be forged, so it does not
	// matter who served the message. The signature still rides along for
	// identity and recording, but it must never be a reason to reject a
	// proven message: requiring the signer to be a CURRENTLY active validator
	// wedged recovery of historical ranges after validator churn, exactly the
	// failure the anchor path documents (#4056).
	h := syn.Message.Hash()
	if syn.Proof.ReceiptList == nil {
		if !syn.Signature.Verify(nil, syn.Message) {
			return nil, errors.BadRequest.With("invalid signature")
		}

		// Verify the signer is a validator of this partition
		partition, ok := protocol.ParsePartitionUrl(seq.Source)
		if !ok {
			return nil, errors.BadRequest.WithFormat("signature source is not a partition")
		}

		// TODO: Consider checking the version. However this can get messy
		// because it takes some time for changes to propagate, so we'd need an
		// activation height or something.

		signer := core.AnchorSigner(&ctx.Executor.globals.Active, partition)
		_, _, ok = signer.EntryByKeyHash(syn.Signature.GetPublicKeyHash())
		if !ok {
			return nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
		}
	}

	// Verify the proof covers the transaction hash: an individual receipt must
	// start with it, a collection proof must include it as an element
	if syn.Proof.ReceiptList != nil {
		if !syn.Proof.ReceiptList.Included(h[:]) {
			return nil, errors.BadRequest.WithFormat("collection proof does not include %x", h)
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
	err := checkSyntheticInnerType(seq)
	if err != nil {
		return nil, err
	}

	return syn, nil
}

// checkSyntheticInnerType verifies the message within the sequenced message
// is a type a synthetic message may carry.
func checkSyntheticInnerType(seq *messaging.SequencedMessage) error {
	switch seq.Message.Type() {
	case messaging.MessageTypeTransaction,
		messaging.MessageTypeSignature,
		messaging.MessageTypeSignatureRequest,
		messaging.MessageTypeCreditPayment,
		messaging.MessageTypeNetworkUpdate,
		messaging.MessageTypeMakeMajorBlock,
		messaging.MessageTypeDidUpdateExecutorVersion:
		return nil

	default:
		return errors.BadRequest.WithFormat("a synthetic message cannot carry a %v message", seq.Message.Type())
	}
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

	// Process the message (error is handled by the next step)
	err = x.process(batch, ctx)

	// A pending result is not a failure — record the message with a pending
	// status, leaving it retryable for when the proof's anchor arrives
	if errors.Code(err) == errors.Pending {
		err = ctx.recordMessageAndStatus(batch, status, errors.Pending, nil)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		return status, nil
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x SyntheticMessage) process(batch *database.Batch, ctx *MessageContext) error {
	// Validate
	syn, err := x.check(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// A replica-accepted message (#4140) carries no proof of its own — its
	// proof was checked, anchored, and absorbed into the replica when it
	// first arrived.
	if syn.Proof == nil {
		_, err = ctx.callMessageExecutor(batch, syn.Message)
		return errors.UnknownError.Wrap(err)
	}

	// Verify the proof ends with a DN anchor. Individual and collection proofs
	// terminate at the same trust root, so one check covers either form.
	anchor := syn.Proof.TerminalAnchor()
	_, err = batch.Account(ctx.Executor.Describe.AnchorPool()).
		AnchorChain(protocol.Directory).
		Root().
		IndexOf(anchor)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		// If the anchor simply hasn't arrived yet, failing the message
		// terminally wedges recovery: the same message can never be
		// re-applied once the anchor shows up. Once collection proofs are
		// active, record it as pending instead so it can be retried (#4048).
		if ctx.GetActiveGlobals().ExecutorVersion.V2KourouEnabled() {
			return errors.Pending.WithFormat("proof anchor %x has not been received", anchor)
		}
		return errors.BadRequest.WithFormat("invalid proof anchor: %x is not a known directory anchor", anchor)
	default:
		return errors.UnknownError.WithFormat("search for directory anchor %x: %w", anchor, err)
	}

	// Absorb an accepted collection proof into the stream's replica (#4140):
	// the proof is valid (checked above) and anchored (just verified), so the
	// replica now answers for every element it covers — a later message under
	// this proof needs no proof of its own.
	if syn.Proof.ReceiptList != nil {
		if seq, ok := syn.Message.(*messaging.SequencedMessage); ok {
			err = ctx.Executor.seedSyntheticReplica(batch, seq.Source, syn.Proof.ReceiptList)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
	}

	// Execute the inner message
	_, err = ctx.callMessageExecutor(batch, syn.Message)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Record the signature (must not fail)
	if syn.Signature == nil {
		return nil // Replica-accepted messages carry no signature (#4140)
	}
	err = batch.Account(syn.Signature.GetSigner()).
		Transaction(syn.Message.Hash()).
		ValidatorSignatures().
		Add(syn.Signature)
	return errors.InternalError.Wrap(err)
}
