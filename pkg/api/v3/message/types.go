package message

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package message enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --elide-package-type --package message messages.yml private.yml --reference ../options.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --elide-package-type --package message --language go-union --out unions_gen.go messages.yml private.yml --reference ../options.yml

type Type int

type NodeStatusOptions = api.NodeStatusOptions
type NetworkStatusOptions = api.NetworkStatusOptions
type MetricsOptions = api.MetricsOptions
type SubscribeOptions = api.SubscribeOptions
type SubmitOptions = api.SubmitOptions
type ValidateOptions = api.ValidateOptions

type response[T any] interface {
	Message
	rval() T
}

func (r *NodeStatusResponse) rval() *api.NodeStatus             { return r.Value } //nolint:unused
func (r *NetworkStatusResponse) rval() *api.NetworkStatus       { return r.Value } //nolint:unused
func (r *MetricsResponse) rval() *api.Metrics                   { return r.Value } //nolint:unused
func (r *RecordResponse) rval() api.Record                      { return r.Value } //nolint:unused
func (r *SubmitResponse) rval() []*api.Submission               { return r.Value } //nolint:unused
func (r *ValidateResponse) rval() []*api.Submission             { return r.Value } //nolint:unused
func (r *PrivateSequenceResponse) rval() *api.TransactionRecord { return r.Value } //nolint:unused
