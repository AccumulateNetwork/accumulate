package protocol

import (
	"bytes"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
)

// MarshalTOML marshals the Network Type to Toml as a string.
func (v PartitionType) MarshalTOML() ([]byte, error) {
	return []byte("\"" + v.String() + "\""), nil
}

func (n *NetworkDefinition) Partition(id string) *PartitionDefinition {
	for i, s := range n.Partitions {
		if strings.EqualFold(s.ID, id) {
			return n.Partitions[i]
		}
	}
	return nil
}

func (p *PartitionDefinition) PartitionID() string { return p.ID }
func (p *PartitionDefinition) SubnetID() string    { return p.ID }

func (p *PartitionDefinition) FindValidator(key []byte) *ValidatorDefinition {
	i, found := sortutil.Search(p.Validators, func(a *ValidatorDefinition) int { return bytes.Compare(a.PublicKey, key) })
	if found {
		return p.Validators[i]
	}
	return nil
}

func (p *PartitionDefinition) UpdateValidator(key []byte, active bool) {
	ptr, new := sortutil.BinaryInsert(&p.Validators, func(a *ValidatorDefinition) int { return bytes.Compare(a.PublicKey, key) })
	if new {
		*ptr = &ValidatorDefinition{PublicKey: key}
	}
	(*ptr).Active = active
}

func (p *PartitionDefinition) RemoveValidator(key []byte) {
	i, found := sortutil.Search(p.Validators, func(a *ValidatorDefinition) int { return bytes.Compare(a.PublicKey, key) })
	if found {
		p.Validators = append(p.Validators[:i], p.Validators[i+1:]...)
	}
}
