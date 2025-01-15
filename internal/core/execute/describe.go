// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type DescribeShim struct {
	NetworkType protocol.PartitionType
	PartitionId string
}

// PartitionUrl returns a NetworkUrl for the local partition.
func (n *DescribeShim) PartitionUrl() config.NetworkUrl {
	return config.NetworkUrl{URL: protocol.PartitionUrl(n.PartitionId)}
}

// NodeUrl returns the URL of the partition, optionally with a path appended.
func (n *DescribeShim) NodeUrl(path ...string) *url.URL {
	return protocol.PartitionUrl(n.PartitionId).JoinPath(path...)
}

// Ledger returns the URL of the partition's ledger account.
func (n *DescribeShim) Ledger() *url.URL {
	return n.PartitionUrl().Ledger()
}

// BlockLedger returns the URL of a partition's ledger for a block.
func (n *DescribeShim) BlockLedger(i uint64) *url.URL {
	return n.PartitionUrl().BlockLedger(i)
}

// Synthetic returns the URL of the partition's ledger account.
func (n *DescribeShim) Synthetic() *url.URL {
	return n.PartitionUrl().Synthetic()
}

// Operators returns the URL of the partition's operator key book.
func (n *DescribeShim) Operators() *url.URL {
	return n.PartitionUrl().Operators()
}

// OperatorsPage returns the URL of the default page of the partition's operator key book.
func (n *DescribeShim) OperatorsPage() *url.URL {
	return n.PartitionUrl().OperatorsPage()
}

// AnchorPool returns the URL of the partition's anchor pool.
func (n *DescribeShim) AnchorPool() *url.URL {
	return n.PartitionUrl().AnchorPool()
}
