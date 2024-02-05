// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var (
	faucetRefRouter = ioc.Needs[routing.Router](func(f *FaucetService) string { return f.Router.base().refOr("") })
	faucetProvides  = ioc.Provides[v3.Faucet](func(f *FaucetService) string { return f.Account.ShortString() })
)

func (f *FaucetService) Requires() []ioc.Requirement {
	return f.Router.RequiresOr(
		faucetRefRouter.Requirement(f),
	)
}

func (f *FaucetService) Provides() []ioc.Provided { return nil }

func (f *FaucetService) start(inst *Instance) error {
	if f.Account == nil {
		return errors.BadRequest.With("account must be specified")
	}
	if f.SigningKey == nil {
		return errors.BadRequest.With("key must be specified")
	}

	sk, err := getPrivateKey(f.SigningKey, inst)
	if err != nil {
		return err
	}

	var router routing.Router
	if f.Router.base().hasValue() {
		router, err = f.Router.value.create(inst)
	} else {
		router, err = faucetRefRouter.Get(inst.services, f)
	}
	if err != nil {
		return err
	}

	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: inst.config.Network,
			Dialer:  inst.p2p.DialNetwork(),
			Router:  routing.MessageRouter{Router: router},
		},
	}

	impl, err := v3impl.NewFaucet(inst.context, v3impl.FaucetParams{
		Logger:    (*logging.Slogger)(inst.logger.With("module", "faucet")),
		Account:   f.Account,
		Key:       build.ED25519PrivateKey(sk),
		Submitter: client,
		Querier:   client,
		Events:    client,
	})
	if err != nil {
		return err
	}
	inst.cleanup(func() { impl.Stop() })

	registerRpcService(inst, impl.ServiceAddress(), message.Faucet{Faucet: impl})
	return faucetProvides.Register(inst.services, f, impl)
}
