// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package faucet

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestCreate(t *testing.T) {
	key := acctesting.GenerateKey()
	u, err := LiteTokenAddress(key[32:], "ACME", SignatureTypeED25519)
	require.NoError(t, err)

	snap, err := CreateLite(u)
	require.NoError(t, err)

	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())

	err = database.Restore(db, ioutil.NewBuffer(snap), &database.RestoreOptions{
		BatchRecordLimit: 50_000,
		SkipHashCheck:    true,
	})
	require.NoError(t, err)

	lite := GetAccount[*LiteTokenAccount](t, db, u)
	require.Equal(t,
		FormatAmount(200_000_000*AcmePrecision, AcmePrecisionPower),
		FormatBigAmount(&lite.Balance, AcmePrecisionPower))
}
