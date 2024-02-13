// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"golang.org/x/term"
)

var currentUser = func() *user.User {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr
}()

var defaultWorkDir = filepath.Join(currentUser.HomeDir, ".accumulate")

var cmdMain = &cobra.Command{
	Use:   "accumulated",
	Short: "Accumulate network daemon",
	Args:  cobra.MaximumNArgs(1),
	Run:   run2,
}

var flagMain struct {
	WorkDir string
	Debug   bool
	Reset   bool
	Pprof   string
}

func init() {
	cmdMain.Flags().BoolVarP(&flagMain.Debug, "debug", "d", false, "Enable debug logging")
	cmdMain.Flags().BoolVar(&flagMain.Reset, "reset", false, "Reset state before starting")
	cmdMain.Flags().StringVar(&flagMain.Pprof, "pprof", "", "Address to run net/http/pprof on")
	cmdMain.PersistentFlags().StringVarP(&flagMain.WorkDir, "work-dir", "w", defaultWorkDir, "Working directory for configuration and data")
}

func main() {
	_ = cmdMain.Execute()
}

func run2(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		printUsageAndExit1(cmd, args)
	}
	if flagMain.Debug {
		logging.EnableDebugFeatures()
	}
	if flagMain.Pprof != "" {
		s := new(http.Server)
		s.Addr = flagMain.Pprof
		s.ReadHeaderTimeout = time.Minute
		go func() { check(s.ListenAndServe()) }() //nolint:gosec
	}

	c := new(run.Config)
	check(c.LoadFrom(findConfigFile(args[0])))

	ctx := cmdutil.ContextForMainProcess(context.Background())
	i, err := run.New(ctx, c)
	check(err)

	if flagMain.Reset {
		check(i.Reset())
	}

	check(i.Start())

	color.HiBlue("\n--- Running ---\n\n")

	<-ctx.Done()
	i.Stop()
}

func printUsageAndExit1(cmd *cobra.Command, _ []string) {
	_ = cmd.Usage()
	os.Exit(1)
}

func findConfigFile(name string) string {
	st, err := os.Stat(name)
	check(err)
	if !st.IsDir() {
		return name
	}

	dir, err := os.ReadDir(name)
	check(err)
	for _, entry := range dir {
		if entry.IsDir() {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if base == "accumulate" {
			return filepath.Join(name, entry.Name())
		}
	}
	fatalf("no configuration file found in %s", name)
	panic("not reached")
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

func formatVersion(version string, known bool) string {
	if !known {
		return "unknown"
	}
	return version
}

func warnf(format string, args ...interface{}) {
	format = "WARNING!!! " + format + "\n"
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, color.RedString(format, args...))
	} else {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
