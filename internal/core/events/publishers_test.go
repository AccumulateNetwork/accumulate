// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package events_test

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

// willChangeGlobalsBuilders lists the non-test files that may construct a
// WillChangeGlobals. On a bus a partition provides, the membership gate, the
// conductor's anchor gate and the adapter's committee trust the event without
// a signature check, so only executed state may build it.
var willChangeGlobalsBuilders = []string{
	// The executors publish the globals they have just committed.
	"internal/core/execute/",
	"internal/bsn/",
	// The router's fetch publishes a peer's answer, but only on a private bus
	// that InitRouter creates when it is given none (TestInitRouter_BoundBusDoesNotFetch).
	"exp/apiutil/p2p.go",
}

// TestWillChangeGlobalsPublishers fails if code outside willChangeGlobalsBuilders
// constructs a WillChangeGlobals or forwards one it received. The scan is
// syntactic; see scanWillChangeGlobals.
func TestWillChangeGlobalsPublishers(t *testing.T) {
	uses, err := scanWillChangeGlobalsTree(moduleRoot(t))
	require.NoError(t, err)
	require.NotZero(t, len(uses), "the scan found no publishers; it is looking in the wrong place")

	for _, u := range uses {
		if !allowedBuilder(u.path, willChangeGlobalsBuilders) {
			t.Errorf("%s:%d %s; only executed state may publish it on a partition bus (see the type's documentation)", u.path, u.line, u.what)
		}
	}
}

// TestWillChangeGlobalsScan runs the scan over fixtures: a fake module root
// whose allowed paths hold plain publishers and whose other files each use one
// form the scan claims to catch.
func TestWillChangeGlobalsScan(t *testing.T) {
	found, err := scanWillChangeGlobalsPublishers("testdata/publishers", []string{"internal/core/execute/", "internal/bsn/"})
	require.NoError(t, err)

	// A type alias is reported at its declaration and at each use.
	var files []string
	seen := map[string]bool{}
	for _, f := range found {
		f = f[:strings.LastIndex(f, ":")]
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	require.ElementsMatch(t, []string{
		"internal/other/literal.go",
		"internal/other/importalias.go",
		"internal/other/dotimport.go",
		"internal/other/new.go",
		"internal/other/var.go",
		"internal/other/typealias.go",
		"internal/other/typedef.go",
		"internal/other/forwarder.go",
		"internal/other/forwarder_ptr.go",
		"internal/other/funcdecl.go",
		"internal/other/forwarder_local.go",
		"internal/other/forwarder_var.go",
	}, files)
}

// scanWillChangeGlobalsPublishers returns the root-relative path:line of every
// use outside the allowed paths.
func scanWillChangeGlobalsPublishers(root string, allowed []string) ([]string, error) {
	uses, err := scanWillChangeGlobalsTree(root)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, u := range uses {
		if !allowedBuilder(u.path, allowed) {
			found = append(found, fmt.Sprintf("%s:%d", u.path, u.line))
		}
	}
	return found, nil
}

// scanWillChangeGlobalsTree scans the non-test Go files under root, skipping
// .git, node_modules and testdata directories.
func scanWillChangeGlobalsTree(root string) ([]globalsUse, error) {
	var uses []globalsUse
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			// Hidden directories include CI's module cache (.go-mod-cache),
			// which sits inside the checkout and holds other modules' sources.
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch d.Name() {
			case "node_modules", "testdata", "vendor":
				return filepath.SkipDir
			}
			// Another module is not this one's code
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, u := range scanWillChangeGlobals(file) {
			uses = append(uses, globalsUse{path: filepath.ToSlash(rel), line: fset.Position(u.pos).Line, what: u.what})
		}
		return nil
	})
	return uses, err
}

type globalsUse struct {
	pos  token.Pos
	path string
	line int
	what string
}

