// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/block"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// #4561: one node serves the sidecar; another node's API reads through to it,
// and that node's own store, the one its executor reads, never has the value.
func TestQuerierReadsTheSidecarFromAnotherNode(t *testing.T) {
	key, value := record.NewKey("Account", "old.acme", "Main"), []byte("pre-reorg")

	// The serving node
	server := newTestInstance(t)
	server.logger = slog.Default()
	side, err := leveldb.Open(filepath.Join(server.rootDir, "sidecar"))
	require.NoError(t, err)
	putValue(t, side, key, value)
	require.NoError(t, side.Close())

	port := freePort(t)
	svc := &SidecarService{Path: "sidecar", Listen: []multiaddr.Multiaddr{multiaddr.StringCast(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/http", port))}}
	require.NoError(t, svc.start(server))

	// The reading node
	node := newTestInstance(t)
	node.logger = slog.Default()
	node.services = ioc.Registry{}
	live := memory.New(nil)
	require.NoError(t, storageProvides.Register(node.services, &StorageService{Name: "BVN1"}, live))

	api, err := (&Querier{Partition: "BVN1", Sidecar: fmt.Sprintf("http://127.0.0.1:%d", port)}).open(node)
	require.NoError(t, err)
	got, err := readValue(api, key)
	require.NoError(t, err)
	require.Equal(t, value, got)

	_, err = readValue(live, key)
	require.ErrorIs(t, err, errors.NotFound, "the node's own store has the sidecar's value")

	// Without the setting, the API reads the node's store alone
	plain, err := (&Querier{Partition: "BVN1"}).open(node)
	require.NoError(t, err)
	_, err = readValue(plain, key)
	require.ErrorIs(t, err, errors.NotFound)
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func putValue(t *testing.T, db keyvalue.Beginner, key *record.Key, value []byte) {
	b := db.Begin(nil, true)
	defer b.Discard()
	require.NoError(t, b.Put(key, value))
	require.NoError(t, b.Commit())
}

func readValue(db keyvalue.Beginner, key *record.Key) ([]byte, error) {
	b := db.Begin(nil, false)
	defer b.Discard()
	return b.Get(key)
}

// The sidecar service serves a sidecar of either backend, selected by
// StorageType, opens it read-only, and does not create one that is missing.
func TestSidecarServiceBackend(t *testing.T) {
	key, value := record.NewKey("Account", "old.acme", "Main"), []byte("pre-reorg")
	get := func(t *testing.T, port int) (int, string) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/key/%x", port, func() [32]byte { h := key.Hash(); return h }()))
		require.NoError(t, err)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(b)
	}
	serve := func(t *testing.T, inst *Instance, svc *SidecarService) int {
		port := freePort(t)
		svc.Listen = []multiaddr.Multiaddr{multiaddr.StringCast(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/http", port))}
		require.NoError(t, svc.start(inst))
		return port
	}
	blockType, levelType := StorageTypeExpBlockDB, StorageTypeLevelDB

	t.Run("block", func(t *testing.T) {
		inst := newTestInstance(t)
		inst.logger = slog.Default()
		db, err := block.Open(filepath.Join(inst.rootDir, "sidecar"))
		require.NoError(t, err)
		putValue(t, db, key, value)
		require.NoError(t, db.Close())

		port := serve(t, inst, &SidecarService{Path: "sidecar", StorageType: &blockType})
		code, body := get(t, port)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, string(value), body)
	})

	t.Run("leveldb explicit", func(t *testing.T) {
		inst := newTestInstance(t)
		inst.logger = slog.Default()
		db, err := leveldb.Open(filepath.Join(inst.rootDir, "sidecar"))
		require.NoError(t, err)
		putValue(t, db, key, value)
		require.NoError(t, db.Close())

		port := serve(t, inst, &SidecarService{Path: "sidecar", StorageType: &levelType})
		code, body := get(t, port)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, string(value), body)
	})

	t.Run("missing is not created", func(t *testing.T) {
		for _, typ := range []*StorageType{nil, &blockType} {
			inst := newTestInstance(t)
			inst.logger = slog.Default()
			svc := &SidecarService{Path: "nothing", StorageType: typ, Listen: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0/http")}}
			require.Error(t, svc.start(inst))
			_, err := os.Stat(filepath.Join(inst.rootDir, "nothing"))
			require.True(t, os.IsNotExist(err), "serving created the sidecar")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		bolt := StorageTypeBolt
		inst := newTestInstance(t)
		inst.logger = slog.Default()
		svc := &SidecarService{Path: "x", StorageType: &bolt, Listen: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0/http")}}
		require.ErrorContains(t, svc.start(inst), "unsupported sidecar storage type")
	})
}
