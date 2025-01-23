// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/light"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Healer struct {
}

func (h *Healer) Reset() {
}

type HealSyntheticArgs struct {
	Client      message.AddressedClient
	Querier     api.Querier
	Submitter   api.Submitter
	NetInfo     *NetworkInfo
	Light       *light.Client
	Pretend     bool
	Wait        bool
	SkipAnchors int
}

func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
	if args.Querier == nil {
		args.Querier = args.Client
	}
	if args.Submitter == nil {
		args.Submitter = args.Client
	}

	// Query the synthetic transaction
	r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
	if err != nil {
		return err
	}
	si.ID = r.ID

	// Query the status
	Q := api.Querier2{Querier: args.Querier}
	if s, err := Q.QueryMessage(ctx, r.ID, nil); err == nil &&
		// Has it already been delivered?
		s.Status.Delivered() &&
		// Does the sequence info match?
		s.Sequence != nil &&
		s.Sequence.Source.Equal(protocol.PartitionUrl(si.Source)) &&
		s.Sequence.Destination.Equal(protocol.PartitionUrl(si.Destination)) &&
		s.Sequence.Number == si.Number {
		// If it's been delivered (and the sequence ID of the delivered message
		// matches what was passed to this call), skip it. If it's been
		// delivered with a different sequence ID, something weird is going on
		// so resubmit it anyways.
		slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
		return errors.Delivered
	}

	slog.InfoContext(ctx, "Resubmitting", "source", si.Source, "destination", si.Destination, "number", si.Number, "id", r.Message.ID())

	// Build the receipt
	var receipt *merkle.Receipt
	if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
		receipt, err = h.buildSynthReceiptV2(ctx, args, si)
	} else {
		receipt, err = h.buildSynthReceiptV1(ctx, args, si)
	}
	if err != nil {
		return err
	}

	// Submit the synthetic transaction directly to the destination partition
	msg := &messaging.SyntheticMessage{
		Message: r.Sequence,
		Proof: &protocol.AnnotatedReceipt{
			Receipt: receipt,
			Anchor: &protocol.AnchorMetadata{
				Account: protocol.DnUrl(),
			},
		},
	}
	for _, sigs := range r.Signatures.Records {
		for _, sig := range sigs.Signatures.Records {
			sig, ok := sig.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			ks, ok := sig.Signature.(protocol.KeySignature)
			if !ok {
				continue
			}
			msg.Signature = ks
		}
	}
	if msg.Signature == nil {
		return fmt.Errorf("synthetic message is not signed")
	}

	if !msg.Signature.Verify(nil, msg.Message) {
		return fmt.Errorf("signature is not valid")
	}

	dontWait := map[[32]byte]bool{}
	env := new(messaging.Envelope)

	if args.NetInfo.Status.ExecutorVersion.V2BaikonurEnabled() {
		env.Messages = []messaging.Message{msg}
	} else {
		env.Messages = []messaging.Message{&messaging.BadSyntheticMessage{
			Message:   msg.Message,
			Signature: msg.Signature,
			Proof:     msg.Proof,
		}}
	}
	if msg, ok := r.Message.(messaging.MessageForTransaction); ok {
		hash := msg.GetTxID().Hash()
		dontWait[hash] = true
		id := protocol.PartitionUrl(si.Source).WithTxID(hash)
		r, err := Q.QueryTransaction(ctx, id, nil)
		if err != nil {
			return errors.InternalError.WithFormat("query transaction for message: %w", err)
		}
		env.Messages = append(env.Messages, r.Message)
	}

	if args.Pretend {
		return nil
	}

	submit := func(s api.Submitter) (error, bool) {
		sub, err := s.Submit(ctx, env, api.SubmitOptions{})
		if err != nil {
			slog.ErrorContext(ctx, "Submission failed", "error", err, "id", env.Messages[0].ID())
			return nil, false
		}
		for _, sub := range sub {
			if !sub.Success {
				slog.ErrorContext(ctx, "Submission failed", "message", sub, "status", sub.Status, "id", sub.Status.TxID)
				continue
			}

			slog.InfoContext(ctx, "Submission succeeded", "id", sub.Status.TxID)
			if !args.Wait || dontWait[sub.Status.TxID.Hash()] {
				continue
			}

			err := waitFor(ctx, Q, sub.Status.TxID)
			if err != nil && strings.HasSuffix(err.Error(), " is not a known directory anchor") {
				return ErrRetry, false
			}
		}

		if args.Wait {
			return waitFor(ctx, Q, si.ID), true
		}
		return nil, true
	}

	// Submit directly to an appropriate node
	switch submitter := args.Submitter.(type) {
	// case message.AddressedClient:

	case *message.Client:
		for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Destination)] {
			var s api.Submitter
			if len(info.Addresses) > 0 {
				s = submitter.ForAddress(info.Addresses[0]).ForPeer(peer)
			} else {
				s = submitter.ForPeer(peer)
			}
			slog.Info("Submitting to", "peer", peer)
			err, ok := submit(s)
			if ok || err != nil {
				return err
			}
		}
		return errors.UnknownError.With("failed to submit")

	default:
		err, _ := submit(args.Submitter)
		return err
	}
}

