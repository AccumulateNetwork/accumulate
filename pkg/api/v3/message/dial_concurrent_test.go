// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"sync"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	mocks "gitlab.com/accumulatenetwork/accumulate/test/mocks/pkg/api/v3"
)

// TestBatchDialerConcurrent tests that BatchDialer handles concurrent access correctly without panics.
func TestBatchDialerConcurrent(t *testing.T) {
	// Set up the mock and expected return value
	expect := &api.UrlRecord{Value: protocol.AccountUrl("foo")}
	s := mocks.NewQuerier(t)
	s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil).Maybe()

	// Set up the service and handler
	handler, err := NewHandler(Querier{Querier: s})
	require.NoError(t, err)

	// Create a dialer that tracks concurrent calls
	var dialCount int
	var dialMu sync.Mutex
	dialer := dialerFunc(func(ctx context.Context, addr multiaddr.Multiaddr) (Stream, error) {
		dialMu.Lock()
		dialCount++
		dialMu.Unlock()

		// Create a pipe stream and handle it
		s := Pipe(ctx)
		go func() {
			defer s.Close()
			handler.Handle(s)
		}()
		return s, nil
	})

	// Create a batch dialer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batch := BatchDialer(ctx, dialer)

	// Create the test address
	addr, err := api.ServiceTypeQuery.AddressFor("foo").MultiaddrFor("foo")
	require.NoError(t, err)

	// Run multiple concurrent Dial calls - this tests that there are no
	// concurrent map access panics
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := batch.Dial(ctx, addr)
			require.NoError(t, err)
		}()
	}

	wg.Wait()

	// Verify that we dialed at least once and at most numGoroutines times
	// (due to race conditions, multiple goroutines may dial before the cache is populated)
	dialMu.Lock()
	count := dialCount
	dialMu.Unlock()

	require.Greater(t, count, 0, "Expected at least one dial call")
	require.LessOrEqual(t, count, numGoroutines, "Expected at most %d dial calls", numGoroutines)
}

// TestBatchDialerConcurrentBadDial tests that BadDial handles concurrent access correctly.
func TestBatchDialerConcurrentBadDial(t *testing.T) {
	// Create a dialer that tracks concurrent calls
	var dialCount int
	var dialMu sync.Mutex
	dialer := dialerFunc(func(ctx context.Context, addr multiaddr.Multiaddr) (Stream, error) {
		dialMu.Lock()
		dialCount++
		dialMu.Unlock()
		return mockStream{}, nil
	})

	// Create a batch dialer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batch := BatchDialer(ctx, dialer)
	multi, ok := batch.(MultiDialer)
	require.True(t, ok, "BatchDialer should implement MultiDialer")

	// Create the test address
	addr, err := api.ServiceTypeQuery.AddressFor("foo").MultiaddrFor("foo")
	require.NoError(t, err)

	// First dial to populate the cache
	s, err := batch.Dial(ctx, addr)
	require.NoError(t, err)

	// Run multiple concurrent BadDial calls
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			multi.BadDial(ctx, addr, s, nil)
		}()
	}

	wg.Wait()

	// Verify no panics occurred (the test would fail if there was a concurrent map write)
}

type mockStream struct{}

func (mockStream) Read() (Message, error)   { return nil, nil }
func (mockStream) Write(Message) error      { return nil }
func (mockStream) Close() error             { return nil }
func (mockStream) Context() context.Context { return context.Background() }
