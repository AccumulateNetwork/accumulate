package config

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkUrl struct {
	*url.URL
}

// Ledger returns the URL of the partition's ledger account.
func (u NetworkUrl) Ledger() *url.URL {
	return u.JoinPath(protocol.Ledger)
}

// Synthetic returns the URL of the partition's synthetic transaction ledger account.
func (u NetworkUrl) Synthetic() *url.URL {
	return u.JoinPath(protocol.Synthetic)
}

// ValidatorBook returns the URL of the partition's validator key book.
func (u NetworkUrl) ValidatorBook() *url.URL {
	return u.JoinPath(protocol.ValidatorBook)
}

// ValidatorPage returns the URL of the page of the partition's validator key book.
func (u NetworkUrl) ValidatorPage(index uint64) *url.URL {
	return protocol.FormatKeyPageUrl(u.ValidatorBook(), index)
}

// OperatorBook returns the URL of the partition's operator key book.
func (u NetworkUrl) OperatorBook() *url.URL {
	return u.JoinPath(protocol.OperatorBook)
}

// OperatorPage returns the URL of the page of the partition's operator key book.
func (u NetworkUrl) OperatorPage(index uint64) *url.URL {
	return protocol.FormatKeyPageUrl(u.OperatorBook(), index)
}

// AnchorPool returns the URL of the partition's anchor pool.
func (u NetworkUrl) AnchorPool() *url.URL {
	return u.JoinPath(protocol.AnchorPool)
}

// NodeUrl returns the URL of the partition, optionally with a path appended.
func (n *Network) NodeUrl(path ...string) *url.URL {
	return protocol.PartitionUrl(n.LocalPartitionID).JoinPath(path...)
}

// Ledger returns the URL of the partition's ledger account.
func (n *Network) Ledger() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.LocalPartitionID)}.Ledger()
}

// Synthetic returns the URL of the partition's ledger account.
func (n *Network) Synthetic() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.LocalPartitionID)}.Synthetic()
}

// ValidatorBook returns the URL of the partition's validator key book.
func (n *Network) ValidatorBook() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.LocalPartitionID)}.ValidatorBook()
}

// ValidatorPage returns the URL of the page of the partition's validator key book.
func (n *Network) ValidatorPage(index uint64) *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.LocalPartitionID)}.ValidatorPage(index)
}

// OperatorBook returns the URL of the partition's operator key book.
func (n *Network) OperatorBook() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.LocalPartitionID)}.OperatorBook()
}

// OperatorPage returns the URL of the page of the partition's operator key book.
func (n *Network) OperatorPage(index uint64) *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.LocalPartitionID)}.OperatorPage(index)
}

// DefaultValidatorPage returns the URL of the default page of the partition operator key book.
func (n *Network) DefaultValidatorPage() *url.URL {
	var index uint64
	if n.Type == Directory {
		index = 0
	} else {
		index = 0 // 1 in AC-1402
	}
	return n.ValidatorPage(index)
}

// DefaultOperatorPage returns the URL of the default page of the partition operator key book.
func (n *Network) DefaultOperatorPage() *url.URL {
	var index uint64
	if n.Type == Directory {
		index = 0
	} else {
		index = 1
	}
	return n.OperatorPage(index)
}

// AnchorPool returns the URL of the partition's anchor pool.
func (n *Network) AnchorPool() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.LocalPartitionID)}.AnchorPool()
}
