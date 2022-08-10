package protocol

import (
	"bytes"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
)

type valKeyCmp []byte

func (k valKeyCmp) cmp(v *ValidatorInfo) int {
	return bytes.Compare(v.PublicKey, k)
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

func (n *NetworkDefinition) Validator(key []byte) *ValidatorInfo {
	i, found := sortutil.Search(n.Validators, valKeyCmp(key).cmp)
	if !found {
		return nil
	}
	return n.Validators[i]
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
	ptr, new := sortutil.BinaryInsert(&n.Validators, valKeyCmp(key).cmp)
	if new {
		*ptr = &ValidatorInfo{PublicKey: key}
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
	i, found := sortutil.Search(n.Validators, valKeyCmp(key).cmp)
	if !found {
		return
	}

	n.Validators = append(n.Validators[:i], n.Validators[i+1:]...)
}

// UpdateValidatorKey updates a validator's key.
func (n *NetworkDefinition) UpdateValidatorKey(oldKey, newKey []byte) error {
	i, found := sortutil.Search(n.Validators, valKeyCmp(oldKey).cmp)
	if !found {
		return errors.NotFound("validator %x not found", oldKey[:4])
	}

	v := n.Validators[i]
	v.PublicKey = newKey

	val := make([]*ValidatorInfo, len(n.Validators)-1)
	copy(val, n.Validators[:i])
	copy(val[i:], n.Validators[i+1:])

	ptr, new := sortutil.BinaryInsert(&val, valKeyCmp(newKey).cmp)
	if !new {
		return errors.Format(errors.StatusConflict, "validator %x already exists", newKey[:4])
	}
	*ptr = v
	n.Validators = val
	return nil
}

// MarshalTOML marshals the partition type to Toml as a string.
func (v PartitionType) MarshalTOML() ([]byte, error) {
	return []byte("\"" + v.String() + "\""), nil
}
