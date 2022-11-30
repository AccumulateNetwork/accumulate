// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"context"

	"github.com/tendermint/tendermint/rpc/client"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
	return nil, errors.NotFound
}

func (NullRouter) Submit(ctx context.Context, partition string, tx *protocol.Envelope, pretend, async bool) (*routing.ResponseSubmit, error) {
	return new(routing.ResponseSubmit), nil
}

func (NullRouter) RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error {
	return errors.NotFound
}
