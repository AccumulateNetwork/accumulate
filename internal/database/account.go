package database

import (
	"fmt"
	"sort"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func (r *Account) url() *url.URL {
	return r.key[1].(*url.URL)
}

func (a *Account) Commit() error {
	if !a.IsDirty() {
		return nil
	}

	// Ensure the synthetic anchors index is up to date
	for anchor, set := range a.syntheticForAnchor {
		if !set.IsDirty() {
			continue
		}

		err := a.SyntheticAnchors().Add(anchor)
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// Ensure the chains index is up to date
	for _, c := range a.chains2 {
		if !c.IsDirty() {
			continue
		}

		err := a.Chains().Add(c.Name())
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// If anything has changed, update the BPT entry
	err := a.putBpt()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Do the normal commit stuff
	err = a.baseCommit()
	return errors.Wrap(errors.StatusUnknownError, err)
}

// GetState loads the record state.
func (r *Account) GetState() (protocol.Account, error) {
	return r.Main().Get()
}

// GetStateAs loads the record state and unmarshals into the given value. In
// most cases `state` should be a double pointer.
func (r *Account) GetStateAs(state interface{}) error {
	return r.Main().GetAs(state)
}

// PutState stores the record state.
func (r *Account) PutState(state protocol.Account) error {
	// Does the record state have a URL?
	if state.GetUrl() == nil {
		return errors.New(errors.StatusInternalError, "invalid URL: empty")
	}

	// Is this the right URL - does it match the record's key?
	if !r.url().Equal(state.GetUrl()) {
		return fmt.Errorf("mismatched url: key is %v, URL is %v", r.url(), state.GetUrl())
	}

	// Make sure the key book is set
	account, ok := state.(protocol.FullAccount)
	if ok && len(account.GetAuth().Authorities) == 0 {
		return fmt.Errorf("missing key book")
	}

	// Store the state
	err := r.Main().Put(state)
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (r *Account) GetPending() (*protocol.TxIdSet, error) {
	v, err := r.Pending().Get()
	if err != nil {
		return nil, err
	}
	return &protocol.TxIdSet{Entries: v}, nil
}

func (r *Account) AddPending(txid *url.TxID) error {
	return r.Pending().Add(txid)
}

func (r *Account) RemovePending(txid *url.TxID) error {
	return r.Pending().Remove(txid)
}

func (r *Account) AllChains() ([]*managed.Chain, error) {
	chains := r.dirtyChains()
	names := map[string]bool{}
	for _, c := range chains {
		names[c.Name()] = true
	}

	index, err := r.Chains().Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	for _, name := range index {
		if names[name] {
			continue
		}
		names[name] = true
		chain, err := r.ChainByName(name)
		if err != nil {
			return nil, errors.Wrap(errors.StatusInternalError, err)
		}
		chains = append(chains, chain)
	}

	for i, n := 0, len(chains); i < n; i++ {
		chain := chains[i].Index()
		head, err := chain.Head().Get()
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
		if head.Count > 0 {
			chains = append(chains, chain)
		}
	}

	sort.Slice(chains, func(i, j int) bool {
		return strings.Compare(chains[i].Name(), chains[j].Name()) < 0
	})
	return chains, nil
}

// Chain returns a chain manager for the given chain.
func (r *Account) Chain(name string) (*Chain, error) {
	return r.wrappedChain(name)
}

// ReadChain returns a read-only chain manager for the given chain.
func (r *Account) ReadChain(name string) (*Chain, error) {
	return r.wrappedChain(name)
}

func (r *Account) wrappedChain(name string) (*Chain, error) {
	c, err := r.ChainByName(name)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return WrapChain(c)
}

// Chain returns a chain manager for the given chain.
func (r *Account) IndexChain(name string) (*Chain, error) {
	// Prevent double-indexing
	if strings.HasSuffix(name, "-index") {
		name = name[:len(name)-len("-index")]
	}

	c, err := r.ChainByName(name)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return WrapChain(c.Index())
}

func (a *Account) ChainByName(name string) (*managed.Chain, error) {
	index := strings.HasSuffix(name, "-index")
	if index {
		name = name[:len(name)-len("-index")]
	}
	c, ok := a.resolveChain(name)
	if !ok {
		return nil, errors.NotFound("account %v: invalid chain name: %q", a.key[1], name)
	}
	if index {
		return c.Index(), nil
	}
	return c, nil
}

func (r *Account) AddSyntheticForAnchor(anchor [32]byte, txid *url.TxID) error {
	return r.SyntheticForAnchor(anchor).Add(txid)
}

func (r *Account) GetSyntheticForAnchor(anchor [32]byte) ([]*url.TxID, error) {
	return r.SyntheticForAnchor(anchor).Get()
}

func tryResolveChainParam(chainPtr **managed.Chain, okPtr *bool, name, prefix string, expectLen int, resolve func([]string, string) (*managed.Chain, bool)) {
	if *okPtr || !strings.HasPrefix(name, prefix) {
		return
	}

	name = name[len(prefix):]
	i := strings.Index(name, ")")
	if i < 0 {
		return
	}

	params := strings.Split(name[:i], ",")
	name = name[i+1:]
	if len(params) != expectLen {
		return
	}

	if strings.HasPrefix(name, "-") {
		name = name[1:]
	}
	chain, ok := resolve(params, name)
	if ok {
		*chainPtr, *okPtr = chain, true
	}
}

func parseChainParam[T any](ok *bool, s string, parse func(string) (T, error)) T {
	if !*ok {
		return zero[T]()
	}

	v, err := parse(s)
	if err == nil {
		return v
	}

	*ok = false
	return zero[T]()
}
