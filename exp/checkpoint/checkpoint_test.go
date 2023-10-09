// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package checkpoint

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestCheckpoint(t *testing.T) {
	c, err := LoadMainNet()
	require.NoError(t, err)

	var account *protocol.DataAccount
	err = c.Account(protocol.DnUrl().JoinPath(protocol.Network)).Main().GetAs(&account)
	require.NoError(t, err)

	value := new(protocol.NetworkDefinition)
	err = value.UnmarshalBinary(account.Entry.GetData()[0])
	require.NoError(t, err)

	b, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	fmt.Println(string(b))
}
