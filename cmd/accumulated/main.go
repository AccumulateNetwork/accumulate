// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
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
}

func init() {
	cmdMain.PersistentFlags().StringVarP(&flagMain.WorkDir, "work-dir", "w", defaultWorkDir, "Working directory for configuration and data")
}

func main() {
	_ = cmdMain.Execute()
}

func run2(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		printUsageAndExit1(cmd, args)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		<-sigs
		signal.Stop(sigs)
		cancel()
	}()

	c := new(run.Config)
	check(c.LoadFrom(args[0]))
	inst, err := run.Start(ctx, c)
	check(err)
	<-ctx.Done()
	check(inst.Stop())
}

func printUsageAndExit1(cmd *cobra.Command, _ []string) {
	_ = cmd.Usage()
	os.Exit(1)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
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
