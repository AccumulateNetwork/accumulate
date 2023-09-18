// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Chain2 is a wrapper for Chain.
type Chain2 struct {
	account *Account
	key     *record.Key
	inner   *MerkleManager
	index   *Chain2
}

func newChain2(parent record.Record, _ log.Logger, _ record.Store, key *record.Key, namefmt string) *Chain2 {
	var account *Account
	switch parent := parent.(type) {
	case *Account:
		account = parent
	case *AccountAnchorChain:
		account = parent.parent
	default:
		panic("unknown chain parent") // Will be removed once chains are completely integrated into the model
	}

	var typ merkle.ChainType
	switch key.Get(2).(string) {
	case "MainChain",
		"SignatureChain",
		"ScratchChain",
		"AnchorSequenceChain",
		"SyntheticSequenceChain":
		typ = merkle.ChainTypeTransaction
	case "RootChain",
		"AnchorChain":
		typ = merkle.ChainTypeAnchor
	case "MajorBlockChain":
		typ = merkle.ChainTypeIndex
	default:
		panic("unknown chain key") // Will be removed once chains are completely integrated into the model
	}

	c := NewChain(account.parent.logger.L, account.parent.store, key, markPower, typ, namefmt)
	return &Chain2{account, key, c, nil}
}

func (c *Chain2) Key() *record.Key { return c.key }

func (c *Chain2) dirtyChains() []*MerkleManager {
	if c == nil {
		return nil
	}
	chains := c.index.dirtyChains()
	if c.inner.IsDirty() {
		chains = append(chains, c.inner)
	}
	return chains
}

// UpdatedChains returns a block entry for every chain updated in the current
// database batch.
func (a *Account) UpdatedChains() ([]*protocol.BlockEntry, error) {
	var entries []*protocol.BlockEntry

	// Add an entry for each modified chain
	for _, c := range a.dirtyChains() {
		head, err := c.Head().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get %s chain head: %w", c.Name(), err)
		}

		entries = append(entries, &protocol.BlockEntry{
			Account: a.Url(),
			Chain:   c.Name(),
			Index:   uint64(head.Count - 1),
		})
	}
	return entries, nil
}

// Account returns the URL of the account.
func (c *Chain2) Account() *url.URL { return c.key.Get(1).(*url.URL) }

// Name returns the name of the chain.
func (c *Chain2) Name() string { return c.inner.Name() }

// Type returns the type of the chain.
func (c *Chain2) Type() merkle.ChainType { return c.inner.Type() }

func (c *Chain2) Inner() *MerkleManager { return c.inner }

// Url returns the URL of the chain: {account}#chain/{name}.
func (c *Chain2) Url() *url.URL {
	return c.Account().WithFragment("chain/" + c.Name())
}

func (c *Chain2) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() > 0 && key.Get(0) == "Index" {
		return c.Index(), key.SliceI(1), nil
	}
	return c.inner.Resolve(key)
}

func (c *Chain2) Walk(opts database.WalkOptions, fn database.WalkFunc) error {
	var err error
	values.Walk(&err, c.inner, opts, fn)
	if c.inner.typ != merkle.ChainTypeIndex {
		values.WalkField(&err, c.index, c.newIndex, opts, fn)
	}
	return err
}

func (c *Chain2) IsDirty() bool {
	return values.IsDirty(c.index) || values.IsDirty(c.inner)
}

func (c *Chain2) Commit() error {
	var err error
	values.Commit(&err, c.index)
	values.Commit(&err, c.inner)
	return err
}

func (c *Chain2) Head() values.Value[*merkle.State] {
	return c.inner.Head()
}

// IndexOf returns the index of the given entry in the chain.
func (c *Chain2) IndexOf(hash []byte) (int64, error) {
	return c.inner.GetElementIndex(hash)
}

// Entry loads the entry in the chain at the given height.
func (c *Chain2) Entry(height int64) ([]byte, error) {
	return c.inner.Get(height)
}

// Get converts the Chain2 to a Chain, updating the account's chains index and
// loading the chain head.
func (c *Chain2) Get() (*Chain, error) {
	index := c.account.Chains()
	_, err := index.Index(&protocol.ChainMetadata{Name: c.Name()})
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, errors.NotFound):
		return nil, errors.UnknownError.Wrap(err)
	default:
		err = c.account.Chains().Add(&protocol.ChainMetadata{Name: c.Name(), Type: c.Type()})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}
	return wrapChain(c.inner)
}

