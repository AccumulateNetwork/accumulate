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

// Synthetic returns the URL of the subnet's synthetic transaction ledger account.
func (u NetworkUrl) Synthetic() *url.URL {
	return u.JoinPath(protocol.Synthetic)
}

// ValidatorBook returns the URL of the subnet's validator key book.
func (u NetworkUrl) ValidatorBook() *url.URL {
	return u.JoinPath(protocol.ValidatorBook)
}

// ValidatorPage returns the URL of the page of the subnet's validator key book.
func (u NetworkUrl) ValidatorPage(index uint64) *url.URL {
	return protocol.FormatKeyPageUrl(u.ValidatorBook(), index)
}

// OperatorBook returns the URL of the subnet's operator key book.
func (u NetworkUrl) OperatorBook() *url.URL {
	return u.JoinPath(protocol.OperatorBook)
}

// OperatorPage returns the URL of the page of the subnet's operator key book.
func (u NetworkUrl) OperatorPage(index uint64) *url.URL {
	return protocol.FormatKeyPageUrl(u.OperatorBook(), index)
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (u NetworkUrl) AnchorPool() *url.URL {
	return u.JoinPath(protocol.AnchorPool)
}

// NodeUrl returns the URL of the subnet, optionally with a path appended.
func (n *Accumulate) NodeUrl(path ...string) *url.URL {
	return protocol.SubnetUrl(n.SubnetId).JoinPath(path...)
}

// Ledger returns the URL of the subnet's ledger account.
func (n *Accumulate) Ledger() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.SubnetId)}.Ledger()
}

// Synthetic returns the URL of the subnet's ledger account.
func (n *Accumulate) Synthetic() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.SubnetId)}.Synthetic()
}

// ValidatorBook returns the URL of the subnet's validator key book.
func (n *Accumulate) ValidatorBook() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.SubnetId)}.ValidatorBook()
}

// ValidatorPage returns the URL of the page of the subnet's validator key book.
func (n *Accumulate) ValidatorPage(index uint64) *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.SubnetId)}.ValidatorPage(index)
}

// OperatorBook returns the URL of the subnet's operator key book.
func (n *Accumulate) OperatorBook() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.SubnetId)}.OperatorBook()
}

// OperatorPage returns the URL of the page of the subnet's operator key book.
func (n *Accumulate) OperatorPage(index uint64) *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.SubnetId)}.OperatorPage(index)
}

// DefaultValidatorPage returns the URL of the default page of the subnet operator key book.
func (n *Accumulate) DefaultValidatorPage() *url.URL {
	var index uint64
	if n.Type == Directory {
		index = 0
	} else {
		index = 0 // 1 in AC-1402
	}
	return n.ValidatorPage(index)
}

// DefaultOperatorPage returns the URL of the default page of the subnet operator key book.
func (n *Network) DefaultOperatorPage() *url.URL {
	var index uint64
	if n.Type == Directory {
		index = 0
	} else {
		index = 1
	}
	return n.OperatorPage(index)
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (n *Network) AnchorPool() *url.URL {
	return NetworkUrl{protocol.SubnetUrl(n.LocalSubnetID)}.AnchorPool()
}
