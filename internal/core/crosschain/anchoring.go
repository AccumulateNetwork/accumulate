// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (c *Conductor) healAnchors(ctx context.Context, batch *database.Batch, destination *url.URL, currentBlock uint64) error {
	if !def(c.EnableAnchorHealing, true) {
		return nil
	}

	// The source-side push is NOT retired under Kourou, and the attempt to do so
	// is what left this branch unable to recover a lost anchor at all (#4103).
	//
	// The reasoning for retiring it was that the destination owns recovery once
	// collection proofs are active (#4048, #4056). That is true only for gaps
	// the destination can SEE: recoverAnchorsViaRange scans its pending window
	// for holes, and a hole exists only because a later anchor arrived to expose
	// it. An anchor dropped before anything followed it leaves no hole, no
	// pending entry, and nothing for the destination to ask about —
	// TestDropInitialAnchor is exactly that case, and it is why main keeps the
	// push.
	//
	// So the two are complements rather than alternatives. The push covers what
	// the destination cannot see; the pull covers runs cheaply once it can, and
	// keeps the push from having to walk large gaps. Retiring either one leaves
	// a class of loss permanent, because dispatch itself never retries.

	// Load the source sequence chain
	sequence := batch.Account(c.Url(protocol.AnchorPool)).AnchorSequenceChain()
	head, err := sequence.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor sequence chain head: %w", err)
	}

	// Paced to HealInterval, so this is cheap — and scan visibility is what
	// makes silent anchor-delivery failures diagnosable (#4054). Log BEFORE
	// querying the destination: if the destination is unreachable the query
	// blocks until the scan deadline, and a scan that never logs is
	// indistinguishable from healing being broken (#4056).
	slog.Debug("Anchor heal scan", "module", "conductor",
		"source", c.Url(),
		"destination", destination,
		"produced", head.Count,
		"currentBlock", currentBlock)

	// Load the destination anchor ledger
	var ledger1 *protocol.AnchorLedger
	_, err = c.Querier.QueryAccountAs(ctx, destination.JoinPath(protocol.AnchorPool), nil, &ledger1)
	if err != nil {
		return errors.UnknownError.WithFormat("query %v anchor ledger: %w", destination, err)
	}
	ledger2 := ledger1.Partition(c.Url())

	// Heal only when delivery is genuinely stalled. A destination that is
	// delivering our anchors — including one merely catching up — advances its
	// Delivered count between scans and needs no help; re-driving its in-flight
	// anchors would just add cross-partition traffic. Only a stalled Delivered
	// marks the next anchor as stuck and in need of resubmission.
	if !c.deliveryStalled(destination.String(), ledger2.Delivered, uint64(head.Count)) {
		return nil
	}

	// For each not-yet delivered anchor — capped per scan. Delivery is
	// sequential, so the stuck head is what matters; re-driving the entire
	// backlog every scan is pure load with no delivery benefit, and with the
	// backlog in the thousands it was one of the storm engines of the
	// 20260819T234054Z collapse: each scan issued one didSign query plus one
	// submission per missing anchor (#4115). The next scan continues from
	// whatever the destination has delivered by then.
	const healAnchorScanCap = 16
	resubmitted := 0
	for i := ledger2.Delivered + 1; i <= uint64(head.Count) && resubmitted < healAnchorScanCap; i++ {
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
			slog.Info("Heal: anchor too recent", "module", "conductor",
				"destination", destination, "sequence", i,
				"anchorBlock", anchor.GetPartitionAnchor().MinorBlockIndex,
				"currentBlock", currentBlock)
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
			slog.Info("Heal: already signed", "module", "conductor",
				"destination", destination, "sequence", i, "txid", txn.ID())
			continue
		}

		// Submit it
		slog.Info("Healing anchor", "module", "conductor",
			"source", c.Url(),
			"destination", destination,
			"sequence", i,
			"delivered", ledger2.Delivered)
		err = c.submit(ctx, destination, env)
		if err != nil {
			return err
		}
		resubmitted++
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

