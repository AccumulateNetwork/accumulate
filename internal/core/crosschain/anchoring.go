// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"crypto/ed25519"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
		slog.DebugContext(ctx, "Skipping anchor", "module", "anchoring", "index", systemLedger.Index)
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
	slog.DebugContext(ctx, "Anchor block", "module", "anchoring", "index", systemLedger.Index, "seq-num", sequenceNumber)

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
	// An anchor sent from the DN to itself must not set MakeMajorBlock. Make a
	// copy to avoid modifying the original.
	//
	// Make no change on Vandenberg and after (all anchors sent from the DN
	// should be identical).
	isSrcDir := x.Source.Type == protocol.PartitionTypeDirectory
	isDstDir := destination.Equal(protocol.DnUrl())
	if !x.Globals.ExecutorVersion.V2VandenbergEnabled() && isSrcDir && isDstDir {
		v := anchor.(*protocol.DirectoryAnchor)
		v = v.Copy()
		v.MakeMajorBlock = 0
		anchor = v
	}

	// If we're on the DN, the last block updated to v2, and the destination is
	// a BVN, then we must send out the anchor as a v1 anchor since the BVNs
	// will still be running v1 (not relevant after v2)
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
