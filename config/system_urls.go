package config

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NodeUrl returns the URL of the subnet, optionally with a path appended.
func (n *Network) NodeUrl(path ...string) *url.URL {
	if n.Type == Directory {
		return protocol.DnUrl().JoinPath(path...)
	}

	return protocol.SubnetUrl(n.LocalSubnetID).JoinPath(path...)
}

// Ledger returns the URL of the subnet's ledger account.
func (n *Network) Ledger() *url.URL {
	return n.NodeUrl(protocol.Ledger)
}

// SyntheticLedger returns the URL of the subnet's synthetic transaction ledger account.
func (n *Network) SyntheticLedger() *url.URL {
	return n.NodeUrl(protocol.SyntheticLedgerPath)
}

// ValidatorBook returns the URL of the subnet's validator key book.
func (n *Network) ValidatorBook() *url.URL {
	return n.NodeUrl(protocol.ValidatorBook)
}

// ValidatorPage returns the URL of the page of the subnet's validator key book.
func (n *Network) ValidatorPage(index uint64) *url.URL {
	return protocol.FormatKeyPageUrl(n.ValidatorBook(), index)
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (n *Network) AnchorPool() *url.URL {
	return n.NodeUrl(protocol.AnchorPool)
}
