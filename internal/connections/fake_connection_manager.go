package connections

import (
	"context"
	"reflect"

	"github.com/golang/mock/gomock"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/local"
	coretypes "github.com/tendermint/tendermint/rpc/coretypes"
	"github.com/tendermint/tendermint/types"
)

type fakeConnectionManager struct {
	ctxMap map[string]ConnectionContext
}

func (fcm *fakeConnectionManager) SelectConnection(partitionId string, allowFollower bool) (ConnectionContext, error) {
	context, ok := fcm.ctxMap[partitionId]
	if ok {
		return context, nil
	} else {
		return nil, errUnknownPartition(partitionId)
	}
}

func NewFakeConnectionManager(clients map[string]ABCIClient) ConnectionManager {
	fcm := new(fakeConnectionManager)
	fcm.ctxMap = make(map[string]ConnectionContext)
	for partition, client := range clients {
		connCtx := &connectionContext{
			partitionId: partition,
			hasClient:   make(chan struct{}),
			metrics:     NodeMetrics{status: Up}}
		connCtx.setClient(client, nil)
		fcm.ctxMap[partition] = connCtx
	}
	return fcm
}

func (fcm *fakeConnectionManager) GetBVNContextMap() map[string][]ConnectionContext {
	return nil
}

func (fcm *fakeConnectionManager) GetDNContextList() []ConnectionContext {
	return nil
}

func (fcm *fakeConnectionManager) GetFNContextList() []ConnectionContext {
	return nil
}

func (fcm *fakeConnectionManager) GetAllNodeContexts() []ConnectionContext {
	return nil
}

func (fcm *fakeConnectionManager) GetLocalNodeContext() ConnectionContext {
	return nil
}

func (fcm *fakeConnectionManager) GetLocalClient() *local.Local {
	return nil
}

func (fcm *fakeConnectionManager) ResetErrors() {
}

// MockClient is a mock of Client interface.
type MockClient struct {
	ctrl     *gomock.Controller
	recorder *MockClientMockRecorder
}

// MockClientMockRecorder is the mock recorder for MockClient.
type MockClientMockRecorder struct {
	mock *MockClient
}

// NewMockClient creates a new mock instance.
func NewMockClient(ctrl *gomock.Controller) *MockClient {
	mock := &MockClient{ctrl: ctrl}
	mock.recorder = &MockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClient) EXPECT() *MockClientMockRecorder {
	return m.recorder
}

// ABCIQueryWithOptions mocks base method.
func (m *MockClient) ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts client.ABCIQueryOptions) (*coretypes.ResultABCIQuery, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ABCIQueryWithOptions", ctx, path, data, opts)
	ret0, _ := ret[0].(*coretypes.ResultABCIQuery)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ABCIQueryWithOptions indicates an expected call of ABCIQueryWithOptions.
func (mr *MockClientMockRecorder) ABCIQueryWithOptions(ctx, path, data, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ABCIQueryWithOptions", reflect.TypeOf((*MockClient)(nil).ABCIQueryWithOptions), ctx, path, data, opts)
}

// BroadcastTxAsync mocks base method.
func (m *MockClient) BroadcastTxAsync(arg0 context.Context, arg1 types.Tx) (*coretypes.ResultBroadcastTx, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BroadcastTxAsync", arg0, arg1)
	ret0, _ := ret[0].(*coretypes.ResultBroadcastTx)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BroadcastTxAsync indicates an expected call of BroadcastTxAsync.
func (mr *MockClientMockRecorder) BroadcastTxAsync(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BroadcastTxAsync", reflect.TypeOf((*MockClient)(nil).BroadcastTxAsync), arg0, arg1)
}

// BroadcastTxSync mocks base method.
func (m *MockClient) BroadcastTxSync(arg0 context.Context, arg1 types.Tx) (*coretypes.ResultBroadcastTx, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BroadcastTxSync", arg0, arg1)
	ret0, _ := ret[0].(*coretypes.ResultBroadcastTx)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BroadcastTxSync indicates an expected call of BroadcastTxSync.
func (mr *MockClientMockRecorder) BroadcastTxSync(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BroadcastTxSync", reflect.TypeOf((*MockClient)(nil).BroadcastTxSync), arg0, arg1)
}

// CheckTx mocks base method.
func (m *MockClient) CheckTx(ctx context.Context, tx types.Tx) (*coretypes.ResultCheckTx, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CheckTx", ctx, tx)
	ret0, _ := ret[0].(*coretypes.ResultCheckTx)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CheckTx indicates an expected call of CheckTx.
func (mr *MockClientMockRecorder) CheckTx(ctx, tx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CheckTx", reflect.TypeOf((*MockClient)(nil).CheckTx), ctx, tx)
}
