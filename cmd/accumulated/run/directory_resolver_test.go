// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type fakeViewer struct{ name string }

func (fakeViewer) View(func(*database.Batch) error) error { return nil }

// The directory resolves to its own database without a lookup at all.
func TestDirectoryResolver_DirectoryIsItself(t *testing.T) {
	own := &fakeViewer{"own"}
	var opened int
	r := newDirectoryResolver(protocol.Directory, own, func() (database.Viewer, error) {
		opened++
		return nil, errors.New("must not be called")
	})

	got, err := r()
	require.NoError(t, err)
	require.Same(t, own, got)
	require.Zero(t, opened, "the directory must not look itself up")
}

// The regression this exists for: a call made before the directory has
// registered its storage must not poison every later call. Caching the failure
// is exactly the start-order problem the deferral was added to avoid.
func TestDirectoryResolver_FailureIsNotCached(t *testing.T) {
	dn := &fakeViewer{"dn"}
	var opened int
	ready := false
	r := newDirectoryResolver("BVN1", &fakeViewer{"own"}, func() (database.Viewer, error) {
		opened++
		if !ready {
			return nil, errors.New("not registered yet")
		}
		return dn, nil
	})

	// Asked too early, twice
	_, err := r()
	require.Error(t, err)
	_, err = r()
	require.Error(t, err)
	require.Equal(t, 2, opened, "a failed lookup must be retried, not remembered")

	// The directory comes up
	ready = true
	got, err := r()
	require.NoError(t, err)
	require.Same(t, dn, got, "the resolver must recover once the directory is up")
}

// Success is cached, so the steady state is not a lookup per proof.
func TestDirectoryResolver_SuccessIsCached(t *testing.T) {
	dn := &fakeViewer{"dn"}
	var opened int
	r := newDirectoryResolver("BVN1", &fakeViewer{"own"}, func() (database.Viewer, error) {
		opened++
		return dn, nil
	})

	for i := 0; i < 5; i++ {
		got, err := r()
		require.NoError(t, err)
		require.Same(t, dn, got)
	}
	require.Equal(t, 1, opened)
}

// Proof calls are concurrent, so the resolver is too.
func TestDirectoryResolver_ConcurrentCallsOpenOnce(t *testing.T) {
	dn := &fakeViewer{"dn"}
	var mu sync.Mutex
	var opened int
	r := newDirectoryResolver("BVN1", &fakeViewer{"own"}, func() (database.Viewer, error) {
		mu.Lock()
		defer mu.Unlock()
		opened++
		return dn, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := r()
			require.NoError(t, err)
			require.Same(t, dn, got)
		}()
	}
	wg.Wait()
	require.Equal(t, 1, opened)
}
