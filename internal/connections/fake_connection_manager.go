package connections

import (
	"github.com/tendermint/tendermint/rpc/client/local"
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
