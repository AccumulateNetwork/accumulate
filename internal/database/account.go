package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (c *ChangeSet) accountByKey(key [32]byte) (*Account, error) {
	if a, ok := c.account[key]; ok {
		return a, nil
	}

	w := newWrapped(c.store, recordKey{key}.Append("State"), "account %[2]v state", false, newUnion(protocol.UnmarshalAccount))
	state, err := w.Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return c.Account(state.GetUrl()), nil
}

func (a *Account) ChainByName(name string) (*Chain, error) {
	c, ok := a.resolveChain(name)
	if ok {
		return c, nil
	}
	return nil, errors.NotFound("account %v: invalid chain name: %q", a.key[1], name)
}

func (a *Account) commit() error {
	// Ensure the chains index is up to date
	for _, c := range a.dirtyChains() {
		chain := &protocol.ChainMetadata{Name: c.name, Type: c.typ}
		other, err := a.Chains().Find(chain)
		switch {
		case err == nil:
			if other.Type != c.typ {
				return errors.Format(errors.StatusInternalError, "chain %s: attempted to change type from %v to %v", c.name, other.Type, c.typ)
			}
		case !errors.Is(err, errors.StatusNotFound):
			return errors.Wrap(errors.StatusUnknown, err)
		}

		err = a.Chains().Add(chain)
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	// Ensure the synthetic anchors index is up to date
	for anchor, set := range a.syntheticForAnchor {
		if !set.isDirty() {
			continue
		}

		err := a.SyntheticAnchors().Add(anchor)
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	// Do the normal commit stuff
	err := a.baseCommit()
	return errors.Wrap(errors.StatusUnknown, err)
}
