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

// NodeUrl returns the URL of the subnet, optionally with a path appended.
func (n *Describe) NodeUrl(path ...string) *url.URL {
	return protocol.PartitionUrl(n.PartitionId).JoinPath(path...)
}

// Ledger returns the URL of the subnet's ledger account.
func (n *Describe) Ledger() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.PartitionId)}.Ledger()
}

// Synthetic returns the URL of the subnet's ledger account.
func (n *Describe) Synthetic() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.PartitionId)}.Synthetic()
}

// ValidatorBook returns the URL of the subnet's validator key book.
func (n *Describe) ValidatorBook() *url.URL {
	return NetworkUrl{protocol.PartitionUrl(n.PartitioId)}.ValidatorBook()
}

// ValidatorPage returns the URL of the page of the subnet's validator key book.
func (n *Describe) ValidatorPage(index uint64) *url.URL {
	return NetworkUrl{protocol.PartitioUrl(n.PartitioId)}.ValidatorPage(index)
}

// DefaultValidatorPage returns the URL of the default page of the subnet's validator key book.
func (n *Describe) DefaultValidatorPage() *url.URL {
	return n.ValidatorPage(1)
}

// OperatorBook returns the URL of the subnet's operator key book.
func (n *Describe) OperatorBook() *url.URL {
	return NetworkUrl{protocol.PartitioUrl(n.PartitioId)}.OperatorBook()
}

// OperatorPage returns the URL of the page of the subnet's operator key book.
func (n *Describe) OperatorPage(index uint64) *url.URL {
	return NetworkUrl{protocol.PartitioUrl(n.PartitioId)}.OperatorPage(index)
}

// DefaultOperatorPage returns the URL of the default page of the subnet operator key book.
func (n *Describe) DefaultOperatorPage() *url.URL {
	var index uint64
	if n.NetworkType == Directory {
		index = 0
	} else {
		index = 1
	}
	return n.OperatorPage(index)
}

// AnchorPool returns the URL of the subnet's anchor pool.
func (n *Describe) AnchorPool() *url.URL {
	return NetworkUrl{protocol.PartitioUrl(n.PartitioId)}.AnchorPool()
}
