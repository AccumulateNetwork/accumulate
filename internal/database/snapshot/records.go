// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// TODO: Check for existing records when restoring?

func CollectSignature(batch *database.Batch, hash [32]byte) (*Signature, error) {
	var msg messaging.MessageWithSignature
	err := batch.Message(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	sig := new(Signature)
	sig.Signature = msg.GetSignature()
	sig.Txid = msg.GetTxID()
	return sig, nil
}

func (s *Signature) Restore(header *Header, batch *database.Batch) error {
	var err error
	if header.ExecutorVersion.V2() {
		err = batch.Message2(s.Signature.Hash()).Main().Put(&messaging.UserSignature{
			Signature: s.Signature,
			TxID:      s.Txid,
		})
	} else {
		err = batch.Transaction(s.Signature.Hash()).Main().Put(&database.SigOrTxn{
			Signature: s.Signature,
			Txid:      s.Txid,
		})
	}
	return errors.UnknownError.Wrap(err)
}

func CollectTransaction(batch *database.Batch, hash [32]byte) (*Transaction, error) {
	var msg messaging.MessageWithTransaction
	err := batch.Message(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	txn := new(Transaction)
	txn.Transaction = msg.GetTransaction()
	txn.Status = loadState(&err, true, batch.Transaction(hash[:]).Status().Get)

	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	for _, signer := range txn.Status.Signers {
		record, err := batch.Transaction(hash[:]).ReadSignaturesForSigner(signer)
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

func (t *Transaction) Restore(header *Header, batch *database.Batch) error {
	var err error
	if header.ExecutorVersion.V2() {
		saveState[messaging.Message](&err, batch.Message(t.Transaction.ID().Hash()).Main().Put, &messaging.UserTransaction{Transaction: t.Transaction})
	} else {
		saveState(&err, batch.Transaction(t.Transaction.GetHash()).Main().Put, &database.SigOrTxn{Transaction: t.Transaction})
	}
	saveState(&err, batch.Transaction(t.Transaction.GetHash()).Status().Put, t.Status)

	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for _, set := range t.SignatureSets {
		err = batch.Transaction(t.Transaction.GetHash()).RestoreSignatureSets(set.Signer, set.Version, set.Entries)
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

		chain, err := CollectChain(record.Inner())
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
		c := new(Chain)
		c.Name = oc.Name
		c.Type = oc.Type
		c.MarkPower = uint64(markPower)
		c.Head = new(database.MerkleState)
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

func (a *Account) RestoreChainHead(batch *database.Batch, c *Chain) (*database.Chain2, error) {
	mgr, err := batch.Account(a.Url).ChainByName(c.Name)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get %s chain: %w", c.Name, err)
	}
	_, err = mgr.Get() // Update index
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get %s chain head: %w", c.Name, err)
	}
	err = c.RestoreHead(mgr.Inner())
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
