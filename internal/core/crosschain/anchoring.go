// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"context"
	"crypto/ed25519"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

func (c *Conductor) healAnchors(ctx context.Context, batch *database.Batch, destination *url.URL, currentBlock uint64) error {
	if !def(c.EnableAnchorHealing, true) {
		return nil
	}

	// Load the source sequence chain
	sequence := batch.Account(c.Url(protocol.AnchorPool)).AnchorSequenceChain()
	head, err := sequence.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor sequence chain head: %w", err)
	}

	// Load the destination anchor ledger
	var ledger1 *protocol.AnchorLedger
	_, err = c.Querier.QueryAccountAs(ctx, destination.JoinPath(protocol.AnchorPool), nil, &ledger1)
	if err != nil {
		return errors.UnknownError.WithFormat("query %v anchor ledger: %w", destination, err)
	}
	ledger2 := ledger1.Partition(c.Url())

	// For each not-yet delivered anchor
	for i := ledger2.Delivered + 1; i <= uint64(head.Count); i++ {
		// Load it
		hash, err := sequence.Entry(int64(i) - 1)
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor sequence entry %d: %w", i-1, err)
		}

		var msg *messaging.TransactionMessage
		err = batch.Message2(hash).Main().GetAs(&msg)
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor %d: %w", i, err)
		}

		anchor, ok := msg.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			return errors.UnknownError.WithFormat("anchor %d is not an anchor: got %v", i, msg.Transaction.Body.Type())
		}

		// Ignore anchors from the last 10 blocks
		if currentBlock-anchor.GetPartitionAnchor().MinorBlockIndex < 10 {
			continue
		}

		// Construct the envelope
		env, txn, err := ValidatorContext{
			Source:       c.Partition,
			Globals:      c.Globals.Load(),
			ValidatorKey: c.ValidatorKey,
		}.PrepareAnchorSubmission(ctx, anchor, i, destination)
		if err != nil {
			return errors.UnknownError.WithFormat("construct anchor %d envelope: %w", i, err)
		}

		// Check the signatures
		ok, err = c.didSign(ctx, txn.ID())
		if err != nil {
			return errors.UnknownError.WithFormat("query anchor %d status on %v: %w", i, destination, err)
		}
		if ok {
			// If we've already signed this one, skip it
			continue
		}

		// Submit it
		err = c.submit(ctx, destination, env)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Conductor) didSign(ctx context.Context, id *url.TxID) (bool, error) {
	r, err := c.Querier.QueryTransaction(ctx, id, nil)
	switch {
	case err == nil:
		for _, sigs := range r.Signatures.Records {
			for _, sig := range sigs.Signatures.Records {
				msg, ok := sig.Message.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if bytes.Equal(msg.Signature.GetPublicKey(), c.ValidatorKey[32:]) {
					return true, nil
				}
			}
		}
		return false, nil

	case errors.Is(err, errors.NotFound):
		return false, nil

	default:
		return false, errors.UnknownError.Wrap(err)
	}
}

