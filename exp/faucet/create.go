// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package faucet

import (
	"math"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CreateLite creates a lite token account with 200M ACME and a corresponding
// lite identity and adds them to a snapshot suitable for use with genesis.
func CreateLite(url *url.URL) ([]byte, error) {
	lta := new(protocol.LiteTokenAccount)
	lta.Url = url
	lta.TokenUrl = protocol.AcmeUrl()
	lta.Balance = *big.NewInt(200_000_000 * protocol.AcmePrecision)

	lid := new(protocol.LiteIdentity)
	lid.Url = lta.Url.RootIdentity()
	lid.CreditBalance = math.MaxUint64

	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	batch := db.Begin(true)
	defer batch.Discard()
	for _, a := range []protocol.Account{lta, lid} {
		err := batch.Account(a.GetUrl()).Main().Put(a)
		if err != nil {
			return nil, err
		}
	}
	err := batch.UpdateBPT()
	if err != nil {
		return nil, err
	}
	err = batch.Commit()
	if err != nil {
		return nil, err
	}

	buf := new(ioutil.Buffer)
	_, err = db.Collect(buf, nil, &database.CollectOptions{
		Predicate: func(r record.Record) (bool, error) {
			// Do not create a BPT
			if r.Key().Get(0) == "BPT" {
				return false, nil
			}
			return true, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
