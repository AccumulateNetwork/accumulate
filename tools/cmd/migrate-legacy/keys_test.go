// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
)

// A node that main's `accumulated migrate` converted has an accumulate.toml
// whose keys name CometBFT key files: p2p.key as cometNodeKeyFile and the
// validator key as cometPrivValFile — or no p2p.key at all, which main's core
// validator filled in with {dnn|bvnn}/config/node_key.json. The node reads
// none of them now (#3910); migrate-legacy writes each key inline, and the
// result loads in the node with the same keys.
func TestConvertMainMigratedConfig(t *testing.T) {
	for _, c := range []struct {
		name    string
		p2pKey  string
		nodeKey string // the file the node key must come from
	}{
		{"Named", `
[p2p.key]
type = "cometNodeKeyFile"
path = "dnn/config/node_key.json"
`, "dnn/config/node_key.json"},
		{"Defaulted", ``, "bvnn/config/node_key.json"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCometKey(t, dir, "dnn/config/node_key.json")
			writeCometKey(t, dir, "bvnn/config/node_key.json")
			writeCometKey(t, dir, "priv_validator_key.json")
			file := filepath.Join(dir, "accumulate.toml")
			require.NoError(t, os.WriteFile(file, []byte(`network = "MainNet"
`+c.p2pKey+`
[[configurations]]
type = "coreValidator"
mode = "bvn"
bvn = "Apollo"
listen = "/tcp/16591"

[configurations.validator-key]
type = "cometPrivValFile"
path = "priv_validator_key.json"
`), 0600))

			// Before: the node refuses it, and says what to do.
			err := new(run.Config).LoadFrom(file)
			require.ErrorContains(t, err, "migrate-legacy")

			flagMain.WorkDir = dir
			migrate(nil, nil)

			cfg := new(run.Config)
			require.NoError(t, cfg.LoadFrom(file))
			require.Equal(t, keyAddr(c.nodeKey), cfg.P2P.Key.(*run.RawPrivateKey).Address)
			cvc := cfg.Configurations[0].(*run.CoreValidatorConfiguration)
			require.Equal(t, keyAddr("priv_validator_key.json"), cvc.ValidatorKey.(*run.RawPrivateKey).Address)
			_, err = run.New(context.Background(), cfg)
			require.NoError(t, err, "the node cannot load the converted keys")

			_, err = os.Stat(filepath.Join(dir, "backup", "accumulate.toml"))
			require.NoError(t, err, "the original is kept")
		})
	}
}

func keyAddr(path string) string {
	return address.FromED25519PrivateKey(testKey(path)).String()
}
