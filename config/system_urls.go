package config

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkUrl struct {
	*url.URL
}

// Ledger returns the URL of the subnet's identity.
func (u NetworkUrl) Identity() *url.URL {
	return u.URL
}

// Ledger returns the URL of the subnet's ledger account.
func (u NetworkUrl) Ledger() *url.URL {
	return u.JoinPath(protocol.Ledger)
}

// Synthetic returns the URL of the subnet's synthetic transaction ledger account.
func (u NetworkUrl) Synthetic() *url.URL {
	return u.JoinPath(protocol.Synthetic)
}

// Operators returns the URL of the subnet's operator key book.
func (u NetworkUrl) Operators() *url.URL {
	return u.JoinPath(protocol.Operators)
}

// OperatorsPage returns the URL of the default page of the subnet's operator key book.
func (n NetworkUrl) OperatorsPage() *url.URL {
	return protocol.FormatKeyPageUrl(n.Operators(), 0)
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (u NetworkUrl) AnchorPool() *url.URL {
	return u.JoinPath(protocol.AnchorPool)
}

// PartitionUrl returns a NetworkUrl for the local partition.
func (n *Describe) PartitionUrl() NetworkUrl {
	return NetworkUrl{protocol.SubnetUrl(n.SubnetId)}
}

// NodeUrl returns the URL of the subnet, optionally with a path appended.
func (n *Describe) NodeUrl(path ...string) *url.URL {
	return protocol.SubnetUrl(n.SubnetId).JoinPath(path...)
}

// Ledger returns the URL of the subnet's ledger account.
func (n *Describe) Ledger() *url.URL {
	return n.PartitionUrl().Ledger()
}

// Synthetic returns the URL of the subnet's ledger account.
func (n *Describe) Synthetic() *url.URL {
	return n.PartitionUrl().Synthetic()
}

// Operators returns the URL of the subnet's operator key book.
func (n *Describe) Operators() *url.URL {
	return n.PartitionUrl().Operators()
}

// OperatorsPage returns the URL of the default page of the subnet's operator key book.
func (n *Describe) OperatorsPage() *url.URL {
	return n.PartitionUrl().OperatorsPage()
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (n *Describe) AnchorPool() *url.URL {
	return n.PartitionUrl().AnchorPool()
}
