package protocol

import "strings"

func (n *NetworkDefinition) Subnet(id string) *SubnetDefinition {
	for i, s := range n.Subnets {
		if strings.EqualFold(s.SubnetID, id) {
			return &n.Subnets[i]
		}
	}
	return nil
}