func waitFor(ctx context.Context, Q api.Querier, id *url.TxID) error {
	slog.InfoContext(ctx, "Waiting", "for", id)
	for i := 0; i < 10; i++ {
		r, err := api.Querier2{Querier: Q}.QueryMessage(ctx, id, nil)
		switch {
		case errors.Is(err, errors.NotFound):
			// Not found, wait
			slog.Info("Status", "id", id, "code", errors.NotFound)

		case err != nil:
			// Unknown error
			return err

		case !r.Status.Delivered():
			// Pending, wait
			slog.Info("Status", "id", id, "code", r.Status)

		case r.Error != nil:
			slog.Error("Failed", "id", id, "error", r.Error)
			return r.AsError()

		default:
			slog.Info("Delivered", "id", id)
			return nil
		}
		time.Sleep(time.Second / 2)
	}
	return ErrRetry
}

func (h *Healer) buildSynthReceiptV1(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
	batch := args.Light.OpenDB(false)
	defer batch.Discard()
	uSrc := protocol.PartitionUrl(si.Source)
	uSys := uSrc.JoinPath(protocol.Ledger)
	uSynth := uSrc.JoinPath(protocol.Synthetic)

	// Load the synthetic sequence chain entry
	b, err := batch.Account(uSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
	if err != nil {
		return nil, err
	}
	seqEntry := new(protocol.IndexEntry)
	err = seqEntry.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	// Locate the synthetic ledger main chain index entry
	mainIndex, err := batch.Index().Account(uSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
	if err != nil {
		return nil, err
	}

	// Build the synthetic ledger part of the receipt
	mainReceipt, err := batch.Account(uSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
	if err != nil {
		return nil, err
	}

	// Search the DN anchors received by the *destination* partition for one
	// that anchors the synthetic transaction
	dnAnchors := batch.Index().Partition(protocol.PartitionUrl(si.Destination)).Anchors()

	var anchoredAnchor *protocol.PartitionAnchorReceipt
	if !strings.EqualFold(si.Source, protocol.Directory) {
		// Find a DN anchor that anchors the source block
		anchoredAnchor, err = getAnchorForBlockAnchor(batch, dnAnchors, uSrc, mainIndex.BlockIndex)
		if err != nil {
			return nil, err
		}

	} else {
		// Find the DN anchor for the given block
		dnAnchor, err := getAnchorForBlock(batch, dnAnchors, mainIndex.BlockIndex, 0)
		if err != nil {
			return nil, err
		}

		// Build a fake partition anchor receipt
		anchoredAnchor = &protocol.PartitionAnchorReceipt{
			Anchor: &dnAnchor.PartitionAnchor,
			RootChainReceipt: &merkle.Receipt{
				Start:  dnAnchor.RootChainAnchor[:],
				Anchor: dnAnchor.RootChainAnchor[:],
			},
		}
	}

	// Locate the root chain index entry
	rootIndex, err := batch.Index().Account(uSys).Chain("root").BlockIndex().FindExactIndexEntry(anchoredAnchor.Anchor.MinorBlockIndex)
	if err != nil {
		return nil, err
	}

	// Build the root chain part of the receipt
	rootReceipt, err := batch.Account(uSys).RootChain().Receipt(mainIndex.Anchor, rootIndex.Source)
	if err != nil {
		return nil, err
	}

	// Combine the receipts
	if args.SkipAnchors == 0 {
		return mainReceipt.Combine(rootReceipt, anchoredAnchor.RootChainReceipt)
	}

	// Find the index entry of the source anchor's entry in the DN's
	// corresponding root anchor chain
	dnSourceRoots := batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(si.Source).Root()
	sourceRootHeight, err := dnSourceRoots.IndexOf(anchoredAnchor.Anchor.RootChainAnchor[:])
	if err != nil {
		return nil, err
	}
	sourceRootIndexHead, err := dnSourceRoots.Index().Head().Get()
	if err != nil {
		return nil, err
	}
	_, sourceRootIndex, err := indexing.SearchIndexChain2(dnSourceRoots.Index(), uint64(sourceRootIndexHead.Count)-1, indexing.MatchAfter, indexing.SearchIndexChainBySource(uint64(sourceRootHeight)))
	if err != nil {
		return nil, err
	}

	dnSourceRootReceipt, err := dnSourceRoots.Receipt(uint64(sourceRootHeight), sourceRootIndex.Source)
	if err != nil {
		return nil, err
	}

	// Find a DN anchor received by the source, after the given block
	dnAnchor, err := getAnchorForBlock(batch, dnAnchors, sourceRootIndex.BlockIndex, args.SkipAnchors)
	if err != nil {
		return nil, err
	}

	dnSys := protocol.DnUrl().JoinPath(protocol.Ledger)
	dnRootIndex, err := batch.Index().Account(dnSys).Chain("root").BlockIndex().FindIndexEntryAfter(dnAnchor.MinorBlockIndex)
	if err != nil {
		return nil, err
	}

	// Build the root chain part of the receipt
	dnRootReceipt, err := batch.Account(dnSys).RootChain().Receipt(sourceRootIndex.Anchor, dnRootIndex.Source)
	if err != nil {
		return nil, err
	}

	// Combine the receipts
	return mainReceipt.Combine(rootReceipt, dnSourceRootReceipt, dnRootReceipt)
}

func getAnchorForBlockAnchor(batch *light.DB, anchors *light.IndexDBPartitionAnchors, source *url.URL, block uint64) (*protocol.PartitionAnchorReceipt, error) {
	index := anchors.Received(source)
	i, err := index.Find(block).After().Index()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate DN anchor for %v block %d: %w", source, block, err)
	}
	b, err := index.Chain().Entry(int64(i))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load DN anchor chain entry for %v block %d: %w", source, block, err)
	}
	var msg *messaging.TransactionMessage
	err = batch.Message2(b).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load DN anchor for %s block %d: %w", source, block, err)
	}
	body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
	if !ok {
		return nil, errors.InternalError.WithFormat("unable to locate DN anchor for %s block %d: not an anchor", source, block)
	}
	for _, r := range body.Receipts {
		if r.Anchor.Source.Equal(source) && r.Anchor.MinorBlockIndex >= block {
			return r, nil
		}
	}
	return nil, errors.InternalError.WithFormat("unable to locate DN anchor for %s block %d: internal error: receipt is missing", source, block)
}

