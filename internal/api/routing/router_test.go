// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestRouteSyntheticMessage(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		router := NewMockRouter(t)
		inner := &messaging.SequencedMessage{Destination: url.MustParse("foo")}
		synth := &messaging.SyntheticMessage{Message: inner}
		router.EXPECT().RouteAccount(inner.Destination).Return("bar", nil)
		_, err := RouteMessages(router, []messaging.Message{synth})
		require.NoError(t, err)
	})

	t.Run("Old", func(t *testing.T) {
		router := NewMockRouter(t)
		inner := &messaging.SequencedMessage{Destination: url.MustParse("foo")}
		synth := &messaging.BadSyntheticMessage{Message: inner}
		router.EXPECT().RouteAccount(inner.Destination).Return("bar", nil)
		_, err := RouteMessages(router, []messaging.Message{synth})
		require.NoError(t, err)
	})
}

// An empty GlobalValues is how a failed initialization is reported: the router
// refuses it and never becomes ready.
func TestRouter_EmptyGlobalsIsNotReady(t *testing.T) {
	bus := events.NewBus(nil)
	r := NewRouter(RouterOptions{Events: bus})

	err := bus.Publish(events.WillChangeGlobals{New: &network.GlobalValues{}})
	require.Error(t, err)
	require.False(t, <-r.Ready())
}

func TestRouter_RoutingTableIsReady(t *testing.T) {
	bus := events.NewBus(nil)
	r := NewRouter(RouterOptions{Events: bus})

	table := &protocol.RoutingTable{Routes: BuildSimpleTable(partitionNames(2))}
	require.NoError(t, bus.Publish(events.WillChangeGlobals{New: &network.GlobalValues{Routing: table}}))
	require.True(t, <-r.Ready())
}
