// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/test/testdata"
)

func TestFactomAddressesUpload(t *testing.T) {
	value, err := LoadFactomAddressesAndBalances(strings.NewReader(testdata.FactomAddresses))
	require.NoError(t, err)
	for _, v := range value {
		fmt.Print("Address : ", v.Address)
		fmt.Println("Balance : ", v.Balance)
	}
}
