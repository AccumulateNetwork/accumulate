// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestAccountState(t *testing.T) {
	db, err := database.OpenBadger("/home/firelizzard/Desktop/temp/bvnn/data/accumulate.db", nil)
	require.NoError(t, err)
	db.SetObserver(internal.NewDatabaseObserver())

	var pending []*url.TxID
	batch := db.Begin(true)
	defer batch.Discard()
	start := time.Now()
	require.NoError(t, batch.ForEachAccount(func(account *database.Account) error {
		p, err := account.Pending().Get()
		if err != nil {
			return err
		}
		pending = append(pending, p...)
		return nil
	}))
	require.NoError(t, batch.Commit())
	fmt.Println(time.Since(start))

	fmt.Printf("%d pending transactions\n", len(pending))
}
