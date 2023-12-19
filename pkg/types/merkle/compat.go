// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"

const (
	ChainTypeUnknown     = merkle.ChainTypeUnknown
	ChainTypeTransaction = merkle.ChainTypeTransaction
	ChainTypeAnchor      = merkle.ChainTypeAnchor
	ChainTypeIndex       = merkle.ChainTypeIndex
)

type (
	ChainType       = merkle.ChainType
	State           = merkle.State
	Receipt         = merkle.Receipt
	ReceiptEntry    = merkle.ReceiptEntry
	ValidateOptions = merkle.ValidateOptions
)
