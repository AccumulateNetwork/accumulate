package statuschk

import (
	"context"
	"flag"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
)

type NodeStatusChecker interface {
	IsStatusOk(connCtx connections.ConnectionContext) bool
}

type nodeStatusChecker struct {
	clientMap map[connections.ConnectionContext]*client.Client
	testMode  bool
}

func NewNodeStatusChecker() NodeStatusChecker {
	n := &nodeStatusChecker{}

	// The API endpoints are not available when running unit tests, so we can't fail on it there.
	if flag.Lookup("test.v") != nil {
		n.testMode = true
	} else {
		n.clientMap = make(map[connections.ConnectionContext]*client.Client)
	}
	return n
}

func (sc *nodeStatusChecker) createAccApiClient(connCtx connections.ConnectionContext) (*client.Client, error) {
	address := connCtx.GetAddress2().WithOffset(int(config.PortOffsetAccumulateApi))
	client, err := client.New(address.String() + "/v2")
	return client, err
}

func (sc *nodeStatusChecker) IsStatusOk(connCtx connections.ConnectionContext) bool {
	if sc.testMode {
		return true
	}

	accClient, ok := sc.clientMap[connCtx]
	if !ok {
		var err error
		accClient, err = sc.createAccApiClient(connCtx)
		if err != nil {
			return false
		}
		sc.clientMap[connCtx] = accClient
	}
	res, err := accClient.Status(context.Background())
	return err != nil && res.Ok
}
