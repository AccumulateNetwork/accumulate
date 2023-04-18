// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/bsn"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (d *Daemon) startCollector() error {
	// Collect block summaries
	_, err := bsn.StartCollector(bsn.CollectorOptions{
		Partition: d.Config.Accumulate.PartitionId,
		Database:  d.db,
		Events:    d.eventBus,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("start collector: %w", err)
	}

	client := &message.Client{Transport: &message.RoutedTransport{
		Network: d.Config.Accumulate.Network.Id,
		Dialer:  d.p2pnode.DialNetwork(),
		Router:  routing.MessageRouter{Router: summaryRouter(d.Config.Accumulate.SummaryNetwork)},
	}}

	events.SubscribeAsync(d.eventBus, func(e bsn.DidCollectBlock) {
		/*// Wait for the summary network to appear
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		err := d.p2pnode.WaitForService(ctx, api.ServiceTypeSubmit.AddressFor(d.Config.Accumulate.SummaryNetwork).Multiaddr())
		if err != nil {
			d.Logger.Error("Summary network did not appear after an hour", "error", err)
			return
		}//*/

		env, err := build.SignatureForMessage(e.Summary).
			Url(protocol.PartitionUrl(d.Config.Accumulate.PartitionId)).
			PrivateKey(d.privVal.Key.PrivKey.Bytes()).
			Done()
		if err != nil {
			d.Logger.Error("Failed to sign block summary", "error", err)
			return
		}

		msg := new(messaging.BlockAnchor)
		msg.Anchor = e.Summary
		msg.Signature = env.Signatures[0].(protocol.KeySignature)

		sub, err := client.Submit(context.Background(), &messaging.Envelope{Messages: []messaging.Message{msg}}, v3.SubmitOptions{})
		if err != nil {
			d.Logger.Error("Failed to submit block summary envelope", "error", err)
			return
		}
		for _, sub := range sub {
			if !sub.Success {
				d.Logger.Error("Block summary envelope failed", "error", sub.Message)
			}
		}
	})
	return nil
}

type summaryRouter string

func (r summaryRouter) RouteAccount(*url.URL) (string, error) {
	return string(r), nil
}

func (r summaryRouter) Route(env ...*messaging.Envelope) (string, error) {
	return string(r), nil
}
