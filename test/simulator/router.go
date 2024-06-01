// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Router struct {
	tree      *routing.RouteTree
	logger    logging.OptionalLogger
	overrides map[[32]byte]string
}

// ResponseSubmit is the response from a call to Submit.
type ResponseSubmit struct {
	Code         uint32
	Data         []byte
	Log          string
	Info         string
	Codespace    string
	MempoolError string
}

func newRouter(logger log.Logger) *Router {
	r := new(Router)
	r.logger.Set(logger, "module", "router")
	r.overrides = map[[32]byte]string{}
	return r
}

func (r *Router) willChangeGlobals(e events.WillChangeGlobals) error {
	tree, err := routing.NewRouteTree(e.New.Routing)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	r.tree = tree
	return nil
}

func (r *Router) SetRoute(account *url.URL, partition string) {
	if partition == "" {
		delete(r.overrides, account.IdentityAccountID32())
	} else {
		r.overrides[account.IdentityAccountID32()] = partition
	}
}

func (r *Router) RouteAccount(account *url.URL) (string, error) {
	if part, ok := r.overrides[account.IdentityAccountID32()]; ok {
		return part, nil
	}
	if r.tree == nil {
		return "", errors.InternalError.With("the routing table has not been initialized")
	}
	if protocol.IsUnknown(account) {
		return "", errors.BadRequest.With("URL is unknown, cannot route")
	}
	return r.tree.Route(account)
}

func (r *Router) Route(envs ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.RouteAccount, envs...)
}
