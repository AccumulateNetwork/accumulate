// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func main() {
	// Make the trace directory
	check(os.MkdirAll("trace", 0700))

	args := os.Args[1:]
	var pkg string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] == '.' {
		pkg, args = args[0], args[1:]
	}
	switch pkg {
	case "", "./...":
		// List all packages that contain test files
		cmd := exec.Command("go", "list", "-f", "{{if .TestGoFiles}}{{.ImportPath}}{{end}}", "./...")
		out, err := cmd.CombinedOutput()
		check(err)

		// Test each one
		for _, pkg := range strings.Split(string(out), "\n") {
			pkg = strings.TrimSpace(pkg)
			pkg = "." + strings.TrimPrefix(pkg, "gitlab.com/accumulatenetwork/accumulate")
			run(pkg, args)
		}
	default:
		run(pkg, args)
	}
}

func run(pkg string, args []string) {
	// Clear the trace directory
	cwd, err := os.Getwd()
	check(err)
	dir := filepath.Join(cwd, "trace", pkg)
	check(os.RemoveAll(dir))
	check(os.MkdirAll(filepath.Dir(dir), 0700))

	// Compile the binary
	cmd := exec.Command("go", append([]string{"test", "-json", "-gcflags", "all=-N -l", "-exec", "rr record -o " + dir, pkg}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	check(cmd.Run())
}
