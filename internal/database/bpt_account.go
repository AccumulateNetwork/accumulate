package database

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// state returns the fully realized account state.
func (a *Account) state(fullTxnState, preserveChains bool) (*accountState, error) {
	s := new(accountState)

	// Load metadata
	obj, err := a.GetObject()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("load metadata: %w", err)
	}
	if obj == nil {
		obj = new(protocol.Object)
	}

	// Load main state
	s.Main, err = a.GetState()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("load main state: %w", err)
	}
	// Load chain state
	s.Chains = make([]*merkleState, len(obj.Chains))
	for i, c := range obj.Chains {
		chain, err := a.ReadChain(c.Name)
		if err != nil {
			return nil, fmt.Errorf("load %s chain state: %w", c.Name, err)
		}

		ms1 := chain.CurrentState()
		ms2 := new(merkleState)
		s.Chains[i] = ms2
		ms2.Name = c.Name
		ms2.Type = c.Type

		if preserveChains {
			ms2.Entries, err = chain.Entries(0, ms1.Count)
			if err != nil {
				return nil, fmt.Errorf("load %s chain entries: %w", c.Name, err)
			}
		} else {
			ms2.Count = uint64(ms1.Count)
			ms2.Pending = make([][]byte, len(ms1.Pending))
			for i, v := range ms1.Pending {
				if len(v) == 0 {
					continue
				}
				ms2.Pending[i] = v
			}
		}
	}

	// Load transaction state
	for i, h := range obj.Pending.Entries {
		h := h.Hash()
		state, err := a.batch.Transaction(h[:]).state(fullTxnState) //nolint:rangevarref
		if err != nil {
			return nil, err
		}
		s.Pending[i] = state
	}

	return s, nil
}

func (a *Account) stateOfTransactionsOnChain(name string) ([]*transactionState, error) {
	chain, err := a.ReadChain(name)
	if err != nil {
		return nil, fmt.Errorf("load main chain: %w", err)
	}

	// TODO We need to be more selective than this
	state := make([]*transactionState, chain.Height())
	for i := range state {
		hash, err := chain.Entry(int64(i))
		if err != nil {
			return nil, fmt.Errorf("load %s chain entry %d: %w", name, i, err)
		}

		state[i], err = a.batch.Transaction(hash).state(true)
		if err != nil {
			return nil, err
		}
	}

	return state, nil
}

func (a *Account) restore(s *accountState) error {
	// Store main state
	if s.Main != nil {
		err := a.PutState(s.Main)
		if err != nil {
			return fmt.Errorf("store main state: %w", err)
		}
	}

	// Store metadata
	metadata := new(protocol.Object)
	metadata.Type = protocol.ObjectTypeAccount
	metadata.Chains = make([]protocol.ChainMetadata, 0, len(s.Chains))
	metadata.Pending.Entries = make([]*url.TxID, len(s.Pending))

	for _, c := range s.Chains {
		err := metadata.AddChain(c.Name, c.Type)
		if err != nil {
			return fmt.Errorf("add %s chain: %w", c.Name, err)
		}
	}

	for _, p := range s.Pending {
		metadata.Pending.Add(p.Transaction.ID())
	}

	a.batch.putValue(a.key.Object(), metadata)

	// Store chain state
	mgr := new(managed.MerkleManager)
	mgr.Manager = a.batch.store
	mgr.MarkPower = markPower
	mgr.MarkFreq = int64(math.Pow(2, float64(markPower)))
	mgr.MarkMask = mgr.MarkFreq - 1
	for _, c := range s.Chains {
		mgr.Key = a.key.Chain(c.Name)
		mgr.MS = new(managed.MerkleState)
		mgr.MS.InitSha256()
		mgr.MS.Count = int64(c.Count)
		mgr.MS.Pending = make(managed.SparseHashList, len(c.Pending))
		for i, v := range c.Pending {
			if len(v) > 0 {
				mgr.MS.Pending[i] = v
			}
		}
		err := mgr.WriteChainHead(mgr.Key)
		if err != nil {
			return fmt.Errorf("store %s chain state: %w", c.Name, err)
		}
		for i, entry := range c.Entries {
			err := mgr.AddHash(entry, false)
			if err != nil {
				return fmt.Errorf("store %s chain entry %d: %w", c.Name, c.Count+uint64(i), err)
			}
		}
	}

	// Store transaction state
	txns := make([]*transactionState, 0, len(s.Pending)+len(s.Transactions))
	txns = append(txns, s.Pending...)
	txns = append(txns, s.Transactions...)
	for _, p := range txns {
		hash := p.Transaction.GetHash()
		if len(p.State.Signers) != len(p.Signatures) {
			return fmt.Errorf("transaction %X state is invalid: %d signers and %d signatures", hash[:4], len(p.State.Signers), len(p.Signatures))
		}

		record := a.batch.Transaction(hash)
		err := record.PutState(&SigOrTxn{Transaction: p.Transaction})
		if err != nil {
			return fmt.Errorf("store transaction %X: %w", hash[:4], err)
		}

		err = record.PutStatus(p.State)
		if err != nil {
			return fmt.Errorf("store transaction %X status: %w", hash[:4], err)
		}

		for i, set := range p.Signatures {
			signer := p.State.Signers[i].GetUrl()
			a.batch.putValue(record.key.Signatures(signer), set)
		}
	}

	return nil
}

