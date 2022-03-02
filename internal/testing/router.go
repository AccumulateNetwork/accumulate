package testing

import (
	"context"

	"github.com/tendermint/tendermint/rpc/client"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type NullRouter struct{}

var _ routing.Router = NullRouter{}

func (NullRouter) Route(account *url.URL) (string, error) {
	return "", nil
}

func (NullRouter) Query(ctx context.Context, subnet string, query []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	return nil, storage.ErrNotFound
}

func (NullRouter) Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*routing.ResponseSubmit, error) {
	return new(routing.ResponseSubmit), nil
}