// Index returns the index chain of this chain. Index will panic if called on an
// index chain.
func (c *Chain2) Index() *Chain2 {
	return values.GetOrCreate(c, &c.index, (*Chain2).newIndex)
}

func (c *Chain2) newIndex() *Chain2 {
	if c.Type() == merkle.ChainTypeIndex {
		panic("cannot index an index chain")
	}
	key := c.key.Append("Index")
	m := NewChain(c.account.logger.L, c.account.store, key, markPower, merkle.ChainTypeIndex, c.Name()+"-index")
	return &Chain2{c.account, key, m, nil}
}

// ChainByName returns account Chain2 for the named chain, or a not found error if
// there is no such chain.
func (a *Account) ChainByName(name string) (*Chain2, error) {
	name = strings.ToLower(name)

	index := strings.HasSuffix(name, "-index")
	if index {
		name = name[:len(name)-len("-index")]
	}

	c := a.chainByName(name)
	if c == nil {
		return nil, errors.NotFound.WithFormat("account %v chain %s not found", a.Url(), name)
	}

	if index {
		c = c.Index()
	}
	return c, nil
}

// GetChainByName calls ChainByName and Get.
func (a *Account) GetChainByName(name string) (*Chain, error) {
	c, err := a.ChainByName(name)
	if err != nil {
		return nil, err
	}
	return c.Get()
}

// GetChainByName calls ChainByName, Index, and Get.
func (a *Account) GetIndexChainByName(name string) (*Chain, error) {
	c, err := a.ChainByName(name)
	if err != nil {
		return nil, err
	}
	return c.Index().Get()
}

func (a *Account) chainByName(name string) *Chain2 {
	switch name {
	case "main":
		return a.MainChain()
	case "signature":
		return a.SignatureChain()
	case "scratch":
		return a.ScratchChain()
	case "root":
		return a.RootChain()
	case "anchor-sequence":
		return a.AnchorSequenceChain()
	case "major-block":
		return a.MajorBlockChain()
	}

	first, arg, rest, ok := splitChainName(name)
	if !ok {
		return nil
	}

	switch first {
	case "anchor":
		a := a.AnchorChain(arg)
		switch rest {
		case "-root":
			return a.Root()
		case "-bpt":
			return a.BPT()
		}

	case "synthetic-sequence":
		return a.SyntheticSequenceChain(arg)
	}

	return nil
}

func splitChainName(name string) (first, arg, rest string, ok bool) {
	i := strings.IndexRune(name, '(')
	j := strings.IndexRune(name, ')')
	if i < 0 || j < 0 {
		return "", "", "", false
	}

	return name[:i], name[i+1 : j], name[j+1:], true
}

func (c *Account) SyntheticSequenceChain(partition string) *Chain2 {
	return c.getSyntheticSequenceChain(strings.ToLower(partition))
}

func (c *Account) AnchorChain(partition string) *AccountAnchorChain {
	return c.getAnchorChain(strings.ToLower(partition))
}

func (c *Account) getSyntheticSequenceKeys() ([]accountSyntheticSequenceChainKey, error) {
	// List all of the account's chains
	chains, err := c.Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Find chains matching the pattern `synthetic-sequence(:id)`
	keys := make([]accountSyntheticSequenceChainKey, 0, len(chains))
	seen := map[string]bool{}
	for _, c := range chains {
		first, arg, _, ok := splitChainName(strings.ToLower(c.Name))
		if !ok || first != "synthetic-sequence" || seen[arg] {
			continue
		}
		seen[arg] = true
		keys = append(keys, accountSyntheticSequenceChainKey{arg})
	}

	// Return the partition IDs
	return keys, nil
}

func (c *Account) getAnchorKeys() ([]accountAnchorChainKey, error) {
	// List all of the account's chains
	chains, err := c.Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Find chains matching the pattern `anchor(:id)`
	keys := make([]accountAnchorChainKey, 0, len(chains))
	for _, c := range chains {
		first, arg, _, ok := splitChainName(strings.ToLower(c.Name))
		if !ok || first != "anchor" {
			continue
		}
		keys = append(keys, accountAnchorChainKey{arg})
	}

	// Return the partition IDs
	return keys, nil
}
