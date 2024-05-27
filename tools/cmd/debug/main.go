// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var (
	outputJSON        bool
	healContinuous    bool
	cachedScan        string
	verbose           bool
	pretend           bool
	debug             bool
	waitForTxn        bool
	peerDb            string
	lightDb           string
	only              string
	pprof             string
	healSinceDuration time.Duration
)

var cmd = &cobra.Command{
	Use:   "debug",
	Short: "Accumulate debug utilities",
}

var currentUser = func() *user.User {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return u
}()

var cacheDir = filepath.Join(currentUser.HomeDir, ".accumulate", "cache")

func main() {
	_ = cmd.Execute()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		err = errors.UnknownError.Skip(1).Wrap(err)
		fatalf("%+v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

func safeClose(c io.Closer) {
	check(c.Close())
}
