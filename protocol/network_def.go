// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"crypto/sha256"
	"strings"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type valHashCmp []byte

func (k valHashCmp) cmp(v *ValidatorInfo) int {
	return bytes.Compare(v.PublicKeyHash[:], k)
}

func (n *NetworkDefinition) AddPartition(id string, typ PartitionType) bool {
	lowerId := strings.ToLower(id)
	ptr, new := sortutil.BinaryInsert(&n.Partitions, func(p *PartitionInfo) int { return strings.Compare(strings.ToLower(p.ID), lowerId) })
	if !new {
		return false
	}

	*ptr = &PartitionInfo{ID: id, Type: typ}
	return true
}

func (n *NetworkDefinition) ValidatorByHash(hash []byte) (int, *ValidatorInfo, bool) {
	i, found := sortutil.Search(n.Validators, valHashCmp(hash).cmp)
	if !found {
		return 0, nil, false
	}
	return i, n.Validators[i], true
}

func (n *NetworkDefinition) ValidatorByKey(key []byte) (int, *ValidatorInfo, bool) {
	hash := sha256.Sum256(key)
	return n.ValidatorByHash(hash[:])
}

// IsActiveOn returns true if the validator is active on the partition.
func (v *ValidatorInfo) IsActiveOn(partition string) bool {
	if v == nil {
		return false
	}
	for _, p := range v.Partitions {
		if strings.EqualFold(p.ID, partition) {
			return p.Active
		}
	}
	return false
}

// AddValidator adds or updates a validator and its corresponding partition
// entry.
func (n *NetworkDefinition) AddValidator(key []byte, partition string, active bool) {
	hash := sha256.Sum256(key)
	ptr, new := sortutil.BinaryInsert(&n.Validators, valHashCmp(hash[:]).cmp)
	if new {
		*ptr = &ValidatorInfo{PublicKey: key, PublicKeyHash: hash}
	}

	v := *ptr
	for _, p := range v.Partitions {
		if strings.EqualFold(p.ID, partition) {
			p.Active = active
			return
		}
	}

	v.Partitions = append(v.Partitions, &ValidatorPartitionInfo{ID: partition, Active: active})
}

// RemoveValidator completely removes a validator.
func (n *NetworkDefinition) RemoveValidator(key []byte) {
	hash := sha256.Sum256(key)
	i, found := sortutil.Search(n.Validators, valHashCmp(hash[:]).cmp)
	if !found {
		return
	}

	n.Validators = append(n.Validators[:i], n.Validators[i+1:]...)
}

// UpdateValidatorKey updates a validator's key.
func (n *NetworkDefinition) UpdateValidatorKey(oldKey, newKey []byte) error {
	oldHash := sha256.Sum256(oldKey)
	i, found := sortutil.Search(n.Validators, valHashCmp(oldHash[:]).cmp)
	if !found {
		return errors.NotFound.WithFormat("validator %x not found", oldKey[:4])
	}

	newHash := sha256.Sum256(newKey)
	v := n.Validators[i]
	v.PublicKey = newKey
	v.PublicKeyHash = newHash

	val := make([]*ValidatorInfo, len(n.Validators)-1)
	copy(val, n.Validators[:i])
	copy(val[i:], n.Validators[i+1:])

	ptr, new := sortutil.BinaryInsert(&val, valHashCmp(newHash[:]).cmp)
	if !new {
		return errors.Conflict.WithFormat("validator %x already exists", newKey[:4])
	}
	*ptr = v
	n.Validators = val
	return nil
}

// MarshalTOML marshals the partition type to Toml as a string.
func (v PartitionType) MarshalTOML() ([]byte, error) {
	return []byte("\"" + v.String() + "\""), nil
}