// ConstructLastAnchor constructs an anchor for the previous block. This *must* be
// called with a batch created during BeginBlock, prior to updating any database
// records.
func ConstructLastAnchor(ctx context.Context, batch *database.Batch, partition *url.URL) (protocol.AnchorBody, uint64, error) {
	var systemLedger *protocol.SystemLedger
	err := batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&systemLedger)
	if err != nil {
		return nil, 0, errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Is there an anchor to send?
	if systemLedger.Anchor == nil {
		slog.DebugCtx(ctx, "Skipping anchor", "module", "anchoring", "index", systemLedger.Index)
		return nil, 0, nil
	}

	// Load the anchor ledger state
	var anchorLedger *protocol.AnchorLedger
	err = batch.Account(partition.JoinPath(protocol.AnchorPool)).Main().GetAs(&anchorLedger)
	if err != nil {
		return nil, 0, errors.UnknownError.WithFormat("load anchor ledger: %w", err)
	}

	// Send the block anchor
	sequenceNumber := anchorLedger.MinorBlockSequenceNumber
	slog.DebugCtx(ctx, "Anchor block", "module", "anchoring", "index", systemLedger.Index, "seq-num", sequenceNumber)

	// Load the root chain
	rootChain, err := batch.Account(partition.JoinPath(protocol.Ledger)).RootChain().Get()
	if err != nil {
		return nil, 0, errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	stateRoot, err := batch.BPT().GetRootHash()
	if err != nil {
		return nil, 0, errors.UnknownError.WithFormat("load state root: %w", err)
	}

	// Construct the anchor
	anchor := systemLedger.Anchor.CopyAsInterface().(protocol.AnchorBody)
	partAnchor := anchor.GetPartitionAnchor()
	partAnchor.RootChainIndex = uint64(rootChain.Height()) - 1
	partAnchor.RootChainAnchor = *(*[32]byte)(rootChain.Anchor())
	partAnchor.StateTreeAnchor = stateRoot
	return anchor, sequenceNumber, nil
}

type ValidatorContext struct {
	Source       *protocol.PartitionInfo
	Globals      *network.GlobalValues
	ValidatorKey ed25519.PrivateKey
}

func (x ValidatorContext) Url(path ...string) *url.URL {
	return protocol.PartitionUrl(x.Source.ID).JoinPath(path...)
}

func (x ValidatorContext) PrepareAnchorSubmission(ctx context.Context, anchor protocol.AnchorBody, sequenceNumber uint64, destination *url.URL) (*messaging.Envelope, *protocol.Transaction, error) {
	isSrcDir := x.Source.Type == protocol.PartitionTypeDirectory
	isDstDir := destination.Equal(protocol.DnUrl())
	if isSrcDir && isDstDir {
		// An anchor sent from the DN to itself must not set MakeMajorBlock.
		// Make a copy to avoid modifying the original.
		v := anchor.(*protocol.DirectoryAnchor)
		v = v.Copy()
		v.MakeMajorBlock = 0
		anchor = v
	}

	// If we're on the DN, the last block updated to v2, and the destination is
	// a BVN, then we must send out the anchor as a v1 anchor since the BVNs
	// will still be running v1
	if isSrcDir && didUpdateToV2(anchor) && !isDstDir {
		env, err := shared.PrepareBlockAnchor(x.Url(), x.Globals.Network, x.ValidatorKey, anchor, sequenceNumber, destination)
		if err != nil {
			return nil, nil, errors.UnknownError.Wrap(err)
		}
		return env, env.Transaction[0], nil
	}

	// Create the transaction
	txn := new(protocol.Transaction)
	txn.Header.Principal = destination.JoinPath(protocol.AnchorPool)
	txn.Body = anchor

	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      x.Url(),
		Destination: destination,
		Number:      sequenceNumber,
	}

	// Create a key signature
	h := seq.Hash()
	keySig, err := x.signTransaction(h[:])
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	// Construct the envelope
	return &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.BlockAnchor{
				Anchor:    seq,
				Signature: keySig,
			},
		},
	}, txn, nil
}

func (x ValidatorContext) signTransaction(hash []byte) (protocol.KeySignature, error) {
	if x.ValidatorKey == nil {
		return nil, errors.InternalError.WithFormat("attempted to sign with a nil key")
	}

	sig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(x.ValidatorKey).
		SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
		SetVersion(x.Globals.Network.Version).
		SetTimestampToNow().
		Sign(hash)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	ks, ok := sig.(protocol.KeySignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("expected key signature, got %v", sig.Type())
	}

	return ks, nil
}

func didUpdateToV2(anchor protocol.AnchorBody) bool {
	dir, ok := anchor.(*protocol.DirectoryAnchor)
	if !ok {
		return false
	}

	for _, update := range dir.Updates {
		update, ok := update.Body.(*protocol.ActivateProtocolVersion)
		if ok && update.Version == protocol.ExecutorVersionV2 {
			return true
		}
	}
	return false
}
