// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// WHAT PRODUCTION CALLS, AND WHAT IT MUST NOT (#4368 §5, committed under
// #4295).
//
// The spec retired two of the four states: "COMPLETE and WAITING, which named
// a backfilled history, are retired: nothing reached them and nothing could"
// (executor.md, "Sync", step 6). The evidence for "nothing could" was a scan
// of every non-test .go file in the module, which found no production caller
// of PromoteToWaiting, PromoteToComplete, CanServeHistory or Restore — and
// exactly one of New, inside join.NewState.
//
// That scan was written for #4368 and deliberately NOT committed there: one
// of the two candidate resolutions would have added precisely the caller it
// forbids, so committing it would have meant the decision could not be
// implemented without deleting its own test. Resolution (A) was chosen and
// that reason expired, so it is committed here as the regression pin: if a
// retired state comes back, or a caller of one appears, this fails.
//
// It is a text scan and not a type check on purpose. The retired symbols are
// deleted, so a compiler cannot say anything about them; what must be pinned
// is that nobody reintroduces them, which is a fact about the source.
func Test4368WhatProductionCalls(t *testing.T) {
	root := moduleRoot(t)

	// The symbols the spec retired. A production reference to any of them is
	// a regression, whether it compiles or not.
	forbidden := []string{
		"PromoteToWaiting(",
		"PromoteToComplete(",
		"CanServeHistory(",
		"nodestate.Restore(",
		"StateWaiting",
		"StateComplete",
	}

	// The symbols that stay, with the callers they are allowed. The whole
	// machine in production is two promotions and one construction.
	allowed := map[string][]string{
		"nodestate.New(": {
			"internal/node/join/state.go",
		},
		"PromoteToActive(": {
			"internal/core/bootstrap/tracker/tracker.go",
			"internal/node/join/state.go",
			// The definition itself.
			"internal/core/bootstrap/nodestate/nodestate.go",
		},
	}

	found := map[string][]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			code := strings.TrimSpace(line)
			// A mention in a comment is a record of the decision, not a
			// call. Only code counts.
			if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "*") {
				continue
			}
			for _, sym := range append(append([]string{}, forbidden...), keysOf(allowed)...) {
				if strings.Contains(line, sym) {
					found[sym] = append(found[sym], rel+":"+itoa(i+1))
				}
			}
		}
		return nil
	}))

	// Print what was found, the way #4368's statement printed it: the table
	// is the evidence, and a reader of the failure needs it.
	for _, sym := range append(append([]string{}, forbidden...), keysOf(allowed)...) {
		if len(found[sym]) == 0 {
			t.Logf("%-24s NO production caller", sym)
			continue
		}
		for _, where := range found[sym] {
			t.Logf("%-24s %s", sym, where)
		}
	}

	for _, sym := range forbidden {
		require.Empty(t, found[sym],
			"%s is retired by executor.md \"Sync\" step 6 and has a production reference again: %v",
			sym, found[sym])
	}

	for sym, files := range allowed {
		for _, where := range found[sym] {
			file := where[:strings.LastIndex(where, ":")]
			require.Contains(t, files, file,
				"%s gained a production caller the spec does not account for: %s", sym, where)
		}
	}

	// And the two promotions that stay are still there: a scan that finds
	// nothing because the package moved would otherwise pass silently.
	require.NotEmpty(t, found["PromoteToActive("], "the promotion to ACTIVE has no caller at all")
	require.NotEmpty(t, found["nodestate.New("], "nothing constructs a node state machine")
}

func keysOf(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// moduleRoot walks up from this package to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above %s", dir)
		dir = parent
	}
}
