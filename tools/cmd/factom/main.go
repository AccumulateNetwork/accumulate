// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var cmd = &cobra.Command{
	Use:   "factom",
	Short: "Factom utilities",
}

func main() {
	testing.EnableDebugFeatures()
	_ = cmd.Execute()
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

func onInterrupt(fn func()) {
	ch := make(chan os.Signal, 1)
	go func() {
		<-ch
		signal.Stop(ch)
		fn()
	}()
	signal.Notify(ch, os.Interrupt)
}

func newLogger() log.Logger {
	logWriter, err := logging.NewConsoleWriter("plain")
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(logWriter), flagConvert.LogLevel, false)
	check(err)
	return logger
}

func tempBadger(logger log.Logger) (*database.Database, func()) {
	dbdir, err := os.MkdirTemp("", "badger-*.db")
	check(err)

	db, err := database.OpenBadger(dbdir, logger)
	checkf(err, "open temp badger")
	return db, func() { _ = os.RemoveAll(dbdir) }
}

func readFactomDir(dir string, process func([]byte, int)) {
	FCTHeight := flagConvert.StartFrom

	ok := true
	onInterrupt(func() { ok = false })

	for ok {
		filename := filepath.Join(dir, fmt.Sprintf("objects-%d.dat", FCTHeight))

		input, err := ioutil.ReadFile(filename)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}

		checkf(err, "read %s", filename)
		fmt.Printf("Processing %s\n", filename)
		process(input, FCTHeight)

		FCTHeight += 2000
		if flagConvert.StopAt > 0 && FCTHeight >= flagConvert.StopAt {
			return
		}
	}
	fmt.Println("Interrupted")
}
