// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var (
	faucetRefRouter = ioc.Needs[routing.Router](func(f *FaucetService) string { return f.Router.base().refOr("") })
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

	go func() {
		if r, ok := router.(*routing.RouterInstance); ok {
			inst.logger.Info("Waiting for router", "module", "run", "service", "faucet")
			<-r.Ready()
		}

		for {
			_, err := api.Querier2{Querier: client}.QueryAccount(inst.context, f.Account, nil)
			switch {
			case err == nil:
				// Ok
			case errors.Is(err, errors.NoPeer):
				time.Sleep(time.Second)
				continue
			default:
				inst.logger.Error("Failed to start faucet", "error", err)
				inst.Stop()
				return
			}
			break
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
			inst.logger.Error("Failed to start faucet", "error", err)
			inst.Stop()
			return
		}

		inst.cleanup(func(context.Context) error { impl.Stop(); return nil })
		registerRpcService(inst, impl.ServiceAddress(), message.Faucet{Faucet: impl})
		inst.logger.Info("Ready", "module", "run", "service", "faucet")
	}()
	return nil
}
