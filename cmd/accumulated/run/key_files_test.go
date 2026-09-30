// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
)

// The node and validator key files a node holds on disk — node_key.json and
// priv_validator_key.json, in the layout CometBFT defined — are read by every
// build. There is one build (#3910); the DAG-BFT-tagged one used to refuse
// them outright.
func TestKeyFilesOnDiskAreRead(t *testing.T) {
	dir := t.TempDir()
	_, sk, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	file := fmt.Sprintf(`{"priv_key":{"type":"tendermint/PrivKeyEd25519","value":%q}}`, base64.StdEncoding.EncodeToString(sk))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_key.json"), []byte(file), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "priv_validator_key.json"), []byte(file), 0600))

	inst := &Instance{rootDir: dir, context: context.Background(), logger: slog.Default()}
	for _, k := range []interface {
		get(*Instance) (address.Address, error)
	}{
		&CometNodeKeyFile{Path: "node_key.json"},
		&CometPrivValFile{Path: "priv_validator_key.json"},
	} {
		got, err := k.get(inst)
		require.NoError(t, err)
		pk, ok := got.GetPrivateKey()
		require.True(t, ok)
		require.Equal(t, []byte(sk), pk)
	}
}
