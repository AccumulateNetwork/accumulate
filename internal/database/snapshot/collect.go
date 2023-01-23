// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
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

func Collect(db database.Beginner, header *Header, file io.WriteSeeker, opts CollectOptions) (*Writer, error) {
	_ = db.View(func(batch *database.Batch) error {
		header.RootHash = *(*[32]byte)(batch.BptRoot())
		return nil
	})

	w, err := Create(file, header)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	w.Logger.Set(opts.Logger)

	// Restoring accounts will fail if they reference transactions that have not
	// yet been restored, so the transaction section must come first. However we
	// need to scan the BPT in order to know what transactions need to be saved.
	var accounts []*url.URL
	var txnHashes HashSet
	err = db.View(func(batch *database.Batch) error {
		return batch.VisitAccounts(func(record *database.Account) error {
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
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Collect transaction and signature hashes
	var sigHashes HashSet
	for _, u := range accounts {
		err = db.View(func(batch *database.Batch) error {
			record := batch.Account(u)
			if opts.PreserveAccountHistory != nil {
				preserve, err := opts.PreserveAccountHistory(record)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				if !preserve {
					return nil
				}
			}

			err = txnHashes.CollectFromChain(record, record.MainChain())
			if err != nil {
				return errors.UnknownError.WithFormat("collect from %v main chain: %v", u, err)
			}

			err = txnHashes.CollectFromChain(record, record.ScratchChain())
			if err != nil {
				return errors.UnknownError.WithFormat("collect from %v scratch chain: %v", u, err)
			}

			err = sigHashes.CollectFromChain(record, record.SignatureChain())
			if err != nil {
				return errors.UnknownError.WithFormat("collect from %v signature chain: %v", u, err)
			}
			return nil
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
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
	err = w.CollectTransactions(db, txnHashes, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Save signatures
	err = w.CollectSignatures(db, sigHashes, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Save accounts
	err = w.CollectAccounts(db, opts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return w, nil
}

func (w *Writer) CollectAccounts(db database.Beginner, opts CollectOptions) error {
	sw, err := w.Open(SectionTypeAccounts)
	if err != nil {
		return errors.UnknownError.WithFormat("open accounts section: %w", err)
	}

	batch := db.Begin(false)
	defer batch.Discard()
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

func (w *Writer) CollectTransactions(db database.Beginner, hashes [][32]byte, opts CollectOptions) error {
	var txns []*Transaction
	for _, h := range hashes {
		h := h // See docs/developer/rangevarref.md
		err := db.View(func(batch *database.Batch) error {
			txn, err := CollectTransaction(batch.Transaction(h[:]))
			if err != nil {
				if errors.Is(err, errors.NotFound) {
					w.Logger.Debug("Skipping transaction", "error", err, "hash", logging.AsHex(h).Slice(0, 4))
					return nil
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
			return nil
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
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

func (w *Writer) CollectSignatures(db database.Beginner, hashes [][32]byte, opts CollectOptions) error {
	var sigs []*Signature
	for _, h := range hashes {
		h := h // See docs/developer/rangevarref.md
		err := db.View(func(batch *database.Batch) error {
			sig, err := CollectSignature(batch.Transaction(h[:]))
			if err != nil {
				if errors.Is(err, errors.NotFound) {
					w.Logger.Error("Skipping signature", "error", err, "hash", logging.AsHex(h).Slice(0, 4))
					return nil
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
			return nil
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
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

// HashSet is an ordered set of 32-byte hashes.
type HashSet [][32]byte

// Add inserts a hash into the set.
func (l *HashSet) Add(v [32]byte) {
	ptr, new := sortutil.BinaryInsert((*[][32]byte)(l), func(u [32]byte) int {
		return bytes.Compare(u[:], v[:])
	})
	if new {
		*ptr = v
	}
}

// Remove removes a hash from the set.
func (l *HashSet) Remove(v [32]byte) {
	sortutil.Remove((*[][32]byte)(l), func(u [32]byte) int {
		return bytes.Compare(u[:], v[:])
	})
}

// Hash checks if the set has the given hash.
func (l HashSet) Has(v [32]byte) bool {
	_, ok := sortutil.Search(([][32]byte)(l), func(u [32]byte) int {
		return bytes.Compare(u[:], v[:])
	})
	return ok
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
