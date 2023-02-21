// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message/mocks"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPrivateMessageRouting(t *testing.T) {
	s := mocks.NewSequencer(t)
	s.EXPECT().Sequence(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(new(api.TransactionRecord), nil)
	c := SetupTest(t, &Sequencer{Sequencer: s})
	c.Router = &routing.MessageRouter{
		Router: routerFunc(func(*url.URL) (string, error) {
			return protocol.Directory, nil
		}),
	}
	_, err := c.Private().Sequence(context.Background(), protocol.DnUrl(), protocol.DnUrl(), 1)
	require.NoError(t, err)
}

type routerFunc func(*url.URL) (string, error)

func (f routerFunc) RouteAccount(u *url.URL) (string, error) {
	return f(u)
}

func (f routerFunc) Route(env ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(f, env...)
}

func (f routerFunc) RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error {
	panic("not implemented")
}

func (f routerFunc) Submit(ctx context.Context, partition string, tx *messaging.Envelope, pretend, async bool) (*routing.ResponseSubmit, error) {
	panic("not implemented")
}