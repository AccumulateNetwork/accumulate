package liteclient

import "gitlab.com/accumulatenetwork/accumulate/protocol"

// UpdateWithBlock applies authority changes from a new block to the tracker.
func (t *AuthorityTracker) UpdateWithBlock(block interface{}) error {
	// TODO: Parse KeyBook/KeyPage updates from the block
	return nil
}

// GetEffectiveAuthority returns the authority set (KeyPage) active at a given block height.
func (t *AuthorityTracker) GetEffectiveAuthority(height int64) (*protocol.KeyPage, error) {
	// TODO: Return the KeyPage that was valid at this height
	return nil, nil
}
