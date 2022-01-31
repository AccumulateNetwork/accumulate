package api

import (
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/routing"
	"github.com/tendermint/tendermint/libs/log"
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