func getAnchorForBlock(batch *light.DB, anchors *light.IndexDBPartitionAnchors, block uint64, skip int) (*protocol.DirectoryAnchor, error) {
	index := anchors.Received(protocol.DnUrl())
	i, err := index.Find(block).After().Index()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate DN anchor for block %d: %w", block, err)
	}

	head, err := index.Chain().Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load DN anchor chain head: %w", err)
	}

	// TODO Use Next() on the chain search result to skip, once that method has
	// been added.

	for j := int64(skip); int64(i)+j < head.Count; j++ {
		body, err := loadAnchorFromChain[protocol.DirectoryAnchor](batch, index.Chain(), block, int64(i), j)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if body != nil {
			return body, nil
		}
	}
	return nil, errors.UnknownError.WithFormat("6 %d (skip %d): reached end of chain", block, skip)
}

func loadAnchorFromChain[V any](batch *light.DB, chain *database.Chain2, block uint64, i, skip int64) (*V, error) {
	b, err := chain.Entry(i + skip)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load DN anchor chain entry for block %d (skip %d): %w", block, skip, err)
	}
	var msg *messaging.TransactionMessage
	err = batch.Message2(b).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load DN anchor for block %d (skip %d): %w", block, skip, err)
	}
	body, ok := any(msg.Transaction.Body).(*V)
	if !ok {
		if skip > 0 {
			return nil, nil
		}
		return nil, errors.UnknownError.WithFormat("unable to locate DN anchor for block %d (skip %d): want %v, got %v", block, skip, protocol.TransactionTypeDirectoryAnchor, msg.Transaction.Body.Type())
	}
	return body, nil
}

