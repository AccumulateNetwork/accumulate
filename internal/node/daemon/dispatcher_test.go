// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated_test

import (
	"context"
	"sync"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestDispatcherPreservesEnvelopeFields verifies that the dispatcher does not
// modify envelopes; if the caller passes transactions and signatures in the
// corresponding fields, the dispatcher does not convert them to messages in the
// messages field.
func TestDispatcherPreservesEnvelopeFields(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	env := helpers.MustBuild(t,
		build.Transaction().For(alice, "tokens").BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	require.Empty(t, env.Messages)

	ch := make(chan message.Stream)
	d := accumulated.NewDispatcher(t.Name(),
		routerFunc(func(u *url.URL) (string, error) { return "foo", nil }),
		dialerFunc(func(ctx context.Context, m multiaddr.Multiaddr) (message.Stream, error) {
			p, q := message.DuplexPipe(ctx)
			ch <- p
			return q, nil
		}),
	)

	wg := new(sync.WaitGroup)
	wg.Add(1)
	require.NoError(t, d.Submit(context.Background(), protocol.AccountUrl("foo"), env))
	go func() {
		defer wg.Done()
		for err := range d.Send(context.Background()) {
			require.NoError(t, err)
		}
	}()

	// Verify the request contains the unmodified envelope
	p := <-ch
	m, err := p.Read()
	require.NoError(t, err)
	require.IsType(t, (*message.Addressed)(nil), m)
	m = m.(*message.Addressed).Message
	require.IsType(t, (*message.SubmitRequest)(nil), m)
	req := m.(*message.SubmitRequest)
	assert.True(t, env.Equal(req.Envelope))
	assert.Empty(t, req.Envelope.Messages)
	assert.NotEmpty(t, req.Envelope.Signatures)
	assert.NotEmpty(t, req.Envelope.Transaction)

	resp := new(message.SubmitResponse)
	require.NoError(t, p.Write(resp))

	wg.Wait()
}

type dialerFunc func(context.Context, multiaddr.Multiaddr) (message.Stream, error)

func (fn dialerFunc) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return fn(ctx, addr)
}

type routerFunc func(*url.URL) (string, error)

func (f routerFunc) RouteAccount(u *url.URL) (string, error) {
	return f(u)
}

func (f routerFunc) Route(env ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(f, env...)
}
