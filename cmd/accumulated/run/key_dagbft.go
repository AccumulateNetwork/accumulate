// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build dagbft

package run

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
)

// CometPrivValFile is not supported when building with dagbft tag
func (k *CometPrivValFile) get(inst *Instance) (address.Address, error) {
	return nil, fmt.Errorf("CometPrivValFile is not supported in dagbft mode")
}

// CometNodeKeyFile is not supported when building with dagbft tag
func (k *CometNodeKeyFile) get(inst *Instance) (address.Address, error) {
	return nil, fmt.Errorf("CometNodeKeyFile is not supported in dagbft mode")
}
