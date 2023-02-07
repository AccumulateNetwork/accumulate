// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"compress/gzip"
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CollectOptions struct {
	Logger           log.Logger
	VisitAccount     func(acct *Account) error
	VisitTransaction func(txn *Transaction) error
	VisitSignature   func(sig *Signature) error

	// PreserveAccountHistory is called for each account to test if the
	// account's transaction history should be preserved. If
	// PreserveAccountHistory is unspecified, all account history is preserved.
	PreserveAccountHistory func(account *database.Account) (bool, error)
}

func Collect(batch *database.Batch, header *Header, file io.WriteSeeker, opts CollectOptions) (*Writer, error) {
	header.RootHash = *(*[32]byte)(batch.BptRoot())

	w, err := Create(file, header)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	w.Logger.Set(opts.Logger)

	// Restoring accounts will fail if they reference transactions that have not
	// yet been restored, so the transaction section must come first. However we
	// need to scan the BPT in order to know what transactions need to be saved.
	var accounts []*url.URL
	txnHashes := new(HashSet)
	err = batch.VisitAccounts(func(record *database.Account) error {
		accounts = append(accounts, record.Url())
		pending, err := record.Pending().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load pending: %w", err)
		}
		for _, txid := range pending {
			txnHashes.Add(txid.Hash())
		}
		return nil
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Collect transaction and signature hashes
	sigHashes := new(HashSet)
	for _, u := range accounts {
		record := batch.Account(u)
		if opts.PreserveAccountHistory != nil {
			preserve, err := opts.PreserveAccountHistory(record)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			if !preserve {
				continue
			}
		}

		err = txnHashes.CollectFromChain(record, record.MainChain())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect from %v main chain: %v", u, err)
		}

		err = txnHashes.CollectFromChain(record, record.ScratchChain())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect from %v scratch chain: %v", u, err)
		}

		err = sigHashes.CollectFromChain(record, record.SignatureChain())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect from %v signature chain: %v", u, err)
		}
	}

	visitTx := opts.VisitTransaction
	opts.VisitTransaction = func(t *Transaction) error {
		for _, set := range t.SignatureSets {
			for _, entry := range set.Entries {
				sigHashes.Add(entry.SignatureHash)
			}
		}
		if visitTx != nil {
			return visitTx(t)
		}
		return nil
	}

	// Save transactions
	err = w.CollectTransactions(batch, txnHashes.Hashes, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Save signatures
	err = w.CollectSignatures(batch, sigHashes.Hashes, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Save accounts
	err = w.CollectAccounts(batch, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return w, nil
}

func (w *Writer) CollectAccounts(batch *database.Batch, opts CollectOptions) error {
	sw, err := w.Open(SectionTypeAccounts)
	if err != nil {
		return errors.UnknownError.WithFormat("open accounts section: %w", err)
	}

	err = batch.SaveAccounts(sw, func(record *database.Account) ([]byte, error) {
		// preserve, err := preserveHistory(record)
		// if err != nil {
		// 	return nil, errors.UnknownError.Wrap(err)
		// }

		// Preserve chain state regardless of whether we preserve transactions
		acct, err := CollectAccount(record, true)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect account: %w", err)
		}

		if opts.VisitAccount != nil {
			err = opts.VisitAccount(acct)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("visit account: %w", err)
			}
		}

		b, err := acct.MarshalBinary()
		if err != nil {
			return nil, errors.EncodingError.WithFormat("marshal account: %w", err)
		}
		return b, nil
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = sw.Close()
	return errors.UnknownError.Wrap(err)
}

func (w *Writer) CollectTransactions(batch *database.Batch, hashes [][32]byte, opts CollectOptions) error {
	var txns []*Transaction
	for _, h := range hashes {
		h := h // See docs/developer/rangevarref.md
		txn, err := CollectTransaction(batch, h)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				w.Logger.Info("Skipping transaction", "error", err, "hash", logging.AsHex(h).Slice(0, 4))
				continue
			}
			return errors.UnknownError.WithFormat("collect transaction %x: %w", h[:4], err)
		}

		if opts.VisitTransaction != nil {
			err = opts.VisitTransaction(txn)
			if err != nil {
				return errors.UnknownError.WithFormat("visit transaction: %w", err)
			}
		}

		txns = append(txns, txn)
	}

	return w.WriteTransactions(txns, false)
}

