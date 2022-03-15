package api

import (
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
)

//go:generate go run ../../../tools/cmd/gen-types --package api types.yml
//go:generate go run ../../../tools/cmd/genapi --package api methods.yml
//go:generate go run github.com/golang/mock/mockgen -source ../../routing/router.go -package api_test -destination ./mock_router_test.go

type Options struct {
	Logger           log.Logger
	Network          *config.Network
	Router           routing.Router
	TxMaxWaitTime    time.Duration
	PrometheusServer string
}
