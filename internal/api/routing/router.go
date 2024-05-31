// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"sync"

	"github.com/cometbft/cometbft/libs/log"
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
	readyCh chan struct{}
	readyOk bool
	tree    *RouteTree
	logger  logging.OptionalLogger
}

type RouterOptions struct {
	Initial *protocol.RoutingTable
	Events  *events.Bus
	Logger  log.Logger
}

// NewRouter constructs a new router. If an initial routing table is provided,
// the router will be initialized with that table. If an event bus is provided,
// the router will listen for changes. If no event bus is provided, the router
// will be static.
//
// NewRouter will panic if the initial routing table is invalid. NewRouter will
// panic unless an initial routing table and/or event bus are specified.
func NewRouter(opts RouterOptions) *RouterInstance {
	r := new(RouterInstance)
	r.readyCh = make(chan struct{})
	r.logger.Set(opts.Logger, "module", "router")

	var readyOnce sync.Once
	markReady := func(ok bool) {
		readyOnce.Do(func() {
			r.readyOk = ok
			close(r.readyCh)
		})
	}

	var ok bool
	if opts.Initial != nil {
		ok = true
		tree, err := NewRouteTree(opts.Initial)
		if err != nil {
			panic(err)
		}
		r.tree = tree
		markReady(true)
	}

	if opts.Events != nil {
		ok = true
		events.SubscribeSync(opts.Events, func(e events.WillChangeGlobals) error {
			if e.New.Routing == nil {
				markReady(false)
				return errors.InternalError.With("missing routing table")
			}

			r.logger.Debug("Loading new routing table", "table", e.New.Routing)

			tree, err := NewRouteTree(e.New.Routing)
			if err != nil {
				markReady(false)
				return errors.UnknownError.Wrap(err)
			}

			r.tree = tree
			markReady(true)
			return nil
		})
	}

	if !ok {
		panic("must have an initial routing table or an event bus")
	}

	return r
}

var _ Router = (*RouterInstance)(nil)

// RouteMessages routes the message bundle based on the routing location of its
// signatures. RouteMessages will return an error if the message bundle does not
// contain any signatures. RouteMessages will return an error if the message
// bundle has signatures with conflicting routing locations.
func RouteMessages(r Router, messages []messaging.Message) (string, error) {
	if len(messages) == 0 {
		return "", errors.BadRequest.With("nothing to route (1)")
	}

	var route string
	for _, msg := range messages {
		err := routeMessage(r.RouteAccount, &route, msg)
		if err != nil {
			return "", errors.UnknownError.WithFormat("cannot route message(s): %w", err)
		}
	}
	if route == "" {
		return "", errors.BadRequest.With("nothing to route (2)")
	}

	return route, nil
}

// RouteEnvelopes routes envelopes based on the routing location of their
// signatures. RouteEnvelopes will return an error if the envelopes do not
// contain any signatures. RouteEnvelopes will return an error if the message
// bundle has signatures with conflicting routing locations.
func RouteEnvelopes(routeAccount func(*url.URL) (string, error), envs ...*messaging.Envelope) (string, error) {
	if len(envs) == 0 {
		return "", errors.BadRequest.With("nothing to route (3)")
	}

	var route string
	for _, env := range envs {
		for _, msg := range env.Messages {
			err := routeMessage(routeAccount, &route, msg)
			if err != nil {
				return "", errors.UnknownError.WithFormat("cannot route message(s): %w", err)
			}
		}
		for _, sig := range env.Signatures {
			err := routeMessage(routeAccount, &route, &messaging.SignatureMessage{Signature: sig})
			if err != nil {
				return "", errors.UnknownError.WithFormat("cannot route message(s): %w", err)
			}
			if sig.Type() == protocol.SignatureTypePartition {
				break
			}
		}
	}
	if route == "" {
		return "", errors.BadRequest.With("nothing to route (4)")
	}

	return route, nil
}

func routeMessage(routeAccount func(*url.URL) (string, error), route *string, msg messaging.Message) error {
	var r string
	var err error
	switch msg := msg.(type) {
	case *messaging.SignatureMessage:
		switch sig := msg.Signature.(type) {
		case *protocol.ReceiptSignature, *protocol.InternalSignature:
			return nil
		case protocol.KeySignature:
			if protocol.DnUrl().ParentOf(sig.GetSigner()) {
				return nil
			}
		}
		r, err = routeAccount(msg.Signature.RoutingLocation())

	case *messaging.SequencedMessage:
		r, err = routeAccount(msg.Destination)

	case *messaging.BlockAnchor:
		return routeMessage(routeAccount, route, msg.Anchor)

	case interface{ Unwrap() messaging.Message }:
		return routeMessage(routeAccount, route, msg.Unwrap())

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

func (r *RouterInstance) Ready() <-chan bool {
	ch := make(chan bool, 1)
	go func() {
		<-r.readyCh
		ch <- r.readyOk
	}()
	return ch
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
