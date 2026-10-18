// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
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

	api, era, err := (&Querier{Partition: "BVN1", Sidecar: fmt.Sprintf("http://127.0.0.1:%d", port)}).open(node)
	require.NoError(t, err)
	got, err := readValue(api, key)
	require.NoError(t, err)
	require.Equal(t, value, got)

	_, err = readValue(live, key)
	require.ErrorIs(t, err, errors.NotFound, "the node's own store has the sidecar's value")

	// The era is the sidecar's era over the live store; this server has no
	// era, so a key only live holds is all it answers
	_, err = readValue(era, key)
	require.ErrorIs(t, err, errors.NotFound)
	liveOnly := record.NewKey("Account", "new.acme", "Main")
	putValue(t, live, liveOnly, value)
	got, err = readValue(era, liveOnly)
	require.NoError(t, err)
	require.Equal(t, value, got)

	// Without the setting, the API reads the node's store alone
	plain, _, err := (&Querier{Partition: "BVN1"}).open(node)
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
