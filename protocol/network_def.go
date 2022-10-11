package protocol

import (
	"bytes"
	"crypto/sha256"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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

// IsActiveOn returns true if the validator is active on the partition.
func (v *ValidatorInfo) IsActiveOn(partition string) bool {
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

// RemoveValidatorFrom completely removes a validator.
func (n *NetworkDefinition) RemoveValidator(key []byte, partition string) {
	hash := sha256.Sum256(key)
	_, v, _ := n.ValidatorByHash(hash[:])

	for _, p := range v.Partitions {
		if strings.EqualFold(p.ID, partition) {
			p.Active = false
			return
		}
	}
}

// UpdateValidatorKey updates a validator's key.
func (n *NetworkDefinition) UpdateValidatorKey(oldKey, newKey []byte) error {
	oldHash := sha256.Sum256(oldKey)
	i, found := sortutil.Search(n.Validators, valHashCmp(oldHash[:]).cmp)
	if !found {
		return errors.NotFound("validator %x not found", oldKey[:4])
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
		return errors.Format(errors.StatusConflict, "validator %x already exists", newKey[:4])
	}
	*ptr = v
	n.Validators = val
	return nil
}

// UpdateValidatorName updates a validator's name.
func (n *NetworkDefinition) UpdateValidatorName(key []byte, name *url.URL) bool {
	hash := sha256.Sum256(key)
	_, v, ok := n.ValidatorByHash(hash[:])
	if !ok {
		return false
	}
	v.Operator = name
	return true
}

// MarshalTOML marshals the partition type to Toml as a string.
func (v PartitionType) MarshalTOML() ([]byte, error) {
	return []byte("\"" + v.String() + "\""), nil
}
