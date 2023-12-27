// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/light"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

type Healer struct {
	receivedAnchors map[string][]*light.AnchorMetadata
}

func (h *Healer) Reset() {
	h.receivedAnchors = nil
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

	// Has it already been delivered?
	Q := api.Querier2{Querier: args.Querier}
	if r, err := Q.QueryMessage(ctx, r.ID, nil); err == nil && r.Status.Delivered() {
		return nil
	}

	slog.InfoCtx(ctx, "Resubmitting", "source", si.Source, "destination", si.Destination, "number", si.Number, "id", r.Message.ID())

	// Build the receipt
	receipt, err := h.buildSynthReceipt(ctx, args, si)
	if err != nil {
		return err
	}

	// Submit the synthetic transaction directly to the destination partition
	msg := &messaging.BadSyntheticMessage{
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

	hash := msg.Message.Hash()
	if !msg.Signature.Verify(nil, hash[:]) {
		return fmt.Errorf("signature is not valid")
	}

	dontWait := map[[32]byte]bool{}
	env := new(messaging.Envelope)
	env.Messages = []messaging.Message{msg}
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

	// Submit directly to an appropriate node
	if c, ok := args.Submitter.(message.AddressedClient); ok && c.Address == nil {
		for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Destination)] {
			if len(info.Addresses) > 0 {
				args.Submitter = c.ForAddress(info.Addresses[0]).ForPeer(peer)
			} else {
				args.Submitter = c.ForPeer(peer)
			}
			break
		}
	}

	sub, err := args.Submitter.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		slog.ErrorCtx(ctx, "Submission failed", "error", err, "id", env.Messages[0].ID())
	}
	for _, sub := range sub {
		if !sub.Success {
			slog.ErrorCtx(ctx, "Submission failed", "message", sub, "status", sub.Status, "id", sub.Status.TxID)
			continue
		}

		slog.InfoCtx(ctx, "Submission succeeded", "id", sub.Status.TxID)
		if !args.Wait || dontWait[sub.Status.TxID.Hash()] {
			continue
		}

		err := waitFor(ctx, Q, sub.Status.TxID)
		if err != nil && strings.HasSuffix(err.Error(), " is not a known directory anchor") {
			return ErrRetry
		}
	}

	if args.Wait {
		return waitFor(ctx, Q, si.ID)
	}
	return nil
}

func waitFor(ctx context.Context, Q api.Querier, id *url.TxID) error {
	slog.InfoCtx(ctx, "Waiting", "for", id)
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

func (h *Healer) buildSynthReceipt(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
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
	_, mainIndex, err := batch.Index().Account(uSynth).Chain("main").Index().Find(light.ByIndexSource(seqEntry.Source))
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
	if h.receivedAnchors == nil {
		h.receivedAnchors = map[string][]*light.AnchorMetadata{}
	}
	dnAnchors, ok := h.receivedAnchors[si.Destination]
	if !ok {
		dnAnchors, err = batch.Index().Partition(protocol.PartitionUrl(si.Destination)).Anchors().Received().Get()
		if err != nil {
			return nil, err
		}
		h.receivedAnchors[si.Destination] = dnAnchors
	}

	var anchoredAnchor *protocol.PartitionAnchorReceipt
	if !strings.EqualFold(si.Source, protocol.Directory) {
		// Find a DN anchor that anchors the source block
		anchoredAnchor, err = getAnchorForBlockAnchor(dnAnchors, uSrc, mainIndex.BlockIndex)
		if err != nil {
			return nil, err
		}

	} else {
		// Find the DN anchor for the given block
		dnAnchor, err := getAnchorForBlock(dnAnchors, mainIndex.BlockIndex, 0)
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
	_, rootIndex, err := batch.Index().Account(uSys).Chain("root").Index().Find(light.ByIndexBlock(anchoredAnchor.Anchor.MinorBlockIndex))
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
	dnAnchor, err := getAnchorForBlock(dnAnchors, sourceRootIndex.BlockIndex, args.SkipAnchors)
	if err != nil {
		return nil, err
	}

	dnSys := protocol.DnUrl().JoinPath(protocol.Ledger)
	_, dnRootIndex, err := batch.Index().Account(dnSys).Chain("root").Index().Find(light.ByIndexBlock(dnAnchor.MinorBlockIndex))
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

func getAnchorForBlockAnchor(anchors []*light.AnchorMetadata, source *url.URL, block uint64) (*protocol.PartitionAnchorReceipt, error) {
	for _, anchor := range anchors {
		body, ok := anchor.Transaction.Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
		}
		for _, r := range body.Receipts {
			if r.Anchor.Source.Equal(source) && r.Anchor.MinorBlockIndex >= block {
				return r, nil
			}
		}
	}
	return nil, errors.NotFound.WithFormat("unable to locate DN anchor for %v block %d or greater", source, block)
}

func getAnchorForBlock(anchors []*light.AnchorMetadata, block uint64, skip int) (*protocol.DirectoryAnchor, error) {
	for _, anchor := range anchors {
		body, ok := anchor.Transaction.Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
		}
		if body.MinorBlockIndex < block {
			continue
		}
		if skip > 0 {
			skip--
			continue
		}
		return body, nil
	}
	return nil, errors.NotFound.WithFormat("unable to locate DN anchor for block %d (skip %d)", block, skip)
}
