package testing

import (
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/ybbus/jsonrpc/v2"
)

type fakeNodeContext struct {
	subnetName   string
	address      string
	nodeUrl      string
	connMgr      *fakeConnectionManager
	netType      config.NetworkType
	nodeType     config.NodeType
	networkGroup connections.NetworkGroup

	fakeClient *FakeTendermint
}

func (f fakeNodeContext) GetSubnetName() string {
	return f.subnetName
}

func (f fakeNodeContext) GetNodeUrl() string {
	return f.nodeUrl
}

func (f fakeNodeContext) GetQueryClient() connections.ABCIQueryClient {
	return f.fakeClient
}

func (f fakeNodeContext) GetNetworkGroup() connections.NetworkGroup {
	return f.networkGroup
}

func (f fakeNodeContext) GetNodeType() config.NodeType {
	return f.nodeType
}

func (f fakeNodeContext) GetMetrics() *connections.NodeMetrics {
	n := &connections.NodeMetrics{}
	n.SetStatus(connections.Up)
	return n
}

func (f fakeNodeContext) GetLastError() error {
	return nil
}

func (f fakeNodeContext) SetQueryClient(client connections.ABCIQueryClient) {
}

func (f fakeNodeContext) SetBroadcastClient(client connections.ABCIBroadcastClient) {
}

func (f fakeNodeContext) GetBroadcastClient() connections.ABCIBroadcastClient {
	return f.fakeClient
}

func (f fakeNodeContext) SetBatchBroadcastClient(client connections.BatchABCIBroadcastClient) {
}

func (f fakeNodeContext) GetBatchBroadcastClient() connections.BatchABCIBroadcastClient {
	return nil
}

func (f fakeNodeContext) SetService(client service.Service) {
}

func (f fakeNodeContext) GetAddress() string {
	return f.address
}

func (f fakeNodeContext) SetRawClient(client connections.RawClient) {
}

func (f fakeNodeContext) GetRawClient() connections.RawClient {
	//TODO implement me
	panic("implement me")
}

func (f fakeNodeContext) SetNodeUrl(addr string) {
	f.nodeUrl = addr
}

func (f fakeNodeContext) SetJsonRpcClient(client jsonrpc.RPCClient) {
}

func (f fakeNodeContext) GetJsonRpcClient() jsonrpc.RPCClient {
	//TODO implement me
	panic("implement me")
}

func (f fakeNodeContext) IsDirectoryNode() bool {
	return f.netType == config.Directory && f.nodeType == config.Validator
}

func (f fakeNodeContext) IsHealthy() bool {
	return true
}

func (f fakeNodeContext) ReportError(err error) {
}

func (f fakeNodeContext) ReportErrorStatus(status connections.NodeStatus, err error) {
}

func (f fakeNodeContext) ClearErrors() {
}