// MerkleHash calculates the Merkle DAG root hash.
func (s *merkleState) MerkleHash() []byte {
	cascade := make([][]byte, len(s.Pending))
	copy(cascade, s.Pending)
	cascade = hash.MerkleCascade(cascade, s.Entries, -1)

	var right []byte
	for _, left := range cascade {
		if right == nil {
			left := left // See docs/developer/rangevarref.md
			right = left[:]
		} else {
			right = hash.Combine(left[:], right)
		}
	}
	return right
}

// chainMerkleHash returns a Merkle hash of the chain states.
func (s *accountState) chainMerkleHash() []byte {
	hasher := make(hash.Hasher, 0, len(s.Chains))
	for _, c := range s.Chains {
		hasher.AddValue(c)
	}
	return hasher.MerkleHash()
}

// pendingMerkleHash returns a Merkle hash of the pending transaction states.
func (s *accountState) pendingMerkleHash() ([]byte, error) {
	hasher := make(hash.Hasher, 0, len(s.Pending))
	for _, p := range s.Pending {
		state := p.State.Copy()
		state.For = *(*[32]byte)(p.Transaction.GetHash())
		data, err := p.State.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("marshal pending transaction %X status: %w", p.State.For[:4], err)
		}
		hasher.AddBytes(data)
	}
	return hasher.MerkleHash(), nil
}

// Hasher returns a hasher populated with hashes of the account state.
func (s *accountState) Hasher() (hash.Hasher, error) {
	hasher := make(hash.Hasher, 0, 1+len(s.Chains)+len(s.Pending))

	// State hash
	if s.Main == nil {
		hasher.AddHash(new([32]byte))
	} else {
		data, err := s.Main.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("marshal main: %w", err)
		}
		hasher.AddBytes(data)
	}

	// Chains hash
	hasher.AddHash((*[32]byte)(s.chainMerkleHash()))

	// Pending transactions hash
	hash, err := s.pendingMerkleHash()
	if err != nil {
		return nil, err
	}
	hasher.AddHash((*[32]byte)(hash))

	return hasher, nil
}

// PutBpt writes the record's BPT entry.
func (a *Account) putBpt() error {
	state, err := a.state(false, false)
	if err != nil {
		return err
	}

	hasher, err := state.Hasher()
	if err != nil {
		return err
	}

	adi := sha256.Sum256([]byte(a.url.Authority))

	hash := *(*[32]byte)(hasher.MerkleHash())
	a.batch.putBpt(adi, a.key.Object(), hash)
	return nil
}

// StateReceipt returns a Merkle receipt for the account state in the BPT.
func (a *Account) StateReceipt() (*managed.Receipt, error) {
	state, err := a.state(false, false)
	if err != nil {
		return nil, err
	}

	hasher, err := state.Hasher()
	if err != nil {
		return nil, err
	}

	rBPT, err := a.batch.BptReceipt(a.key.Object(), *(*[32]byte)(hasher.MerkleHash()))
	if err != nil {
		return nil, err
	}

	rState := hasher.Receipt(0, len(hasher)-1)
	if !bytes.Equal(rState.MDRoot, rBPT.Element) {
		return nil, errors.New("bpt entry does not match account state")
	}

	receipt, err := rState.Combine(rBPT)
	if err != nil {
		return nil, fmt.Errorf("combine receipt: %w", err)
	}

	return receipt, nil
}
