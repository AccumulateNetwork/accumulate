// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// newRawKeyForLogTest returns a raw private key and the strings that must
// never appear in a log: the address (which encodes the private key), and the
// private key and seed in hex and base64.
func newRawKeyForLogTest(t *testing.T) (*RawPrivateKey, *address.PrivateKey, []string) {
	t.Helper()
	sk := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x5a}, ed25519.SeedSize))
	pk := &address.PrivateKey{
		PublicKey: address.PublicKey{Type: protocol.SignatureTypeED25519, Key: sk[32:]},
		Key:       sk,
	}
	s := pk.String()
	require.True(t, strings.HasPrefix(s, "AS1"), "expected a private key address, got %q", s[:3])
	secrets := []string{
		s,
		hex.EncodeToString(sk), strings.ToUpper(hex.EncodeToString(sk)),
		hex.EncodeToString(sk[:32]), strings.ToUpper(hex.EncodeToString(sk[:32])),
		base64.StdEncoding.EncodeToString(sk),
		base64.StdEncoding.EncodeToString(sk[:32]),
	}
	return &RawPrivateKey{Address: s}, pk, secrets
}

func newLogCapture() (*Instance, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	inst := new(Instance)
	inst.logger = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return inst, buf
}

func requireNoSecrets(t *testing.T, log string, secrets []string) {
	t.Helper()
	for _, s := range secrets {
		require.NotContains(t, log, s, "key material written to the log")
	}
	// A truncated copy of the address must not appear either.
	require.NotContains(t, log, secrets[0][:len(secrets[0])-8], "key material written to the log")
}

// TestRawKeyLoadDoesNotLogKey is the regression test for #4494.
func TestRawKeyLoadDoesNotLogKey(t *testing.T) {
	key, pk, secrets := newRawKeyForLogTest(t)
	inst, buf := newLogCapture()

	addr, err := key.get(inst)
	require.NoError(t, err)
	got, ok := addr.GetPrivateKey()
	require.True(t, ok)
	require.Equal(t, []byte(pk.Key), got)

	log := buf.String()
	require.NotEmpty(t, log, "expected the key load to be logged")
	requireNoSecrets(t, log, secrets)

	// The log identifies the key by its public key hash.
	hash, ok := pk.GetPublicKeyHash()
	require.True(t, ok)
	require.Contains(t, log, hex.EncodeToString(hash))
}

// TestRawKeyParseErrorDoesNotLogKey checks the failure path: a key that does
// not parse must not be echoed into the log or the error either.
func TestRawKeyParseErrorDoesNotLogKey(t *testing.T) {
	key, _, secrets := newRawKeyForLogTest(t)

	// Corrupt the last character so the checksum fails.
	repl := byte('2')
	if key.Address[len(key.Address)-1] == repl {
		repl = '3'
	}
	bad := key.Address[:len(key.Address)-1] + string(repl)
	inst, buf := newLogCapture()
	_, err := (&RawPrivateKey{Address: bad}).get(inst)
	require.Error(t, err)

	log := buf.String()
	require.NotEmpty(t, log)
	requireNoSecrets(t, log, append(secrets, bad))
	require.NotContains(t, err.Error(), bad[:len(bad)-8])
}

// TestGeneratedFollowerKeyDoesNotLogKey covers defaultNodeKey, which
// generates a follower's node key, writes it into the configuration file and
// then loads it: neither step may put the key into the log.
func TestGeneratedFollowerKeyDoesNotLogKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "accumulate.toml")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	cfg := &Config{
		file:           file,
		P2P:            new(P2P),
		Configurations: []Configuration{new(FollowerConfiguration)},
	}
	inst, buf := newLogCapture()
	require.NoError(t, defaultNodeKey(cfg, inst))

	key, ok := cfg.P2P.Key.(*RawPrivateKey)
	require.True(t, ok, "expected a raw key, got %T", cfg.P2P.Key)
	_, err := key.get(inst)
	require.NoError(t, err)

	addr, err := address.Parse(key.Address)
	require.NoError(t, err)
	sk, ok := addr.GetPrivateKey()
	require.True(t, ok)
	log := buf.String()
	require.NotEmpty(t, log)
	requireNoSecrets(t, log, []string{
		key.Address,
		hex.EncodeToString(sk), strings.ToUpper(hex.EncodeToString(sk)),
		hex.EncodeToString(sk[:32]), strings.ToUpper(hex.EncodeToString(sk[:32])),
		base64.StdEncoding.EncodeToString(sk),
		base64.StdEncoding.EncodeToString(sk[:32]),
	})
}
