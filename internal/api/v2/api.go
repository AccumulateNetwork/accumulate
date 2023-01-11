// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	stdurl "net/url"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

//go:generate go run ../../../tools/cmd/gen-types --package api types.yml
//go:generate go run ../../../tools/cmd/gen-api --package api methods.yml
//go:generate go run github.com/golang/mock/mockgen -source ../../routing/router.go -package api_test -destination ./mock_router_test.go

type Options struct {
	Logger            log.Logger
	Describe          *config.Describe
	Router            routing.Router
	TxMaxWaitTime     time.Duration
	PrometheusServer  string
	Database          database.Beginner
	ConnectionManager connections.ConnectionManager
	Key               []byte
	ExplorerProxy     *stdurl.URL
}

func (o *Options) loadGlobals() (*core.GlobalValues, error) {
	v := new(core.GlobalValues)
	err := v.Load(o.Describe.PartitionUrl(), func(account *url.URL, target interface{}) error {
		return o.Database.View(func(batch *database.Batch) error {
			return batch.Account(account).GetStateAs(target)
		})
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}
