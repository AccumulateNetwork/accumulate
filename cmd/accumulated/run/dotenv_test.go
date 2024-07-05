// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	testutil "gitlab.com/accumulatenetwork/accumulate/test/util"
)

func TestDotenv(t *testing.T) {
	// When dot-env is set, ${FOO} is resolved
	t.Run("Set", func(t *testing.T) {
		fs := mkfs(map[string]string{
			".env": `
				FOO=bar`,
			"accumulate.toml": `
				dot-env = true
				network = "${FOO}"`,
		})

		cfg := new(Config)
		require.NoError(t, cfg.LoadFromFS(fs, "accumulate.toml"))
		require.Equal(t, "bar", cfg.Network)
	})

	// When dot-env is unset, ${FOO} is left as is
	t.Run("Unset", func(t *testing.T) {
		fs := mkfs(map[string]string{
			".env": `
				FOO=bar`,
			"accumulate.toml": `
				network = "${FOO}"`,
		})

		cfg := new(Config)
		require.NoError(t, cfg.LoadFromFS(fs, "accumulate.toml"))
		require.Equal(t, "${FOO}", cfg.Network)
	})

	// When dot-env is set, referencing an unset variable ${BAR} is an error
	t.Run("Wrong var", func(t *testing.T) {
		fs := mkfs(map[string]string{
			".env": `
				FOO=bar`,
			"accumulate.toml": `
				dot-env = true
				network = "${BAR}"`,
		})

		cfg := new(Config)
		err := cfg.LoadFromFS(fs, "accumulate.toml")
		require.EqualError(t, err, `"BAR" is not defined`)
	})

	// Variable are resolved exclusively from .env, not from actual environment
	// variables
	t.Run("Ignore env", func(t *testing.T) {
		fs := mkfs(map[string]string{
			"accumulate.toml": `
				dot-env = true
				network = "${FOO}"`,
		})
		require.NoError(t, os.Setenv("FOO", "bar"))

		cfg := new(Config)
		err := cfg.LoadFromFS(fs, "accumulate.toml")
		require.EqualError(t, err, `open .env: file does not exist`)
	})
}

func mkfs(files map[string]string) fs.FS {
	root := new(testutil.Dir)
	for name, data := range files {
		root.Files = append(root.Files, &testutil.File{
			Name: name,
			Data: strings.NewReader(data),
		})
	}
	return root
}
