// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package apiutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// recordStarts replaces the background fetch with a recorder.
func recordStarts(t *testing.T) *[]RouterOptions {
	var started []RouterOptions
	old := startInitRouter
	startInitRouter = func(opts RouterOptions) { started = append(started, opts) }
	t.Cleanup(func() { startInitRouter = old })
	return &started
}

// A bus the caller provides is the router's only source of routing: InitRouter
// must not fetch a table from a peer and publish it there, because on a
// partition's bus WillChangeGlobals is trusted as executed state.
func TestInitRouter_BoundBusDoesNotFetch(t *testing.T) {
	started := recordStarts(t)

	bus := events.NewBus(nil)
	var published int
	events.SubscribeSync(bus, func(events.WillChangeGlobals) error { published++; return nil })

	r, err := InitRouter(RouterOptions{Events: bus})
	require.NoError(t, err)
	require.Empty(t, *started, "InitRouter fetched on a bound bus")
	require.Zero(t, published)
	ready := r.(*routing.RouterInstance).Ready()
	select {
	case <-ready:
		t.Fatal("router is ready before any globals were published")
	default:
	}

	table := &protocol.RoutingTable{Routes: routing.BuildSimpleTable([]string{"a", "b"})}
	require.NoError(t, bus.Publish(events.WillChangeGlobals{New: &network.GlobalValues{Routing: table}}))
	require.True(t, <-ready)
}

func TestInitRouter_UnboundBusFetchesOnPrivateBus(t *testing.T) {
	started := recordStarts(t)

	r, err := InitRouter(RouterOptions{})
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Len(t, *started, 1)
	require.NotNil(t, (*started)[0].Events)
}

func TestPublishInitFailure_RouterIsNotReady(t *testing.T) {
	bus := events.NewBus(nil)
	r := routing.NewRouter(routing.RouterOptions{Events: bus})
	publishInitFailure(RouterOptions{Events: bus})
	require.False(t, <-r.Ready())
}
