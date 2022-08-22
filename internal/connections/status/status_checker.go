package statuschk

import (
	"context"
	"flag"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
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
	address, err := config.OffsetPort(connCtx.GetAddress(), connCtx.GetBasePort(), int(config.PortOffsetAccumulateApi))
	if err != nil {
		return nil, fmt.Errorf("invalid address: %v", err)
	}

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
