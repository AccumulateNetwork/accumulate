// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

func loadMessage(batch *database.Batch, txid *url.TxID) (*api.MessageRecord[messaging.Message], error) {
	r := new(api.MessageRecord[messaging.Message])

	// Load the message
	var err error
	r.Message, err = batch.Message(txid.Hash()).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load message: %w", err)
	}
	r.ID = r.Message.ID()

	// Load the status
	status, err := batch.Transaction2(txid.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	r.Status = status.Code
	r.Error = status.Error
	r.Result = status.Result
	r.Received = status.Received

	// Load produced and cause
	p1, err := batch.Transaction2(txid.Hash()).Produced().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load produced: %w", err)
	}
	p2, err := batch.Message(txid.Hash()).Produced().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load produced: %w", err)
	}
	r.Produced, _ = api.MakeRange(append(p1, p2...), 0, 0, func(v *url.TxID) (*api.TxIDRecord, error) {
		return &api.TxIDRecord{Value: v}, nil
	})

	cause, err := batch.Message(txid.Hash()).Cause().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load cause: %w", err)
	}
	r.Cause, _ = api.MakeRange(cause, 0, 0, func(v *url.TxID) (*api.TxIDRecord, error) {
		return &api.TxIDRecord{Value: v}, nil
	})

	// Look for a sequenced message cause
	for _, id := range cause {
		msg, err := batch.Message(id.Hash()).Main().Get()
		switch {
		case err == nil:
			// Ok
		case errors.Code(err).IsClientError():
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load cause %v: %w", id, err)
		}

		if seq, ok := msg.(*messaging.SequencedMessage); ok {
			r.Sequence = seq
			break
		}
	}

	// Load the transaction-specific fields
	r.Signatures = new(api.RecordRange[*api.SignatureSetRecord])

	err = loadTransactionSignaturesV1(batch, r)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = loadTransactionSignaturesV2(batch, r)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	r.Signatures.Total = uint64(len(r.Signatures.Records))

	return r, nil
}

