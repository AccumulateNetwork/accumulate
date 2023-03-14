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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func loadTransactionOrSignature(batch *database.Batch, txid *url.TxID) (api.Record, error) {
	msg, err := batch.Message(txid.Hash()).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state: %w", err)
	}
	for {
		switch m := msg.(type) {
		case messaging.MessageWithTransaction:
			return loadTransaction(batch, m.GetTransaction())
		case messaging.MessageWithSignature:
			return loadSignature(batch, m.GetSignature(), m.GetTxID())
		case interface{ Unwrap() messaging.Message }:
			msg = m.Unwrap()
		default:
			return loadOtherMessage(batch, msg)
		}
	}
}

func loadOtherMessage(batch *database.Batch, msg messaging.Message) (*api.TransactionRecord, error) {
	// TODO Improve the API for messages

	h := msg.Hash()
	record := batch.Transaction(h[:])
	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	produced, err := record.Produced().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load produced: %w", err)
	}

	r := new(api.TransactionRecord)
	r.TxID = msg.ID()
	r.Message = msg
	r.Status = status
	r.Produced, _ = api.MakeRange(produced, 0, 0, func(x *url.TxID) (*api.TxIDRecord, error) {
		return &api.TxIDRecord{Value: x}, nil
	})
	return r, nil
}

func loadTransaction(batch *database.Batch, txn *protocol.Transaction) (*api.TransactionRecord, error) {
	record := batch.Transaction(txn.GetHash())
	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// Copy to avoid weirdness
	signers := make([]protocol.Signer, len(status.Signers))
	copy(signers, status.Signers)

	s2, err := batch.Message2(txn.GetHash()).Signers().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load signers: %w", err)
	}
	for _, u := range s2 {
		signers = append(signers, &protocol.UnknownSigner{Url: u})
	}

	produced, err := record.Produced().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load produced: %w", err)
	}

	r := new(api.TransactionRecord)
	r.TxID = txn.ID()
	r.Transaction = txn
	r.Status = status
	r.Produced, _ = api.MakeRange(produced, 0, 0, func(x *url.TxID) (*api.TxIDRecord, error) {
		return &api.TxIDRecord{Value: x}, nil
	})
	r.Signatures, err = api.MakeRange(signers, 0, 0, func(s protocol.Signer) (*api.SignatureRecord, error) {
		// If something can't be loaded (not found), ignore the error since what
		// the user is asking for is the transaction, not the signature(s)

		sig := new(protocol.SignatureSet)
		sig.Signer = s.GetUrl()
		sig.TransactionHash = *(*[32]byte)(txn.GetHash())
		sig.Vote = protocol.VoteTypeAccept

		if _, err := protocol.ParseLiteAddress(s.GetUrl()); err == nil {
			sig.Authority = s.GetUrl().RootIdentity()
		} else if u, _, ok := protocol.ParseKeyPageUrl(s.GetUrl()); ok {
			sig.Authority = u
		} else {
			sig.Authority = &url.URL{Authority: protocol.Unknown}
		}

		set, err := record.SignaturesForSigner(s)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			return loadSignature(batch, sig, txn.ID())
		default:
			return nil, errors.UnknownError.WithFormat("load %s signature set: %w", s.GetUrl(), err)
		}

		for _, e := range set.Entries() {
			var msg messaging.MessageWithSignature
			err = batch.Message(e.SignatureHash).Main().GetAs(&msg)
			switch {
			case err == nil:
				sig.Signatures = append(sig.Signatures, msg.GetSignature())
			case errors.Is(err, errors.NotFound):
				continue
			default:
				return nil, errors.UnknownError.WithFormat("load signature %x: %w", e.SignatureHash[:4], err)
			}
		}

		return loadSignature(batch, sig, txn.ID())
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return r, nil
}

func loadSignature(batch *database.Batch, sig protocol.Signature, txid *url.TxID) (*api.SignatureRecord, error) {
	record := batch.Transaction(sig.Hash())
	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	produced, err := record.Produced().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load produced: %w", err)
	}

	r := new(api.SignatureRecord)
	r.Signature = sig
	r.TxID = txid
	r.Status = status
	r.Produced, _ = api.MakeRange(produced, 0, 0, func(x *url.TxID) (*api.TxIDRecord, error) {
		return &api.TxIDRecord{Value: x}, nil
	})

	if sig.Type().IsSystem() {
		return r, nil
	}
	if !sig.RoutingLocation().LocalTo(sig.GetSigner()) {
		return r, nil
	}

	var signer protocol.Signer
	err = batch.Account(sig.GetSigner()).Main().GetAs(&signer)
	switch {
	case err == nil:
		r.Signer = signer
	case errors.Is(err, errors.NotFound),
		errors.Is(err, errors.WrongType):
		r.Signer = &protocol.UnknownSigner{Url: sig.GetSigner()}
	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	r.Status, err = batch.Transaction(sig.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	return r, nil
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
		return r, errors.UnknownError.WithFormat("load %s chain entry %d: %w", entry.Chain, entry.Index, err)
	}
	r.Entry = *(*[32]byte)(value)

	switch chain.Type() {
	case merkle.ChainTypeIndex:
		v := new(protocol.IndexEntry)
		if v.UnmarshalBinary(value) == nil {
			r.Value = &api.IndexEntryRecord{Value: v}
		}

	case merkle.ChainTypeTransaction:
		r.Value, err = loadTransactionOrSignature(batch, protocol.UnknownUrl().WithTxID(r.Entry))
		if err != nil {
			return r, errors.UnknownError.WithFormat("load %s chain entry %d transaction: %w", entry.Chain, entry.Index, err)
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

func normalizeBlockEntries(batch *database.Batch, ledger *protocol.BlockLedger) ([]*protocol.BlockEntry, error) {
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
		txn, ok := rec.Value.(*api.TransactionRecord)
		if !ok {
			continue // Bad, but ignore
		}

		anchorBody, ok := txn.Transaction.Body.(protocol.AnchorBody)
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
