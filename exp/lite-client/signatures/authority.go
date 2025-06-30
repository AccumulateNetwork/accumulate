package signatures

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AuthoritySet represents the set of keys and threshold required to validate a block.
// This is a data structure only; extraction helpers may use blocks package utilities.
type AuthoritySet struct {
	Keys      [][]byte
	Threshold uint64
}

// NewAuthoritySetFromAccount extracts an AuthoritySet from a KeyPage or KeyBook account.
// May use lower-level helpers from blocks package.
func NewAuthoritySetFromAccount(account interface{}) (*AuthoritySet, error) {
	switch acc := account.(type) {
	case *protocol.KeyPage:
		keys := make([][]byte, len(acc.Keys))
		for i, keySpec := range acc.Keys {
			keys[i] = keySpec.PublicKeyHash
		}
		return &AuthoritySet{Keys: keys, Threshold: acc.AcceptThreshold}, nil
	case *protocol.KeyBook:
		// Handle KeyBook case if needed
		return nil, fmt.Errorf("KeyBook handling not implemented")
	case *protocol.AccountAuth:
		return nil, fmt.Errorf("resolving authorities from URLs is not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported account type")
	}
}

// AuthorityTracker maintains a history of authority changes across major blocks.
type AuthorityTracker struct {
	history map[uint64]*AuthoritySet
}

// NewAuthorityTracker initializes a new AuthorityTracker.
func NewAuthorityTracker(initial *AuthoritySet, startBlock uint64) *AuthorityTracker {
	return &AuthorityTracker{
		history: map[uint64]*AuthoritySet{startBlock: initial},
	}
}

// GetAuthorityAt returns the AuthoritySet at a given block index.
func (t *AuthorityTracker) GetAuthorityAt(blockIndex uint64) (*AuthoritySet, error) {
	if auth, ok := t.history[blockIndex]; ok {
		return auth, nil
	}
	return nil, errors.New("authority set not found for block")
}

// RecordChange records an authority change at a specific block index.
func (t *AuthorityTracker) RecordChange(blockIndex uint64, newAuth *AuthoritySet) {
	t.history[blockIndex] = newAuth
}

// DetectAndApplyChanges scans block for authority/keybook/keypage changes and records them if found.
func (t *AuthorityTracker) DetectAndApplyChanges(block *blocks.MajorBlockRecord) error {
	newAuth, err := blocks.ExtractAuthorityChange(block)
	if err != nil {
		return err
	}
	if newAuth != nil {
		t.RecordChange(block.Index, newAuth)
	}
	return nil
}