// scanWillChangeGlobals finds the places a file makes a WillChangeGlobals or
// passes one on: composite literals, new() calls, var declarations, type
// aliases and definitions of the type (which let G{} bypass the other checks),
// and Publish calls that forward a WillChangeGlobals parameter to a bus. It
// resolves the events package under any import name, including a dot import.
func scanWillChangeGlobals(file *ast.File) []globalsUse {
	names := eventsImportNames(file)
	inEvents := file.Name.Name == "events"

	// local holds the types this file declares as WillChangeGlobals, so that
	// G{}, new(G) and a G parameter are caught as well.
	local := map[string]bool{}
	is := func(typ ast.Expr) bool {
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		if id, ok := typ.(*ast.Ident); ok && local[id.Name] {
			return true
		}
		return isWillChangeGlobals(typ, names, inEvents)
	}
	for changed := true; changed; {
		changed = false
		ast.Inspect(file, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok && !local[ts.Name.Name] && is(ts.Type) {
				local[ts.Name.Name] = true
				changed = true
			}
			return true
		})
	}

	// received reports whether e is a parameter (or a copy of one), possibly
	// behind & or *.
	received := func(e ast.Expr, params map[string]bool) bool {
		for {
			switch x := e.(type) {
			case *ast.ParenExpr:
				e = x.X
			case *ast.StarExpr:
				e = x.X
			case *ast.UnaryExpr:
				if x.Op != token.AND {
					return false
				}
				e = x.X
			case *ast.Ident:
				return params[x.Name]
			default:
				return false
			}
		}
	}

	var uses []globalsUse
	add := func(n ast.Node, what string) { uses = append(uses, globalsUse{pos: n.Pos(), what: what}) }

	// forwards flags Publish calls inside a function that pass one of its
	// WillChangeGlobals parameters on.
	forwards := func(typ *ast.FuncType, body *ast.BlockStmt) {
		if body == nil {
			return
		}
		params := map[string]bool{}
		for _, f := range typ.Params.List {
			if is(f.Type) {
				for _, n := range f.Names {
					params[n.Name] = true
				}
			}
		}
		if len(params) == 0 {
			return
		}
		ast.Inspect(body, func(n ast.Node) bool {
			if vs, ok := n.(*ast.ValueSpec); ok && len(vs.Names) == len(vs.Values) {
				for i, r := range vs.Values { // var x = e, a chain of copies included
					if received(r, params) {
						params[vs.Names[i].Name] = true
					}
				}
				return true
			}
			if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
				for i, r := range as.Rhs { // x := e
					if id, ok := as.Lhs[i].(*ast.Ident); ok && received(r, params) {
						params[id.Name] = true
					}
				}
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Publish" {
				return true
			}
			for _, arg := range call.Args {
				if received(arg, params) {
					add(call, "forwards a received events.WillChangeGlobals to a bus")
				}
			}
			return true
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if n.Type != nil && is(n.Type) {
				add(n, "constructs events.WillChangeGlobals")
			}
		case *ast.CallExpr: // new(events.WillChangeGlobals)
			if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "new" && len(n.Args) == 1 && is(n.Args[0]) {
				add(n, "constructs events.WillChangeGlobals")
			}
		case *ast.ValueSpec: // var e events.WillChangeGlobals
			if n.Type != nil && is(n.Type) {
				add(n, "declares an events.WillChangeGlobals")
			}
		case *ast.TypeSpec: // type G = events.WillChangeGlobals
			if is(n.Type) {
				add(n, "names events.WillChangeGlobals as another type")
			}
		case *ast.FuncDecl:
			forwards(n.Type, n.Body)
		case *ast.FuncLit:
			forwards(n.Type, n.Body)
		}
		return true
	})
	return uses
}

// TestWillChangeGlobalsScanSource checks the scan against code it must and must
// not flag.
func TestWillChangeGlobalsScanSource(t *testing.T) {
	const imp = `import events "gitlab.com/accumulatenetwork/accumulate/internal/core/events"` + "\n"
	cases := map[string]struct {
		src  string
		want int
	}{
		"literal":       {`var _ = events.WillChangeGlobals{}`, 1},
		"pointer":       {`var _ = &events.WillChangeGlobals{}`, 1},
		"new":           {`var _ = new(events.WillChangeGlobals)`, 1},
		"var":           {`var e events.WillChangeGlobals`, 1},
		"alias":         {`type G = events.WillChangeGlobals`, 1},
		"definition":    {`type G events.WillChangeGlobals`, 1},
		"alias literal": {"type G = events.WillChangeGlobals\nvar _ = G{}", 2},
		"forwarder": {`func f(other *events.Bus) {
			events.SubscribeSync(other, func(e events.WillChangeGlobals) error { return other.Publish(e) })
		}`, 1},
		"forwarder func": {`func f(bus *events.Bus, e events.WillChangeGlobals) error { return bus.Publish(e) }`, 1},
		"alias use":      {"type G = events.WillChangeGlobals\nvar _ = new(G)", 2},
		"alias of alias": {"type G = events.WillChangeGlobals\ntype H = G\nvar _ = H{}", 3},
		"forwarder ptr":  {`func f(bus *events.Bus, e *events.WillChangeGlobals) error { return bus.Publish(*e) }`, 1},
		"forwarder copy": {`func f(bus *events.Bus, e events.WillChangeGlobals) error { x := e; return bus.Publish(x) }`, 1},
		"subscriber":     {`func f(e events.WillChangeGlobals) error { _ = e.New; return nil }`, 0},
		"other event":    {`func f(bus *events.Bus, e events.DidCommitBlock) error { return bus.Publish(e) }`, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "x.go", "package x\n"+imp+c.src+"\n", 0)
			require.NoError(t, err)
			require.Len(t, scanWillChangeGlobals(file), c.want)
		})
	}

	t.Run("alias import", func(t *testing.T) {
		file, err := parser.ParseFile(token.NewFileSet(), "x.go", `package x
import ev "gitlab.com/accumulatenetwork/accumulate/internal/core/events"
var _ = ev.WillChangeGlobals{}`, 0)
		require.NoError(t, err)
		require.Len(t, scanWillChangeGlobals(file), 1)
	})
}

// eventsImportNames returns the names the file gives the events package. A dot
// import is named ".".
func eventsImportNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != "gitlab.com/accumulatenetwork/accumulate/internal/core/events" {
			continue
		}
		if imp.Name != nil {
			names[imp.Name.Name] = true
		} else {
			names["events"] = true
		}
	}
	return names
}

func isWillChangeGlobals(typ ast.Expr, names map[string]bool, inEvents bool) bool {
	switch typ := typ.(type) {
	case *ast.SelectorExpr:
		pkg, ok := typ.X.(*ast.Ident)
		return ok && names[pkg.Name] && typ.Sel.Name == "WillChangeGlobals"
	case *ast.Ident:
		return (inEvents || names["."]) && typ.Name == "WillChangeGlobals"
	}
	return false
}

func allowedBuilder(rel string, allowed []string) bool {
	for _, a := range allowed {
		if rel == a || (strings.HasSuffix(a, "/") && strings.HasPrefix(rel, a)) {
			return true
		}
	}
	return false
}

func moduleRoot(t *testing.T) string {
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found")
		dir = parent
	}
}
