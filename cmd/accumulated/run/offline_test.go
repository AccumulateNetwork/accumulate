// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// The offline tools (repair-indices, debug db explore, test/validate) read a
// stopped node through its accumulate.toml — the layout `init network` and
// `init node` write — now that no node has the CometBFT layout they used to
// load (#3910). Each partition's store opens where the node keeps it, and the
// node's identity is the one its key gives.
func TestOfflineToolsReadTheNodeConfiguration(t *testing.T) {
	dir := t.TempDir()
	_, nodeKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, valKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	cfg := new(Config)
	cfg.Network = "Offline"
	cfg.P2P = &P2P{Key: &RawPrivateKey{Address: address.FromED25519PrivateKey(nodeKey).String()}}
	cfg.Configurations = []Configuration{&CoreValidatorConfiguration{
		Listen:       multiaddr.StringCast("/ip4/127.0.0.1/tcp/40000"),
		BVN:          "BVN1",
		ValidatorKey: &RawPrivateKey{Address: address.FromED25519PrivateKey(valKey).String()},
		StorageType:  Ptr(StorageTypeLevelDB),
	}}
	file := filepath.Join(dir, "accumulate.toml")
	require.NoError(t, cfg.SaveTo(file))

	for _, part := range []string{"Directory", "BVN1"} {
		store, err := OpenStorage(context.Background(), file, part)
		require.NoError(t, err, part)
		b := store.Begin(nil, true)
		require.NoError(t, b.Put(record.NewKey("test"), []byte(part)), part)
		require.NoError(t, b.Commit(), part)
	}
	_, err = OpenStorage(context.Background(), file, "BVN2")
	require.True(t, errors.Is(err, errors.NotFound), "a partition the node does not run: %v", err)

	network, id, listen, err := NodeIdentity(context.Background(), file)
	require.NoError(t, err)
	require.Equal(t, "Offline", network)
	sk, _, err := crypto.KeyPairFromStdKey(&nodeKey)
	require.NoError(t, err)
	want, err := peer.IDFromPrivateKey(sk)
	require.NoError(t, err)
	require.Equal(t, want, id)
	require.NotEmpty(t, listen, "the node's P2P listen addresses come from its core validator configuration")
}
