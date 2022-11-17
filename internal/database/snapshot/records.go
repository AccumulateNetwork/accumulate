// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// TODO: Check for existing records when restoring?

func CollectSignature(record *database.Transaction) (*Signature, error) {
	state, err := record.Main().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if state.Signature == nil || state.Transaction != nil {
		return nil, errors.BadRequest.WithFormat("signature is not a signature")
	}

	sig := new(Signature)
	sig.Signature = state.Signature
	sig.Txid = state.Txid
	return sig, nil
}

func (s *Signature) Restore(batch *database.Batch) error {
	err := batch.Transaction(s.Signature.Hash()).Main().Put(&database.SigOrTxn{Signature: s.Signature, Txid: s.Txid})
	return errors.UnknownError.Wrap(err)
}

func CollectTransaction(record *database.Transaction) (*Transaction, error) {
	state, err := record.Main().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if state.Transaction == nil || state.Signature != nil {
		return nil, errors.BadRequest.WithFormat("transaction is not a transaction")
	}

	txn := new(Transaction)
	txn.Transaction = state.Transaction
	txn.Status = loadState(&err, true, record.Status().Get)

	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	for _, signer := range txn.Status.Signers {
		record, err := record.ReadSignaturesForSigner(signer)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %v signature set: %w", signer.GetUrl(), err)
		}

		set := new(TxnSigSet)
		set.Signer = signer.GetUrl()
		set.Version = record.Version()
		set.Entries = record.Entries()
		txn.SignatureSets = append(txn.SignatureSets, set)
	}

	return txn, nil
}

func (t *Transaction) Restore(batch *database.Batch) error {
	var err error
	record := batch.Transaction(t.Transaction.GetHash())
	saveState(&err, record.Main().Put, &database.SigOrTxn{Transaction: t.Transaction})
	saveState(&err, record.Status().Put, t.Status)

	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for _, set := range t.SignatureSets {
		err = record.RestoreSignatureSets(set.Signer, set.Version, set.Entries)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return nil
}

func CollectAccount(record *database.Account, fullChainHistory bool) (*Account, error) {
	var err error
	acct := new(Account)
	acct.Url = record.Url()
	acct.Main = loadState(&err, true, record.Main().Get)
	acct.Pending = loadState(&err, true, record.Pending().Get)
	acct.Directory = loadState(&err, true, record.Directory().Get)

	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	for _, meta := range loadState(&err, false, record.Chains().Get) {
		record, err := record.ChainByName(meta.Name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %s chain state: %w", meta.Name, err)
		}

		chain, err := record.Inner().CollectSnapshot()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect %s chain snapshot: %w", meta.Name, err)
		}

		if !fullChainHistory {
			chain.MarkPoints = nil
		}

		acct.Chains = append(acct.Chains, chain)
	}

	return acct, nil
}

// ConvertOldChains converts OldChains to Chains.
func (a *Account) ConvertOldChains(markPower int64) {
	if len(a.OldChains) == 0 || len(a.Chains) > 0 {
		return
	}

	for _, oc := range a.OldChains {
		c := new(managed.Snapshot)
		c.Name = oc.Name
		c.Type = oc.Type
		c.MarkPower = uint64(markPower)
		c.Head = new(managed.MerkleState)
		c.Head.Count = int64(oc.Count)
		c.Head.Pending = oc.Pending

		for _, e := range oc.Entries {
			c.AddEntry(e)
		}

		a.Chains = append(a.Chains, c)
	}

	a.OldChains = nil
}

func (a *Account) Restore(batch *database.Batch) error {
	var err error
	record := batch.Account(a.Url)
	saveState(&err, record.Main().Put, a.Main)
	saveState(&err, record.Directory().Put, a.Directory)
	saveState(&err, record.Pending().Put, a.Pending)

	return errors.UnknownError.Wrap(err)
}

func (a *Account) RestoreChainHead(batch *database.Batch, c *managed.Snapshot) (*database.Chain2, error) {
	mgr, err := batch.Account(a.Url).ChainByName(c.Name)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get %s chain: %w", c.Name, err)
	}
	_, err = mgr.Get() // Update index
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get %s chain head: %w", c.Name, err)
	}
	err = mgr.Inner().RestoreHead(c)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore %s chain: %w", c.Name, err)
	}
	return mgr, nil
}

func zero[T any]() (z T) { return z }

func loadState[T any](lastErr *error, allowMissing bool, get func() (T, error)) T {
	if *lastErr != nil {
		return zero[T]()
	}

	v, err := get()
	if allowMissing && errors.Is(err, errors.NotFound) {
		return zero[T]()
	}
	if err != nil {
		*lastErr = err
		return zero[T]()
	}

	return v
}

func saveState[T any](lastErr *error, put func(T) error, v T) {
	if *lastErr != nil || any(v) == nil {
		return
	}

	err := put(v)
	if err != nil {
		*lastErr = err
	}
}
