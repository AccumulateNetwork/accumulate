// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestFaucet(t *testing.T) {
	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)

	txn := &protocol.AcmeFaucet{Url: aliceUrl}
	req, payload, err := api.Package{}.ConstructFaucetTxn(txn)
	require.NoError(t, err)

	_, err = api.Package{}.ProcessExecuteRequest(req, payload)
	require.NoError(t, err)
}
