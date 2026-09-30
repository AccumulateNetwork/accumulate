// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// migrate-legacy converts a node initialized for CometBFT — dnn/ and bvnn/,
// each with config/tendermint.toml and the old config/accumulate.toml, and
// the CometBFT key files — into one accumulate.toml of the layout `accumulated
// init network` writes. It is the only code that reads that layout; the node
// does not (#3910).
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/fatih/color"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
)

var flagMain struct {
	WorkDir string
}

func main() {
	cmdMigrate.PersistentFlags().StringVarP(&flagMain.WorkDir, "work-dir", "w", "", "The node's directory, holding dnn/ and/or bvnn/")
	cmdMigrate.AddCommand(cmdutil.VersionCmd())
	_ = cmdMigrate.Execute()
}

// runCfg runs the migrated configuration without writing it (--try).
func runCfg(c *run.Config) {
	ctx := cmdutil.ContextForMainProcess(context.Background())
	i, err := run.New(ctx, c)
	check(err)
	check(i.Start())
	color.HiBlue("\n----- Running -----\n\n")
	<-i.Done()
	color.HiBlack("\n----- Stopping -----\n\n")
	i.Stop()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%+v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %+v", append(otherArgs, err)...)
	}
}

func resolveAddrWithPort(addr string) (string, int, error) {
	ip, err := url.Parse(addr)
	if err != nil {
		ip, err = url.Parse("tcp://" + addr)
		if err != nil {
			return "", 0, fmt.Errorf("%q is not a URL", addr)
		}
	}
	if ip.Path != "" && ip.Path != "/" {
		return "", 0, fmt.Errorf("address cannot have a path")
	}
	if ip.Port() == "" {
		return "", 0, fmt.Errorf("%q does not specify a port number", addr)
	}
	port, err := strconv.ParseUint(ip.Port(), 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("%q is not a valid port number", ip.Port())
	}
	return ip.Hostname(), int(port), nil
}