func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
	batch := args.Light.OpenDB(false)
	defer batch.Discard()
	uSrc := protocol.PartitionUrl(si.Source)
	uSrcSys := uSrc.JoinPath(protocol.Ledger)
	uSrcSynth := uSrc.JoinPath(protocol.Synthetic)
	uDn := protocol.DnUrl()
	uDnSys := uDn.JoinPath(protocol.Ledger)
	uDnAnchor := uDn.JoinPath(protocol.AnchorPool)

	// Load the synthetic sequence chain entry
	b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"load synthetic sequence chain entry %d: %w", si.Number, err)
	}
	seqEntry := new(protocol.IndexEntry)
	err = seqEntry.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	// Locate the synthetic ledger main chain index entry
	mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"locate synthetic ledger main chain index entry after %d: %w", seqEntry.Source, err)
	}

	// Build the synthetic ledger part of the receipt
	receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"build synthetic ledger receipt: %w", err)
	}

	// Locate the BVN root index entry
	bvnRootIndex, err := batch.Index().Account(uSrcSys).Chain("root").SourceIndex().FindIndexEntryAfter(mainIndex.Anchor)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"locate BVN root index entry after %d: %w", mainIndex.Anchor, err)
	}

	// Build the BVN part of the receipt
	bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"build BVN receipt: %w", err)
	}
	receipt, err = receipt.Combine(bvnReceipt)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"append BVN receipt: %w", err)
	}

	// If the source is the DN we don't need to do anything else
	if strings.EqualFold(si.Source, protocol.Directory) {
		return receipt, nil
	}

	// Locate the DN-BVN anchor entry
	dnBvnAnchorChain := batch.Account(uDnAnchor).AnchorChain(si.Source).Root()
	bvnAnchorHeight, err := dnBvnAnchorChain.IndexOf(receipt.Anchor)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"locate DN-BVN anchor entry: %w", err)
	}

	// Locate the DN-BVN anchor index entry
	bvnAnchorIndex, err := batch.Index().Account(uDnAnchor).Chain(dnBvnAnchorChain.Name()).SourceIndex().FindIndexEntryAfter(uint64(bvnAnchorHeight))
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"locate DN-BVN anchor index entry after %d: %w", bvnAnchorHeight, err)
	}

	// Build the DN-BVN part of the receipt
	bvnDnReceipt, err := dnBvnAnchorChain.Receipt(uint64(bvnAnchorHeight), bvnAnchorIndex.Source)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"build DN-BVN receipt: %w", err)
	}
	receipt, err = receipt.Combine(bvnDnReceipt)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"append DN-BVN receipts: %w", err)
	}

	// Locate the DN root index entry
	dnRootIndex, err := batch.Index().Account(uDnSys).Chain("root").SourceIndex().FindIndexEntryAfter(bvnAnchorIndex.Anchor)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"locate DN root index entry after %d: %w", bvnAnchorIndex.Anchor, err)
	}

	// Build the DN part of the receipt
	dnReceipt, err := batch.Account(uDnSys).RootChain().Receipt(bvnAnchorIndex.Anchor, dnRootIndex.Source)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"build DN receipt: %w", err)
	}
	receipt, err = receipt.Combine(dnReceipt)
	if err != nil {
		return nil, errors.UnknownError.WithFormat(
			"append DN receipt: %w", err)
	}

	return receipt, nil
}
