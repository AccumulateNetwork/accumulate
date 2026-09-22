// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"fmt"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTheJoinAsksNoPeerForItsConclusions holds the E11 second-pass gate: the
// staging API, the take-staging path, the execute-from-own-state branch and
// the pull's meeting point are gone, not bypassed. A join settles on blocks it
// collected and state it proved; it never takes a peer's staging, and it never
// starts executing because no peer had any.
//
// It scans the identifiers (not comments or strings) of every non-test .go
// file of this module. Directories the go tool ignores (vendor, testdata,
// names starting with . or _) and nested modules are not part of the module's
// build and are skipped. Generated *_gen.go files are NOT skipped: a generated
// staging API is still a staging API.
func TestTheJoinAsksNoPeerForItsConclusions(t *testing.T) {
	root := moduleRoot(t)

	// Identifiers that must not exist anywhere in the module.
	banned := []string{
		"takeStaging",
		"NoPeerHasStaging",
		"ErrNoPeerHasStaging",
		"executeFromOwnState",
		"LoadStaging",
	}

	// Under the state pull, the meeting point is the type `meeting` and
	// whatever is named for it; any identifier containing the fragment.
	pullDir := filepath.Join(root, "internal", "core", "bootstrap", "pull")

	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root {
				if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		inPull := filepath.Dir(path) == pullDir

		fset := token.NewFileSet()
		file := fset.AddFile(path, -1, len(src))
		var s scanner.Scanner
		s.Init(file, src, nil, 0)
		for {
			pos, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if tok != token.IDENT {
				continue
			}
			line := fset.Position(pos).Line
			for _, b := range banned {
				if lit == b {
					found = append(found, fmt.Sprintf("%s:%d: %s", rel, line, lit))
				}
			}
			if inPull && strings.Contains(strings.ToLower(lit), "meeting") {
				found = append(found, fmt.Sprintf("%s:%d: %s (meeting point)", rel, line, lit))
			}
		}
		return nil
	})
	require.NoError(t, err)

	for _, f := range found {
		t.Errorf("still present, not gone: %s", f)
	}
}

// moduleRoot walks up from the package directory to the go.mod that declares
// this module.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.HasPrefix(string(mod), "module gitlab.com/accumulatenetwork/accumulate\n") {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "module root not found")
		dir = parent
	}
}