// recoverAnchorsViaRange pulls the run of anchors this partition is missing
// from source, with ONE range request carrying ONE collection proof covering
// every anchor in it (#4087).
//
// This inverts who owns anchor recovery. The source-side push in healAnchors
// has every validator independently re-sign and re-submit each historical
// anchor, and consults the destination once per candidate anchor to see whether
// it already signed — so the query load grows with the size of the gap, aimed at
// a destination that is by definition behind. Main's own comment records the
// cost: 1.2M re-submissions and 14 cores in 25 minutes of the #4067 soak.
//
// Pulling instead costs one request and one proof regardless of gap size, and
// the proof is the authorization, so no signature quorum has to be re-gathered.
//
// The invariant that makes this always serviceable: only ever ask for anchors
// BELOW something we already hold. An anchor is a merkle root committing to
// every prior entry, so a gap is visible only because a later anchor arrived —
// and that later anchor is the root the recovered run proves against. Asking
// above it would be asking the source to prove something it has not anchored
// yet, which is exactly the failure that produced 1,492 errors in ten minutes
// in #4086.
func (c *Conductor) recoverAnchorsViaRange(ctx context.Context, batch *database.Batch, source *url.URL) error {
	if !c.Globals.Load().ExecutorVersion.V2KourouEnabled() {
		return nil
	}
	ranger, ok := c.Sequencer.(private.SequenceRanger)
	if !ok {
		return nil // Peer does not serve ranges; the per-message path still applies
	}

	// Our own ledger: what have we received from that source?
	var ledger *protocol.AnchorLedger
	err := batch.Account(c.Url(protocol.AnchorPool)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger: %w", err)
	}
	part := ledger.Partition(source)

	// The FIRST CONTIGUOUS run of anchors we do not have.
	//
	// Contiguous matters: the source proves the run against the anchor numbered
	// last+1, so that anchor has to be one we hold. Taking every hole at once
	// would produce a range spanning entries we already have, and last+1 would
	// then be a different anchor than the one bounding the first hole. One run
	// per scan; the next block's scan takes the next.
	//
	// The run is always bounded above, which is the whole reason a hole is
	// visible at all: what we have SIGHTED reaches only as far as the highest
	// anchor staged. A gap exists because something later arrived — and that
	// later something is what proves the gap's contents.
	//
	// Asked of staging, not of the ledger's pending array (#4189). The array is
	// what the block wrote, and past its bound the executor stored an anchor
	// while refusing to record it, so the ledger reported holes for anchors
	// already in the local database.
	id := c.anchorStream(source)
	runs, err := execute.Missing(batch, id, part.Delivered, c.sightedOn(batch, id, part.Delivered), 1)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if len(runs) == 0 {
		return nil // Nothing missing
	}
	first, last := runs[0][0], runs[0][1]
	if last-first+1 > protocol.MaxReceiptListElements {
		last = first + protocol.MaxReceiptListElements - 1
	}

	// Throttle per (source, run) so every block does not re-pull the same run.
	if !c.claimSyntheticRequest(source.JoinPath("anchor-range"), first, time.Now()) {
		return nil
	}

	// Warn, not Info: nodes default to error level for this module, so an Info
	// line here is invisible on a real deployment — and a recovery that cannot be
	// observed is indistinguishable from one that never ran, which is how #4073
	// hid a completely broken pull path across six soak runs. Reaching this point
	// means anchors were lost outright, which is worth surfacing anyway.
	slog.WarnContext(ctx, "Recovering anchors by range", "module", "conductor",
		"source", source, "start", first, "end", last)

	records, err := ranger.SequenceRange(ctx, source.JoinPath(protocol.AnchorPool), c.Url(), first, last, private.SequenceOptions{})
	if err != nil {
		return errors.UnknownError.WithFormat("request anchor range [%d, %d] from %v: %w", first, last, source, err)
	}
	if len(records) == 0 {
		return nil
	}

	// One proof, carried on the last record, covers the whole run.
	list := records[len(records)-1].SourceReceiptList
	if list == nil {
		return errors.InvalidRecord.WithFormat("anchor range response from %v carries no collection proof", source)
	}
	// The proof terminates at a root of the SOURCE, not of the directory: it is
	// the source's own root chain, committed by the anchor we already hold from
	// it (#4087).
	proof := &protocol.AnnotatedReceipt{
		ReceiptList: list,
		Anchor:      &protocol.AnchorMetadata{Account: source},
	}

	for _, r := range records {
		if r.Message == nil || r.Sequence == nil || r.Sequence.Source == nil {
			return errors.InvalidRecord.WithFormat("anchor range response from %v is missing required fields", source)
		}
		// The collection proof is the authorization, but the serving node's
		// signature is carried through as well: BlockAnchor.ID() derives the
		// message identity from the signer, so an anchor with no signature
		// panics before it can be validated. The signature is not what makes
		// the anchor acceptable here — the proof is — it is what keeps identity
		// and the pre-Kourou checks working.
		keySig := anchorKeySignature(r)
		if keySig == nil {
			return errors.InvalidRecord.WithFormat("anchor %d in range response from %v carries no signature", r.Sequence.Number, source)
		}
		msg := &messaging.BlockAnchor{
			Anchor: &messaging.SequencedMessage{
				Message:     r.Message,
				Source:      r.Sequence.Source,
				Destination: r.Sequence.Destination,
				Number:      r.Sequence.Number,
			},
			Signature: keySig,
			Proof:     proof,
		}
		err = c.submit(ctx, c.Url(), &messaging.Envelope{Messages: []messaging.Message{msg}})
		if err != nil {
			return errors.UnknownError.WithFormat("submit recovered anchor %d: %w", r.Sequence.Number, err)
		}
		mHeals.WithLabelValues("anchor-range", c.Partition.ID, partitionLabel(source)).Inc()
		if c.Heals != nil {
			c.Heals.Anchor.Add(1)
		}
	}
	return nil
}

// anchorKeySignature extracts the serving node's key signature from a range
// response record, if it has one.
func anchorKeySignature(r *api.MessageRecord[messaging.Message]) protocol.KeySignature {
	if r.Signatures == nil {
		return nil
	}
	for _, set := range r.Signatures.Records {
		if set.Signatures == nil {
			continue
		}
		for _, sr := range set.Signatures.Records {
			msg, ok := sr.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			if sig, ok := msg.Signature.(protocol.KeySignature); ok {
				return sig
			}
		}
	}
	return nil
}
