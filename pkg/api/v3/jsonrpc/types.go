package jsonrpc

import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"

//go:generate go run ../../../../tools/cmd/gen-types --package jsonrpc requests.yml --reference ../options.yml

type RangeOptions = api.RangeOptions
type NodeStatusOptions = api.NodeStatusOptions
type NetworkStatusOptions = api.NetworkStatusOptions
type MetricsOptions = api.MetricsOptions
type SubmitOptions = api.SubmitOptions
type ValidateOptions = api.ValidateOptions
