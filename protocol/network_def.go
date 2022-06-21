package protocol

import "strings"

func (n *NetworkDefinition) Partition(id string) *PartitionDefinition {
	for i, s := range n.Partitions {
		if strings.EqualFold(s.PartitionID, id) {
			return &n.Partitions[i]
		}
	}
	return nil
}
