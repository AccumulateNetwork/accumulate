// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"sync"
)

// ValidatorInfo represents a validator in the committee.
type ValidatorInfo struct {
	// PublicKey is the ed25519 public key of the validator.
	PublicKey ed25519.PublicKey
	// Stake is the validator's stake weight.
	Stake uint64
}

// Committee represents the set of validators for a given epoch.
// It provides stake-weighted quorum calculations.
type Committee struct {
	// Validators is the ordered list of validators.
	Validators []ValidatorInfo
	// Epoch is the epoch number this committee is valid for.
	Epoch uint64

	// mu protects cached values.
	mu sync.RWMutex
	// totalStake is the cached total stake.
	totalStake uint64
	// totalStakeComputed indicates whether totalStake has been computed.
	totalStakeComputed bool
	// indexByKey maps public key bytes to validator index.
	indexByKey map[string]uint16
}

// NewCommittee creates a new Committee from a list of validators.
func NewCommittee(validators []ValidatorInfo, epoch uint64) *Committee {
	return &Committee{
		Validators: validators,
		Epoch:      epoch,
	}
}

// TotalStake returns the sum of all validator stakes.
func (c *Committee) TotalStake() uint64 {
	c.mu.RLock()
	if c.totalStakeComputed {
		total := c.totalStake
		c.mu.RUnlock()
		return total
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.totalStakeComputed {
		return c.totalStake
	}

	var total uint64
	for _, v := range c.Validators {
		total += v.Stake
	}
	c.totalStake = total
	c.totalStakeComputed = true
	return total
}

// QuorumThreshold returns the minimum stake required for a quorum (2f+1).
// For n validators with total stake S, this is ceil(2*S/3) + 1.
// In BFT consensus, f = floor((n-1)/3), so 2f+1 ensures safety.
func (c *Committee) QuorumThreshold() uint64 {
	total := c.TotalStake()
	// 2f+1 where f < n/3, so quorum needs > 2/3 of stake
	// We use (2*total)/3 + 1 to get > 2/3
	return (2*total)/3 + 1
}

// ValidityThreshold returns the minimum stake for validity (f+1).
// This ensures at least one honest validator is included.
func (c *Committee) ValidityThreshold() uint64 {
	total := c.TotalStake()
	// f+1 where f < n/3, so we need > 1/3 of stake
	return total/3 + 1
}

// GetValidator returns the validator at the given index.
// Returns nil if the index is out of bounds.
func (c *Committee) GetValidator(index uint16) *ValidatorInfo {
	if int(index) >= len(c.Validators) {
		return nil
	}
	return &c.Validators[index]
}

// FindValidator returns the index of a validator by public key.
// Returns the index and true if found, or 0 and false if not found.
func (c *Committee) FindValidator(key ed25519.PublicKey) (uint16, bool) {
	c.mu.RLock()
	if c.indexByKey != nil {
		keyStr := string(key)
		idx, ok := c.indexByKey[keyStr]
		c.mu.RUnlock()
		return idx, ok
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Build index if not already built
	if c.indexByKey == nil {
		c.indexByKey = make(map[string]uint16, len(c.Validators))
		for i, v := range c.Validators {
			c.indexByKey[string(v.PublicKey)] = uint16(i)
		}
	}

	keyStr := string(key)
	idx, ok := c.indexByKey[keyStr]
	return idx, ok
}

// HasQuorum returns true if the given stake meets the quorum threshold.
func (c *Committee) HasQuorum(stake uint64) bool {
	return stake >= c.QuorumThreshold()
}

// HasValidity returns true if the given stake meets the validity threshold.
func (c *Committee) HasValidity(stake uint64) bool {
	return stake >= c.ValidityThreshold()
}

// Len returns the number of validators in the committee.
func (c *Committee) Len() int {
	return len(c.Validators)
}

// StakeOf returns the stake of the validator with the given public key.
// Returns 0 if the validator is not found.
func (c *Committee) StakeOf(key ed25519.PublicKey) uint64 {
	idx, ok := c.FindValidator(key)
	if !ok {
		return 0
	}
	return c.Validators[idx].Stake
}

// Clone creates a deep copy of the committee.
func (c *Committee) Clone() *Committee {
	validators := make([]ValidatorInfo, len(c.Validators))
	for i, v := range c.Validators {
		pubKey := make([]byte, len(v.PublicKey))
		copy(pubKey, v.PublicKey)
		validators[i] = ValidatorInfo{
			PublicKey: pubKey,
			Stake:     v.Stake,
		}
	}
	return &Committee{
		Validators: validators,
		Epoch:      c.Epoch,
	}
}

// Validate checks that the committee is well-formed.
func (c *Committee) Validate() error {
	if len(c.Validators) == 0 {
		return errors.New("committee has no validators")
	}

	seen := make(map[string]bool)
	for i, v := range c.Validators {
		if len(v.PublicKey) != ed25519.PublicKeySize {
			return errors.New("validator has invalid public key size")
		}
		if v.Stake == 0 {
			return errors.New("validator has zero stake")
		}
		keyStr := string(v.PublicKey)
		if seen[keyStr] {
			return errors.New("duplicate validator public key")
		}
		seen[keyStr] = true
		_ = i
	}

	return nil
}

// ContainsValidator returns true if the committee contains a validator with the given public key.
func (c *Committee) ContainsValidator(key ed25519.PublicKey) bool {
	_, ok := c.FindValidator(key)
	return ok
}

// QuorumCount returns the minimum number of validators required for a quorum.
// QuorumCount returns the minimum number of validators needed for quorum.
// This assumes equal stake for all validators and returns 2f+1 where f = floor((n-1)/3).
// For stake-weighted voting, use HasQuorum instead.
func (c *Committee) QuorumCount() int {
	n := len(c.Validators)
	if n == 0 {
		return 0
	}
	// BFT requires n = 3f+1, so f = (n-1)/3
	// Quorum = 2f+1
	f := (n - 1) / 3
	return 2*f + 1
}

// ValidatorsEqual returns true if two public keys are equal.
func ValidatorsEqual(a, b ed25519.PublicKey) bool {
	return bytes.Equal(a, b)
}
