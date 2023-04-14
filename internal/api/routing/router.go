// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Router defines a basic interface for routing and sending transactions.
//
// TODO Route and Send should probably be handled separately.
type Router interface {
	RouteAccount(*url.URL) (string, error)
	Route(...*messaging.Envelope) (string, error)
}

// RouterInstance sends transactions to remote nodes via RPC calls.
type RouterInstance struct {
	tree   *RouteTree
	logger logging.OptionalLogger
}

func NewRouter(eventBus *events.Bus, logger log.Logger) *RouterInstance {
	r := new(RouterInstance)
	if logger != nil {
		r.logger.L = logger.With("module", "router")
	}

	events.SubscribeSync(eventBus, func(e events.WillChangeGlobals) error {
		r.logger.Debug("Loading new routing table", "table", e.New.Routing)

		tree, err := NewRouteTree(e.New.Routing)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		r.tree = tree
		return nil
	})

	return r
}

// NewStaticRouter returns a router that uses a static routing table
func NewStaticRouter(table *protocol.RoutingTable, logger log.Logger) (*RouterInstance, error) {
	tree, err := NewRouteTree(table)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	r := new(RouterInstance)
	r.tree = tree
	if logger != nil {
		r.logger.L = logger.With("module", "router")
	}
	return r, nil
}

var _ Router = (*RouterInstance)(nil)

// RouteMessages routes the message bundle based on the routing location of its
// signatures. RouteMessages will return an error if the message bundle does not
// contain any signatures. RouteMessages will return an error if the message
// bundle has signatures with conflicting routing locations.
func RouteMessages(r Router, messages []messaging.Message) (string, error) {
	if len(messages) == 0 {
		return "", errors.BadRequest.With("nothing to route")
	}

	var route string
	for _, msg := range messages {
		err := routeMessage(r.RouteAccount, &route, msg)
		if err != nil {
			return "", errors.UnknownError.With("cannot route message(s): %w", err)
		}
	}
	if route == "" {
		return "", errors.BadRequest.With("nothing to route")
	}

	return route, nil
}

// RouteEnvelopes routes envelopes based on the routing location of their
// signatures. RouteEnvelopes will return an error if the envelopes do not
// contain any signatures. RouteEnvelopes will return an error if the message
// bundle has signatures with conflicting routing locations.
func RouteEnvelopes(routeAccount func(*url.URL) (string, error), envs ...*messaging.Envelope) (string, error) {
	if len(envs) == 0 {
		return "", errors.BadRequest.With("nothing to route")
	}

	var route string
	for _, env := range envs {
		for _, msg := range env.Messages {
			err := routeMessage(routeAccount, &route, msg)
			if err != nil {
				return "", errors.UnknownError.With("cannot route message(s): %w", err)
			}
		}
		for _, sig := range env.Signatures {
			err := routeMessage(routeAccount, &route, &messaging.SignatureMessage{Signature: sig})
			if err != nil {
				return "", errors.UnknownError.With("cannot route message(s): %w", err)
			}
		}
	}
	if route == "" {
		return "", errors.BadRequest.With("nothing to route")
	}

	return route, nil
}

func routeMessage(routeAccount func(*url.URL) (string, error), route *string, msg messaging.Message) error {
	var r string
	var err error
	switch msg := msg.(type) {
	case *messaging.SignatureMessage:
		r, err = routeAccount(msg.Signature.RoutingLocation())

	case *messaging.SequencedMessage:
		r, err = routeAccount(msg.Destination)

	case *messaging.BlockAnchor:
		return routeMessage(routeAccount, route, msg.Anchor)

	default:
		return nil
	}
	if err != nil {
		return err
	}

	if *route == "" {
		*route = r
		return nil
	}

	if *route != r {
		return errors.BadRequest.With("conflicting routes")
	}
	return nil
}

func (r *RouterInstance) RouteAccount(account *url.URL) (string, error) {
	if r.tree == nil {
		return "", errors.InternalError.With("the routing table has not been initialized")
	}
	if protocol.IsUnknown(account) {
		return "", errors.BadRequest.With("URL is unknown, cannot route")
	}

	route, err := r.tree.Route(account)
	if err != nil {
		r.logger.Debug("Failed to route", "account", account, "error", err)
		return "", errors.UnknownError.Wrap(err)
	}

	r.logger.Debug("Routing", "account", account, "to", route)
	return route, nil
}

// Route routes the account using modulo routing.
func (r *RouterInstance) Route(envs ...*messaging.Envelope) (string, error) {
	return RouteEnvelopes(r.RouteAccount, envs...)
}