func loadTransactionSignaturesV1(batch *database.Batch, r *api.MessageRecord[messaging.Message]) error {
	txn := batch.Transaction2(r.ID.Hash())
	status, err := txn.Status().Get()
	if err != nil && !errors.Code(err).IsClientError() {
		return errors.UnknownError.WithFormat("load status: %w", err)
	}

	if r.Sequence == nil {
		r.Sequence = new(messaging.SequencedMessage)
		r.Sequence.Source = status.SourceNetwork
		r.Sequence.Destination = status.DestinationNetwork
		r.Sequence.Number = status.SequenceNumber
	}
	if r.SourceReceipt == nil {
		r.SourceReceipt = status.Proof
	}

	// Add a pretend credit payment
	if status.Initiator != nil || len(status.AnchorSigners) > 0 {
		set := new(api.SignatureSetRecord)
		set.Account = &protocol.UnknownAccount{Url: r.ID.Account()}
		set.Signatures = new(api.RecordRange[*api.MessageRecord[messaging.Message]])
		r.Signatures.Records = append(r.Signatures.Records, set)

		if status.Initiator != nil {
			set.Signatures.Records = append(set.Signatures.Records, &api.MessageRecord[messaging.Message]{
				Message: &messaging.CreditPayment{
					Payer:     status.Initiator,
					Initiator: true,
				},
			})
		}

		for _, signer := range status.AnchorSigners {
			set.Signatures.Records = append(set.Signatures.Records, &api.MessageRecord[messaging.Message]{
				Message: &messaging.BlockAnchor{
					Signature: &protocol.ED25519Signature{
						PublicKey: signer,
					},
				},
			})
		}

		set.Signatures.Total = uint64(len(set.Signatures.Records))
	}

	for _, signer := range status.Signers {
		entries, err := txn.ReadSignaturesForSigner(signer)
		if err != nil && !errors.Code(err).IsClientError() {
			return errors.UnknownError.WithFormat("load %v signatures: %w", signer.GetUrl(), err)
		}

		messages, err := api.MakeRange(entries.Entries(), 0, 0, func(e database.SigSetEntry) (*api.MessageRecord[messaging.Message], error) {
			r := new(api.MessageRecord[messaging.Message])

			msg, err := batch.Message2(e.SignatureHash[:]).Main().Get()
			if err != nil {
				slog.Info("Failed to load signature", "error", err, "account", signer.GetUrl(), "hash", logging.AsHex(e.SignatureHash))
				r.Error = errors.UnknownError.WithFormat("load signature %x: %w", e.SignatureHash[:], err)
				r.Status = r.Error.Code
				return r, nil
			}

			r.ID = msg.ID()
			r.Message = msg
			return r, nil
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		set := new(api.SignatureSetRecord)
		set.Account = signer
		set.Signatures = messages
		r.Signatures.Records = append(r.Signatures.Records, set)
	}

	return nil
}

func loadTransactionSignaturesV2(batch *database.Batch, r *api.MessageRecord[messaging.Message]) error {
	signers, err := batch.Message(r.ID.Hash()).Signers().Get()
	if err != nil && !errors.Code(err).IsClientError() {
		return errors.UnknownError.WithFormat("load signers: %w", err)
	}

	// Add the principal to the signers (but copy the slice to avoid weird bugs)
	{
		s := make([]*url.URL, len(signers), len(signers)+1)
		copy(s, signers)
		ptr, _ := sortutil.BinaryInsert(&s, func(u *url.URL) int { return u.Compare(r.ID.Account()) })
		*ptr = r.ID.Account()
		signers = s
	}

	for _, s := range signers {
		// Mark which messages are active
		txn := batch.Account(s).Transaction(r.ID.Hash())
		active := map[[32]byte]bool{}
		for _, v := range tryLoad(&err, txn.Votes().Get, "%v votes", s) {
			active[v.Hash] = true
		}
		for _, h := range tryLoad(&err, txn.Payments().Get, "%v payments", s) {
			active[h] = true
		}
		for _, s := range tryLoad(&err, txn.Signatures().Get, "%v signatures", s) {
			active[s.Hash] = true
		}

		history := tryLoad(&err, txn.History().Get, "%s history", s)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Load messages
		messages, err := api.MakeRange(history, 0, 0, func(i uint64) (*api.MessageRecord[messaging.Message], error) {
			r := new(api.MessageRecord[messaging.Message])

			hash, err := batch.Account(s).SignatureChain().Entry(int64(i))
			if err != nil {
				slog.Info("Failed to load signature chain entry", "error", err, "account", s, "index", i)
				r.Error = errors.UnknownError.WithFormat("load %v signature chain entry %d: %w", s, i, err)
				r.Status = r.Error.Code
				return r, nil
			}

			msg, err := batch.Message2(hash).Main().Get()
			if err != nil {
				slog.Info("Failed to load signature", "error", err, "account", s, "index", i, "hash", logging.AsHex(hash))
				r.Error = errors.UnknownError.WithFormat("load signature %x: %w", hash, err)
				r.Status = r.Error.Code
				return r, nil
			}

			r.ID = msg.ID()
			r.Message = msg
			r.Historical = !active[*(*[32]byte)(hash)] && msg.Type() != messaging.MessageTypeBlockAnchor
			return r, nil
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		if messages.Total == 0 {
			continue
		}

		account, err := batch.Account(s).Main().Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			account = &protocol.UnknownAccount{Url: s}
		default:
			return errors.UnknownError.WithFormat("load %v: %w", s, err)
		}

		// Build the signature set
		set := new(api.SignatureSetRecord)
		set.Account = account
		set.Signatures = messages
		r.Signatures.Records = append(r.Signatures.Records, set)
	}

	r.Signatures.Total = uint64(len(r.Signatures.Records))
	return nil
}

func loadBlockEntry(batch *database.Batch, entry *protocol.BlockEntry) (*api.ChainEntryRecord[api.Record], error) {
	r := new(api.ChainEntryRecord[api.Record])
	r.Account = entry.Account
	r.Name = entry.Chain
	r.Index = entry.Index

	chain, err := batch.Account(entry.Account).ChainByName(entry.Chain)
	if err != nil {
		return r, errors.UnknownError.WithFormat("load %s chain: %w", entry.Chain, err)
	}
	r.Type = chain.Type()

	value, err := chain.Entry(int64(entry.Index))
	if err != nil {
		slog.Info("Failed to load chain entry", "error", err, "account", entry.Account, "chain", entry.Chain, "index", entry.Index)
		r.Value = &api.ErrorRecord{
			Value: errors.UnknownError.WithFormat("load %s chain entry %d: %w", entry.Chain, entry.Index, err),
		}
		return r, nil
	}
	r.Entry = *(*[32]byte)(value)

	switch chain.Type() {
	case merkle.ChainTypeIndex:
		v := new(protocol.IndexEntry)
		if v.UnmarshalBinary(value) == nil {
			r.Value = &api.IndexEntryRecord{Value: v}
		}

	case merkle.ChainTypeTransaction:
		r.Value, err = loadMessage(batch, protocol.UnknownUrl().WithTxID(r.Entry))
		if err != nil {
			slog.Info("Failed to load message", "error", err, "account", entry.Account, "chain", entry.Chain, "index", entry.Index, "hash", logging.AsHex(r.Entry))
			r.Value = &api.ErrorRecord{
				Value: errors.UnknownError.WithFormat("load %s chain entry %d transaction: %w", entry.Chain, entry.Index, err),
			}
		}
	}

	return r, nil
}

func getAccountAuthoritySet(batch *database.Batch, account *url.URL) (*protocol.AccountAuth, error) {
	state, err := batch.Account(account).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load account: %w", err)
	}

	auth, url, err := shared.GetAccountAuthoritySet(state)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if auth != nil {
		return auth, nil
	}

	return getAccountAuthoritySet(batch, url)
}

func normalizeBlockEntries(batch *database.Batch, ledger *protocol.BlockLedger) ([]*protocol.BlockEntry, error) { //nolint:unused
	// For each account+chain, find the entry with the largest index. This is
	// necessary because some protocol versions add every chain entry to the
	// ledger and others only add the last.
	type AccountAndChain struct {
		Account [32]byte
		Chain   string
	}
	end := map[AccountAndChain]*protocol.BlockEntry{}
	for _, e := range ledger.Entries {
		k := AccountAndChain{e.Account.AccountID32(), e.Chain}
		if d := end[k]; d == nil || d.Index < e.Index {
			end[k] = e
		}
	}

	// For each account+chain, find the previously anchored chain height and
	// construct a BlockEntry for every new chain entry
	//
	// TODO This really needs to be indexed
	var entries []*protocol.BlockEntry
	for _, e := range end {
		chain, err := batch.Account(e.Account).ChainByName(e.Chain)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %v %s chain: %w", e.Account, e.Chain, err)
		}
		indexChain, err := chain.Index().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %v %s index chain: %w", e.Account, e.Chain, err)
		}

		var start uint64
		i, _, err := indexing.SearchIndexChain(indexChain, 0, indexing.MatchExact, indexing.SearchIndexChainBySource(e.Index))
		switch {
		case err == nil:
			if i == 0 {
				break
			}

			entry := new(protocol.IndexEntry)
			err = indexChain.EntryAs(int64(i)-1, entry)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load %v %s index chain entry %d: %w", e.Account, e.Chain, i-1, err)
			}

			start = entry.Source + 1

		case errors.Is(err, errors.NotFound):
			start = e.Index

		case errors.Is(err, errors.InvalidRecord):
			// Skip invalid records
			continue

		default:
			return nil, errors.UnknownError.WithFormat("find %v %s index chain entry for block %d: %w", e.Account, e.Chain, ledger.Index, err)
		}

		for j := start; j <= e.Index; j++ {
			entries = append(entries, &protocol.BlockEntry{
				Account: e.Account,
				Chain:   e.Chain,
				Index:   j,
			})
		}
	}

	return entries, nil
}

