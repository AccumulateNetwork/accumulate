package protocol

import (
	"bytes"
	"strings"
)

func (n *NetworkDefinition) Partition(id string) *PartitionDefinition {
	for i, s := range n.Partitions {
		if strings.EqualFold(s.PartitionID, id) {
			return n.Partitions[i]
		}
	}
	return nil
}

func (n *PartitionDefinition) FindValidator(pubKey []byte) bool {
	// TODO Make the array ordered so we can do a binary search
	for _, k := range n.ValidatorKeys {
		if bytes.Equal(k, pubKey) {
			return true
		}
	}
	return false
}
