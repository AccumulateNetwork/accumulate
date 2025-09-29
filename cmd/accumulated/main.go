// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	WorkDir  string
	Debug    bool
	Reset    bool
	InitOnly bool
	Pprof    string
}

func init() {
	setRunFlags(cmdMain)
	cmdMain.PersistentFlags().StringVarP(&flagMain.WorkDir, "work-dir", "w", defaultWorkDir, "Working directory for configuration and data")
}

func setRunFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&flagMain.Debug, "debug", "d", false, "Enable debug logging")
	cmd.Flags().BoolVar(&flagMain.Reset, "reset", false, "Reset state before starting")
	cmd.Flags().BoolVar(&flagMain.InitOnly, "init-only", false, "Initialize and exit, do not run")
	cmd.Flags().StringVar(&flagMain.Pprof, "pprof", "", "Address to run net/http/pprof on")
}

func main() {
	_ = cmdMain.Execute()
}

func run2(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		printUsageAndExit1(cmd, args)
	}

	c := new(run.Config)
	check(c.LoadFrom(mustFindConfigFile(args[0])))
	runCfg(c, nil)
}

func runCfg(c *run.Config, predicate func(run.Service) bool) {
	// Add extensive logging at the start
	fmt.Printf("runCfg starting with %d services\n", len(c.Services))
	for idx, svc := range c.Services {
		fmt.Printf("Service %d: Type=%v\n", idx, svc.Type())
	}

	if flagMain.Debug {
		fmt.Println("Enabling debug features")
		logging.EnableDebugFeatures()
	}
	if flagMain.Pprof != "" {
		s := new(http.Server)
		s.Addr = flagMain.Pprof
		s.ReadHeaderTimeout = time.Minute
		fmt.Printf("Starting pprof server at %s\n", flagMain.Pprof)
		go func() { check(s.ListenAndServe()) }() //nolint:gosec
	}

	fmt.Println("Creating context for main process")
	ctx := cmdutil.ContextForMainProcess(context.Background())

	fmt.Println("Creating new run instance")
	i, err := run.New(ctx, c)
	check(err)
	fmt.Println("Run instance created successfully")

	if flagMain.Reset {
		fmt.Println("Resetting...")
		check(i.Reset())
	}

	if flagMain.InitOnly {
		fmt.Println("Init only mode, returning")
		return
	}

	fmt.Println("Starting services with filter")
	fmt.Printf("Predicate function: %v\n", predicate != nil)
	fmt.Println("About to call i.StartFiltered...")
	err = i.StartFiltered(predicate)
	fmt.Printf("i.StartFiltered returned, err: %v\n", err)
	check(err)
	fmt.Println("Services started successfully")
	color.HiBlue("\n----- Running -----\n\n")
	fmt.Println("After printing Running message")

	// Add heartbeat logging
	fmt.Println("Starting heartbeat goroutine...")
	go func() {
		fmt.Println("[Heartbeat] Goroutine started")
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			fmt.Printf("[Heartbeat] Services still running at %v\n", time.Now().Format("15:04:05"))
		}
	}()
	fmt.Println("Heartbeat goroutine launched")

	fmt.Println("Waiting for Done signal...")
	<-i.Done()
	color.HiBlack("\n----- Stopping -----\n\n")
	i.Stop()
}

func printUsageAndExit1(cmd *cobra.Command, _ []string) {
	_ = cmd.Usage()
	os.Exit(1)
}

func findConfigFile(name string) (string, bool) {
	st, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	check(err)
	if !st.IsDir() {
		return name, true
	}

	dir, err := os.ReadDir(name)
	check(err)
	for _, entry := range dir {
		if entry.IsDir() {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if base == "accumulate" {
			return filepath.Join(name, entry.Name()), true
		}
	}

	return "", false
}

func mustFindConfigFile(name string) string {
	f, ok := findConfigFile(name)
	if ok {
		return f
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
