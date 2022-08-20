package api

import (
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/routing"
)

//go:generate go run ../../../tools/cmd/gen-types --package api types.yml
//go:generate go run ../../../tools/cmd/gen-api --package api methods.yml
//go:generate go run github.com/golang/mock/mockgen -source ../../node/routing/router.go -package api_test -destination ./mock_router_test.go

type Options struct {
	Logger            log.Logger
	Describe          *config.Describe
	Router            routing.Router
	TxMaxWaitTime     time.Duration
	PrometheusServer  string
	Database          database.Beginner
	ConnectionManager connections.ConnectionManager
	Key               []byte
}
