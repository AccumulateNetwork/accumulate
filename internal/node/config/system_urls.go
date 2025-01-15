// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"strconv"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkUrl struct {
	*url.URL
}

func (u NetworkUrl) PartitionID() string {
	id, _ := protocol.ParsePartitionUrl(u.URL)
	return id
}

// Ledger returns the URL of the partition's identity.
func (u NetworkUrl) Identity() *url.URL {
	return u.URL
}

// Ledger returns the URL of the partition's ledger account.
func (u NetworkUrl) Ledger() *url.URL {
	return u.JoinPath(protocol.Ledger)
}

// BlockLedger returns the URL of a partition's ledger for a block.
func (u NetworkUrl) BlockLedger(i uint64) *url.URL {
	return u.JoinPath(protocol.Ledger, strconv.FormatUint(i, 10))
}

// Synthetic returns the URL of the partition's synthetic transaction ledger account.
func (u NetworkUrl) Synthetic() *url.URL {
	return u.JoinPath(protocol.Synthetic)
}

// Operators returns the URL of the partition's operator key book.
func (u NetworkUrl) Operators() *url.URL {
	return u.JoinPath(protocol.Operators)
}

// OperatorsPage returns the URL of the default page of the partition's operator key book.
func (n NetworkUrl) OperatorsPage() *url.URL {
	return protocol.FormatKeyPageUrl(n.Operators(), 0)
}

// AnchorPool returns the URL of the partition's anchor pool.
func (u NetworkUrl) AnchorPool() *url.URL {
	return u.JoinPath(protocol.AnchorPool)
}

// PartitionUrl returns a NetworkUrl for the local partition.
func (n *Describe) PartitionUrl() NetworkUrl {
	return NetworkUrl{protocol.PartitionUrl(n.PartitionId)}
}

// NodeUrl returns the URL of the partition, optionally with a path appended.
func (n *Describe) NodeUrl(path ...string) *url.URL {
	return protocol.PartitionUrl(n.PartitionId).JoinPath(path...)
}

// Ledger returns the URL of the partition's ledger account.
func (n *Describe) Ledger() *url.URL {
	return n.PartitionUrl().Ledger()
}

// BlockLedger returns the URL of a partition's ledger for a block.
func (n *Describe) BlockLedger(i uint64) *url.URL {
	return n.PartitionUrl().BlockLedger(i)
}

// Synthetic returns the URL of the partition's ledger account.
func (n *Describe) Synthetic() *url.URL {
	return n.PartitionUrl().Synthetic()
}

// Operators returns the URL of the partition's operator key book.
func (n *Describe) Operators() *url.URL {
	return n.PartitionUrl().Operators()
}

// OperatorsPage returns the URL of the default page of the partition's operator key book.
func (n *Describe) OperatorsPage() *url.URL {
	return n.PartitionUrl().OperatorsPage()
}

// AnchorPool returns the URL of the partition's anchor pool.
func (n *Describe) AnchorPool() *url.URL {
	return n.PartitionUrl().AnchorPool()
}
