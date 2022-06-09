package connections

import (
	"github.com/tendermint/tendermint/rpc/client/local"
)

type fakeConnectionManager struct {
	ctxMap map[string]ConnectionContext
}

func (fcm *fakeConnectionManager) SelectConnection(subnetId string, allowFollower bool) (ConnectionContext, error) {
	context, ok := fcm.ctxMap[subnetId]
	if ok {
		return context, nil
	} else {
		return nil, errUnknownSubnet(subnetId)
	}
}

func NewFakeConnectionManager(clients map[string]ABCIClient) ConnectionManager {
	fcm := new(fakeConnectionManager)
	fcm.ctxMap = make(map[string]ConnectionContext)
	for subnet, client := range clients {
		connCtx := &connectionContext{
			subnetId:  subnet,
			hasClient: make(chan struct{}),
			metrics:   NodeMetrics{status: Up}}
		connCtx.setClient(client, nil)
		fcm.ctxMap[subnet] = connCtx
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
