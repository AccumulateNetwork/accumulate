package testing

import (
	"context"

	"github.com/tendermint/tendermint/rpc/client"
	core "github.com/tendermint/tendermint/rpc/coretypes"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NullRouter struct{}

var _ routing.Router = NullRouter{}

func (NullRouter) RouteAccount(*url.URL) (string, error) {
	return "", nil
}

func (NullRouter) Route(...*protocol.Envelope) (string, error) {
	return "", nil
}

func (NullRouter) Query(ctx context.Context, partition string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	return nil, errors.StatusNotFound
}

func (NullRouter) Submit(ctx context.Context, partition string, tx *protocol.Envelope, pretend, async bool) (*routing.ResponseSubmit, error) {
	return new(routing.ResponseSubmit), nil
}

func (NullRouter) RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error {
	return errors.StatusNotFound
}
