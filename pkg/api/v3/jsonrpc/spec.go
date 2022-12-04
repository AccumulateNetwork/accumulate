//go:build spec

package jsonrpc

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

type API struct{}

func (API) NodeStatus(message.NodeStatusRequest) api.NodeStatus
func (API) NetworkStatus(message.NetworkStatusRequest) api.NetworkStatus
func (API) Metrics(message.MetricsRequest) api.Metrics
func (API) Query(message.QueryRequest) api.Record
func (API) Submit(message.SubmitRequest) []api.Submission
func (API) Validate(message.SubmitRequest) []api.Submission
