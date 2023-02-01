// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func loadTransactionOrSignature(batch *database.Batch, record *database.Transaction) (api.Record, error) {
	state, err := record.Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state: %w", err)
	}
	switch {
	case state.Transaction != nil:
		return loadTransaction(batch, record, state.Transaction)
	case state.Signature != nil:
		return loadSignature(batch, state.Signature, state.Txid)
	default:
		return nil, errors.InternalError.WithFormat("invalid transaction state")
	}
}

func loadTransaction(batch *database.Batch, record *database.Transaction, txn *protocol.Transaction) (*api.TransactionRecord, error) {
	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
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
	r.Signatures, err = api.MakeRange(status.Signers, 0, 0, func(s protocol.Signer) (*api.SignatureRecord, error) {
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
			s, err := batch.Transaction(e.SignatureHash[:]).Main().Get()
			switch {
			case err == nil:
				if s.Signature != nil {
					sig.Signatures = append(sig.Signatures, s.Signature)
				}
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
		r.Value, err = loadTransactionOrSignature(batch, batch.Transaction(value))
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
