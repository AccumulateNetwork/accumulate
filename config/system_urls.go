package config

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkUrl struct {
	*url.URL
}

// Ledger returns the URL of the subnet's ledger account.
func (u NetworkUrl) Ledger() *url.URL {
	return u.JoinPath(protocol.Ledger)
}

// ValidatorBook returns the URL of the subnet's validator key book.
func (u NetworkUrl) ValidatorBook() *url.URL {
	return u.JoinPath(protocol.ValidatorBook)
}

// ValidatorPage returns the URL of the page of the subnet's validator key book.
func (u NetworkUrl) ValidatorPage(index uint64) *url.URL {
	return protocol.FormatKeyPageUrl(u.ValidatorBook(), index)
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (u NetworkUrl) AnchorPool() *url.URL {
	return u.JoinPath(protocol.AnchorPool)
}

// NodeUrl returns the URL of the subnet, optionally with a path appended.
func (n *Network) NodeUrl(path ...string) *url.URL {
	return protocol.SubnetUrl(n.LocalSubnetID).JoinPath(path...)
}

// Ledger returns the URL of the subnet's ledger account.
func (n *Network) Ledger() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.LocalSubnetID)}.Ledger()
}

// ValidatorBook returns the URL of the subnet's validator key book.
func (n *Network) ValidatorBook() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.LocalSubnetID)}.ValidatorBook()
}

// ValidatorPage returns the URL of the page of the subnet's validator key book.
func (n *Network) ValidatorPage(index uint64) *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.LocalSubnetID)}.ValidatorPage(index)
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (n *Network) AnchorPool() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.LocalSubnetID)}.AnchorPool()
}