func (w *Writer) WriteTransactions(txns []*Transaction, gz bool) error {
	if len(txns) == 0 {
		return nil
	}

	b, err := (&txnSection{Transactions: txns}).MarshalBinary()
	if err != nil {
		return errors.UnknownError.WithFormat("marshal transactions: %w", err)
	}

	typ := SectionTypeTransactions
	if gz {
		typ = SectionTypeGzTransactions
	}
	sw, err := w.Open(typ)
	if err != nil {
		return errors.UnknownError.WithFormat("open transactions section: %w", err)
	}

	if gz {
		w := gzip.NewWriter(sw)
		_, err = w.Write(b)
		if err != nil {
			return errors.UnknownError.WithFormat("write transactions: %w", err)
		}
		err = w.Close()
		if err != nil {
			return errors.UnknownError.WithFormat("close gzip writer: %w", err)
		}
	} else {
		_, err = sw.Write(b)
		if err != nil {
			return errors.UnknownError.WithFormat("write transactions: %w", err)
		}
	}

	err = sw.Close()
	return errors.UnknownError.Wrap(err)
}

func (w *Writer) CollectSignatures(batch *database.Batch, hashes [][32]byte, opts CollectOptions) error {
	var sigs []*Signature
	for _, h := range hashes {
		h := h // See docs/developer/rangevarref.md
		sig, err := CollectSignature(batch, h)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				w.Logger.Error("Skipping signature", "error", err, "hash", logging.AsHex(h).Slice(0, 4))
				continue
			}
			return errors.UnknownError.WithFormat("collect signature %x: %w", h[:4], err)
		}

		if opts.VisitSignature != nil {
			err = opts.VisitSignature(sig)
			if err != nil {
				return errors.UnknownError.WithFormat("visit signature: %w", err)
			}
		}

		sigs = append(sigs, sig)
	}

	if len(sigs) == 0 {
		return nil
	}

	b, err := (&sigSection{Signatures: sigs}).MarshalBinary()
	if err != nil {
		return errors.UnknownError.WithFormat("marshal signatures: %w", err)
	}

	sw, err := w.Open(SectionTypeSignatures)
	if err != nil {
		return errors.UnknownError.WithFormat("open signatures section: %w", err)
	}

	_, err = sw.Write(b)
	if err != nil {
		return errors.UnknownError.WithFormat("write signatures: %w", err)
	}

	err = sw.Close()
	return errors.UnknownError.Wrap(err)
}

type HashSet struct {
	seen   map[[32]byte]bool
	Hashes [][32]byte
}

func (s *HashSet) Add(h [32]byte) {
	if s.seen == nil {
		s.seen = map[[32]byte]bool{}
	}
	if s.seen[h] {
		return
	}
	s.seen[h] = true
	s.Hashes = append(s.Hashes, h)
}

func (s *HashSet) CollectFromChain(a *database.Account, c *database.Chain2) error {
	// Calling c.Get() modifies the account, so don't do that if the chain
	// doesn't exist
	_, err := a.Chains().Find(&protocol.ChainMetadata{Name: c.Name(), Type: c.Type()})
	switch {
	case err == nil:
		// Found
	case errors.Is(err, errors.NotFound):
		return nil
	default:
		return errors.UnknownError.WithFormat("load chains index: %w", err)
	}

	snap, err := CollectChain(c.Inner())
	if err != nil {
		return errors.UnknownError.WithFormat("collect %s chain: %w", c.Name(), err)
	}

	for _, h := range snap.Head.HashList {
		if h == nil {
			continue
		}
		s.Add(*(*[32]byte)(h))
	}
	for _, mp := range snap.MarkPoints {
		if mp == nil {
			continue
		}
		for _, h := range mp.HashList {
			if h == nil {
				continue
			}
			s.Add(*(*[32]byte)(h))
		}
	}

	return nil
}
