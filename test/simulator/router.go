// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"sync"
	"sync/atomic"
)

type Router struct {
	// tree is REPLACED on a globals change, never mutated, and read while
	// routing on other goroutines. An atomic store is also what gives the
	// reader a happens-before edge to the tree's construction — without it a
	// reader could observe a partly-built tree, which is what the
	// RouteTree.Route vs NewRouteTree race reports were (#4170).
	tree atomic.Pointer[routing.RouteTree]

	logger logging.OptionalLogger

	// overrides is written by SetRoute from the test's goroutine and read
	// while routing on the nodes'.
	overridesMu sync.RWMutex
	overrides   map[[32]byte]string
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

func newRouter(logger logging.Logger) *Router {
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

	r.tree.Store(tree)
	return nil
}

func (r *Router) SetRoute(account *url.URL, partition string) {
	r.overridesMu.Lock()
	defer r.overridesMu.Unlock()
	if partition == "" {
		delete(r.overrides, account.IdentityAccountID32())
	} else {
		r.overrides[account.IdentityAccountID32()] = partition
	}
}

func (r *Router) RouteAccount(account *url.URL) (string, error) {
	r.overridesMu.RLock()
	part, ok := r.overrides[account.IdentityAccountID32()]
	r.overridesMu.RUnlock()
	if ok {
		return part, nil
	}
	tree := r.tree.Load()
	if tree == nil {
		return "", errors.InternalError.With("the routing table has not been initialized")
	}
	if protocol.IsUnknown(account) {
		return "", errors.BadRequest.With("URL is unknown, cannot route")
	}
	return tree.Route(account)
}

func (r *Router) Route(envs ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.RouteAccount, envs...)
}
