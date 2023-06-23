// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// GenesisTime is 2022-7-1 0:00 UTC.
var GenesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

// NewSim creates a simulator with the given database, network initialization,
// and snapshot function and calls NewSimWith.
func NewSim(tb testing.TB, database simulator.OpenDatabaseFunc, init *accumulated.NetworkInit, snapshot simulator.SnapshotFunc) *Sim {
	s, err := simulator.New2(acctesting.NewTestLogger(tb), database, init, snapshot, Recordings(tb))
	require.NoError(tb, err)
	return NewSimWith(tb, s)
}

func Recordings(tb testing.TB) simulator.RecordingFunc {
	onFail := os.Getenv("RECORD_FAILURE")
	if onFail == "" {
		return nil
	}

	require.NoError(tb, os.MkdirAll(onFail, 0755))

	prefix := sha256.Sum256([]byte(tb.Name()))
	var didAnnounce bool

	dir := tb.TempDir()
	return func(partition string, node int) (io.WriteSeeker, error) {
		file := filepath.Join(dir, fmt.Sprintf("%s-%d.record", partition, node))
		f, err := os.Create(file)
		if err != nil {
			return nil, err
		}
		tb.Cleanup(func() {
			if !tb.Failed() {
				assert.NoError(tb, f.Close())
				return
			}
			if !didAnnounce {
				tb.Logf("Failure recordings for %s are prefixed with %x", tb.Name(), prefix)
				didAnnounce = true
			}
			dst := filepath.Join(onFail, fmt.Sprintf("%x-%s-%d.record", prefix, partition, node))
			g, err := os.Create(dst)
			if !assert.NoError(tb, err) {
				return
			}
			_, err = f.Seek(0, io.SeekStart)
			if !assert.NoError(tb, err) {
				return
			}
			_, err = io.Copy(g, f)
			if !assert.NoError(tb, err) {
				return
			}
			assert.NoError(tb, f.Close())
			assert.NoError(tb, g.Close())
		})
		return f, nil
	}
}

// NewSimWith creates a Harness for the given simulator instance and wraps it as
// a Sim.
func NewSimWith(tb testing.TB, s *simulator.Simulator) *Sim {
	return &Sim{*New(tb, s.Services(), s), s}
}

// Sim is a Harness with some extra simulator-specific features.
type Sim struct {
	Harness
	S *simulator.Simulator
}

// Router calls Simulator.Router.
func (s *Sim) Router() *simulator.Router {
	return s.S.Router()
}

// Partitions calls Simulator.Partitions.
func (s *Sim) Partitions() []*protocol.PartitionInfo {
	return s.S.Partitions()
}

// Database calls Simulator.Database.
func (s *Sim) Database(partition string) database.Updater {
	return s.S.Database(partition)
}

// DatabaseFor calls Simulator.DatabaseFor.
func (s *Sim) DatabaseFor(account *url.URL) database.Updater {
	return s.S.DatabaseFor(account)
}

// SetRoute calls Simulator.SetRoute.
func (s *Sim) SetRoute(account *url.URL, partition string) {
	s.S.SetRoute(account, partition)
}

// SetSubmitHook calls Simulator.SetSubmitHook.
func (s *Sim) SetSubmitHook(partition string, fn simulator.SubmitHookFunc) {
	s.S.SetSubmitHook(partition, fn)
}

// SetSubmitHookFor calls Simulator.SetSubmitHookFor.
func (s *Sim) SetSubmitHookFor(account *url.URL, fn simulator.SubmitHookFunc) {
	s.S.SetSubmitHookFor(account, fn)
}

// SetBlockHook calls Simulator.SetBlockHook.
func (s *Sim) SetBlockHook(partition string, fn simulator.BlockHookFunc) {
	s.S.SetBlockHook(partition, fn)
}

// SetBlockHookFor calls Simulator.SetBlockHookFor.
func (s *Sim) SetBlockHookFor(account *url.URL, fn simulator.BlockHookFunc) {
	s.S.SetBlockHookFor(account, fn)
}

// SetNodeBlockHook calls Simulator.SetNodeBlockHook.
func (s *Sim) SetNodeBlockHook(partition string, fn simulator.NodeBlockHookFunc) {
	s.S.SetNodeBlockHook(partition, fn)
}

// SetNodeBlockHookFor calls Simulator.SetNodeBlockHookFor.
func (s *Sim) SetNodeBlockHookFor(account *url.URL, fn simulator.NodeBlockHookFunc) {
	s.S.SetNodeBlockHookFor(account, fn)
}

// SignWithNode calls Simulator.SignWithNode.
func (s *Sim) SignWithNode(partition string, i int) signing.Signer {
	return s.S.SignWithNode(partition, i)
}

func (s *Sim) SubmitTo(partition string, envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	return s.S.SubmitTo(partition, envelope)
}
