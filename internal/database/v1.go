package database

import (
	"encoding"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

type ctype = protocol.ChainType

func (a *Account) GetState() (protocol.Account, error)          { return a.State().Get() }
func (a *Account) GetStateAs(v interface{}) error               { return a.State().GetAs(v) }
func (a *Account) PutState(v protocol.Account) error            { return a.State().Put(v) }
func (a *Account) AddPending(txid *url.TxID) error              { return a.Pending().Add(txid) }
func (a *Account) RemovePending(txid *url.TxID) error           { return a.Pending().Remove(txid) }
func (a *Account) Chain(name string, _ ctype) (*ChainV1, error) { return a.getChainV1(name, nil) }
func (a *Account) ReadChain(name string) (*ChainV1, error)      { return a.getChainV1(name, nil) }

func (a *Account) GetObject() (*protocol.Object, error) {
	var err error
	o := new(protocol.Object)
	o.Type = protocol.ObjectTypeAccount
	o.Chains = loadState(&err, true, a.Chains().Get)
	o.Pending.Entries = loadState(&err, true, a.Pending().Get)
	return o, err
}

// func (a *Account) GetPending() (*protocol.TxIdSet, error) {
// 	p, err := a.Pending().Get()
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &protocol.TxIdSet{Entries: p}, nil
// }

func (a *Account) IndexChain(name string, major bool) (*ChainV1, error) {
	return a.getChainV1(name, &major)
}

func (a *Account) ReadIndexChain(name string, major bool) (*ChainV1, error) {
	return a.getChainV1(name, &major)
}

func (a *Account) AddSyntheticForAnchor(anchor [32]byte, txid *url.TxID) error {
	return a.SyntheticForAnchor(anchor).Add(txid)
}

// func (a *Account) GetSyntheticForAnchor(anchor [32]byte) ([]*url.TxID, error) {
// 	return a.SyntheticForAnchor(anchor).Get()
// }

func (t *Transaction) GetStatus() (*protocol.TransactionStatus, error) { return t.Status().Get() }
func (t *Transaction) PutStatus(v *protocol.TransactionStatus) error   { return t.Status().Put(v) }
func (t *Transaction) PutSyntheticTxns(v *protocol.TxIdSet) error      { return t.Produced().Put(v.Entries) }
func (t *Transaction) AddSyntheticTxns(txids ...*url.TxID) error       { return t.Produced().Add(txids...) }

// func (t *Transaction) Signatures(signer *url.URL) (*SignatureSet, error)
// func (t *Transaction) AddSignature(keyEntryIndex uint64, newSignature protocol.Signature) (int, error)

func (t *Transaction) GetState() (*SigOrTxn, error) {
	var err error
	r := new(SigOrTxn)
	r.Transaction = loadState(&err, true, t.Value().Get)
	r.Signature = loadState(&err, true, t.Signature().Get)
	r.Txid = loadState(&err, true, t.TxID().Get)
	if err != nil {
		return nil, err
	}
	if r.Transaction == nil && r.Signature == nil && r.Txid == nil {
		return nil, errors.NotFound("transaction %x not found", t.key[1])
	}
	return r, nil
}

func (t *Transaction) PutState(v *SigOrTxn) error {
	var err error
	if v.Transaction != nil {
		saveState(&err, t.Value().Put, v.Transaction)
	}
	if v.Signature != nil {
		saveState(&err, t.Signature().Put, v.Signature)
		saveState(&err, t.TxID().Put, v.Txid)
	}
	return err
}

func (t *Transaction) ReadSignatures(signer *url.URL) (SignatureSet, error) {
	return t.Signatures(signer), nil
}

func (t *Transaction) SignaturesForSigner(signer protocol.Signer) (SignatureSet, error) {
	return t.Signatures(signer.GetUrl()), nil
}

func (t *Transaction) ReadSignaturesForSigner(signer protocol.Signer) (SignatureSet, error) {
	return t.Signatures(signer.GetUrl()), nil
}

func (t *Transaction) AddSystemSignature(net *config.Describe, newSignature protocol.Signature) error {
	return t.SystemSignatures().Add(newSignature)
}

func (t *Transaction) GetSyntheticTxns() (*protocol.TxIdSet, error) {
	p, err := t.Produced().Get()
	return &protocol.TxIdSet{Entries: p}, err
}

type ChainV1 struct {
	*managed.Chain
	state *managed.MerkleState
}

func (a *Account) getChainV1(name string, major *bool) (*ChainV1, error) {
	if major != nil {
		name = protocol.IndexChain(name, *major)
	}

	c, err := a.ChainByName(name)
	if err != nil {
		return nil, err
	}

	s, err := c.Head().Get()
	if err != nil {
		return nil, err
	}

	return &ChainV1{c, s}, nil
}

func (c *ChainV1) Height() int64 {
	return c.state.Count
}

func (c *ChainV1) Entry(height int64) ([]byte, error) {
	entry, err := c.Element(uint64(height)).Get()
	return entry, err
}

func (c *ChainV1) EntryAs(height int64, value encoding.BinaryUnmarshaler) error {
	data, err := c.Entry(height)
	if err != nil {
		return err
	}

	return value.UnmarshalBinary(data)
}

func (c *ChainV1) CurrentState() *managed.MerkleState {
	return c.state
}

func (c *ChainV1) HeightOf(hash []byte) (int64, error) {
	v, err := c.ElementIndex(hash).Get()
	return int64(v), err
}

func (c *ChainV1) AnchorAt(height uint64) ([]byte, error) {
	ms, err := c.States(height).Get()
	if err != nil {
		return nil, err
	}
	return ms.GetMDRoot(), nil
}

func (c *ChainV1) Pending() []managed.Hash {
	return c.state.Pending
}

func (c *ChainV1) Anchor() []byte {
	return c.state.GetMDRoot()
}

func (c *ChainV1) State(height int64) (*managed.MerkleState, error) {
	return c.GetAnyState(height)
}

func (d *AccountData) Put(entryHash, txnHash []byte) error {
	err := d.Entry().Put(*(*[32]byte)(entryHash))
	if err != nil {
		return err
	}
	err = d.Transaction(*(*[32]byte)(entryHash)).Put(*(*[32]byte)(txnHash))
	if err != nil {
		return err
	}
	return nil
}

func GetDataEntry(batch *ChangeSet, txnHash []byte) (protocol.DataEntry, error) {
	state, err := batch.Transaction(txnHash).GetState()
	if err != nil {
		return nil, err
	}

	switch txn := state.Transaction.Body.(type) {
	case *protocol.WriteData:
		return txn.Entry, nil
	case *protocol.WriteDataTo:
		return txn.Entry, nil
	case *protocol.SyntheticWriteData:
		return txn.Entry, nil
	case *protocol.SystemWriteData:
		return txn.Entry, nil
	default:
		return nil, errors.Format(errors.StatusInternalError, "invalid data transaction: expected %v or %v, got %v", protocol.TransactionTypeWriteData, protocol.TransactionTypeWriteDataTo, state.Transaction.Body.Type())
	}
}

func (d *AccountData) GetLatest() (index uint64, entryHash, txnHash [32]byte, err error) {
	count, err := d.Entry().Count()
	if err != nil {
		return 0, entryHash, txnHash, err
	}

	if count == 0 {
		return 0, entryHash, txnHash, errors.NotFound("empty")
	}

	entryHash, err = d.Entry().Get(int(count - 1))
	if err != nil {
		return 0, entryHash, txnHash, err
	}

	txnHash, err = d.Transaction(entryHash).Get()
	if err != nil {
		return 0, entryHash, txnHash, err
	}

	return uint64(count - 1), entryHash, txnHash, nil
}

func (d *AccountData) GetLatestEntry() (protocol.DataEntry, error) {
	_, _, txnHash, err := d.GetLatest()
	if err != nil {
		return nil, err
	}

	return GetDataEntry(d.container.container, txnHash[:])
}
