// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"
	"math/big"
	"sync"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type staticRouter struct{}

func (staticRouter) RouteAccount(u *url.URL) (string, error) { return "bvn1", nil }
func (staticRouter) Route(env ...*messaging.Envelope) (string, error) {
	return "bvn1", nil
}

// TestDispatcherConcurrentSubmit reproduces #4067: the conductor submits from
// concurrent background tasks (anchor healing per destination + synthetic
// healing) while the per-block flush swaps the queue. Run with -race; before
// the queue mutex this was a fatal concurrent map write that crashed four soak
// validators.
func TestDispatcherConcurrentSubmit(t *testing.T) {
	d := &dispatcher{router: staticRouter{}, queue: map[string][]*messaging.Envelope{}}
	// Each submitter uses its own envelopes, as the conductor's tasks do.
	newEnv := func() *messaging.Envelope {
		return &messaging.Envelope{Messages: []messaging.Message{
			&messaging.TransactionMessage{Transaction: &protocol.Transaction{
				Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("foo")},
				Body:   &protocol.SendTokens{To: []*protocol.TokenRecipient{{Url: protocol.AccountUrl("bar"), Amount: *big.NewInt(1)}}},
			}},
		}}
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = d.Submit(context.Background(), protocol.AccountUrl("foo"), newEnv())
			}
		}()
	}
	// Flush concurrently, like the conductor's deferred per-block Send
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			d.mu.Lock()
			d.queue = map[string][]*messaging.Envelope{}
			d.mu.Unlock()
		}
	}()
	wg.Wait()
}