func (s *Querier) loadAnchoredBlocks(ctx context.Context, batch *database.Batch, anchorIndex uint64) (*api.RecordRange[*api.MinorBlockRecord], error) {
	// From this point on we're making a best effort to return a list of
	// anchored blocks. However if something fails, we're going to ignore it and
	// return what we have regardless.

	indexChain, err := batch.Account(s.partition.AnchorPool()).MainChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor ledger index chain: %w", err)
	}
	if indexChain.Height() == 0 {
		return nil, nil // Bad, but ignore
	}

	i, _, err := indexing.SearchIndexChain(indexChain, uint64(indexChain.Height()-1), indexing.MatchExact, indexing.SearchIndexChainBySource(anchorIndex))
	if err != nil {
		return nil, nil //nolint // Bad, but ignore
	}

	// If this is the first time the anchor chain has been anchored, start is 0.
	// Otherwise, find the previous index entry and start at its position +1.
	var start uint64
	end := anchorIndex
	if i > 0 {
		entry := new(protocol.IndexEntry)
		err := indexChain.EntryAs(int64(i-1), entry)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor ledger index chain entry %d: %w", i-1, err)
		}
		start = entry.Source + 1
	}

	// Start and end are inclusive. Start 0 and end 3 means get four entries
	// starting at 0.
	count := end - start + 1

	expand := true
	anchors, err := s.queryChainEntryRange(ctx, batch, batch.Account(s.partition.AnchorPool()).MainChain(), &api.RangeOptions{Start: start, Count: &count, Expand: &expand})
	if err != nil {
		return nil, nil //nolint // Bad, but ignore
	}

	blocks := new(api.RecordRange[*api.MinorBlockRecord])
	blocks.Total = count
	blocks.Records = make([]*api.MinorBlockRecord, 0, len(anchors.Records))
	for _, rec := range anchors.Records {
		txn, err := api.ChainEntryRecordAsMessage[*messaging.TransactionMessage](rec)
		if err != nil {
			return nil, nil //nolint // Bad, but ignore
		}

		anchorBody, ok := txn.Value.Message.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			continue // Bad, but ignore
		}

		anchor := anchorBody.GetPartitionAnchor()
		b := new(api.MinorBlockRecord)
		b.Index = anchor.MinorBlockIndex
		b.Source = anchor.Source
		blocks.Records = append(blocks.Records, b)
	}

	return blocks, nil
}

func tryLoad[T any](errp *error, fn func() (T, error), format string, args ...any) (z T) {
	if *errp != nil {
		return
	}

	v, err := fn()
	switch {
	case err == nil:
		return v
	case errors.Code(err).IsClientError():
		return z
	default:
		*errp = errors.UnknownError.WithFormat("load "+format+": %w", append(args, err)...)
		return z
	}
}
