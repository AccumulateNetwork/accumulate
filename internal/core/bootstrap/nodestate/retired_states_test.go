// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNoProductionCodeNamesARetiredNodeState pins that COMPLETE and WAITING
// are retired (PLAN.md E11 second pass, item 4): no production code outside
// this package names StateComplete, StateWaiting, PromoteToComplete or
// PromoteToWaiting.
func TestNoProductionCodeNamesARetiredNodeState(t *testing.T) {
	retired := map[string]bool{
		"StateComplete":     true,
		"StateWaiting":      true,
		"PromoteToComplete": true,
		"PromoteToWaiting":  true,
	}

	pkgDir, err := os.Getwd()
	require.NoError(t, err)
	root := moduleRoot(t, pkgDir)

	var uses []string
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			switch {
			case path == root:
				return nil
			case path == pkgDir,
				path == filepath.Join(root, "test"),
				name == "vendor", name == "testdata",
				strings.HasPrefix(name, "."), strings.HasPrefix(name, "_"):
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && retired[id.Name] {
				pos := fset.Position(id.Pos())
				rel, _ := filepath.Rel(root, pos.Filename)
				uses = append(uses, fmt.Sprintf("%s:%d: %s", rel, pos.Line, id.Name))
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)

	for _, use := range uses {
		t.Errorf("production code names a retired node state: %s", use)
	}
}

func moduleRoot(t *testing.T, dir string) string {
	t.Helper()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above the package")
		dir = parent
	}
}
