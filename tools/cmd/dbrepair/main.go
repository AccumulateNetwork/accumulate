// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var cmd = &cobra.Command{
	Use: "dbRepair",
	Short: "utilities to use a reference db to repair or synchronize a target db.\n" +
		"As short hand, the reference is 'good' and the target is 'bad'",
}

var cmdBuildTestDBs = &cobra.Command{
	Use:   "buildTestDBs [number of entries] [good database] [bad database]",
	Short: "Number of entries must be greater than 100, should be greater than 5000",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildTestDBs,
}

var cmdBuildSummary = &cobra.Command{
	Use:   "buildSummary [good database] [summary file]",
	Short: "Build a Summary file of the keys and values in a good database",
	Args:  cobra.ExactArgs(2),
	Run:   runBuildSummary,
}

var cmdBuildDiff = &cobra.Command{
	Use:   "buildDiff [summary file] [bad database] [diff File]",
	Short: "Given the summary data and the bad database, build a diff File to make the bad database good",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildDiff,
}
var cmdBuildMissing = &cobra.Command{
	Use:   "buildMissing [summary file] [bad database] [diff File]",
	Short: "Given the summary data and the bad database, build a diff File of solely the keys missing from the bad database",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildMissing,
}

var cmdPrintDiff = &cobra.Command{
	Use:   "printDiff [diff file] [good database]",
	Short: "Given the summary data and the good database, print the diff file",
	Args:  cobra.ExactArgs(2),
	Run:   runPrintDiff,
}

var cmdBuildFix = &cobra.Command{
	Use:   "buildFix [diff file] [good database] [fix file]",
	Short: "Take the difference between the good and bad database, the good data base, and build a fix file",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildFix,
}

var cmdPrintFix = &cobra.Command{
	Use:   "printFix [fix file]",
	Short: "Prints the contents of the fix file",
	Args:  cobra.ExactArgs(1),
	Run:   runPrintFix,
}

var cmdApplyFix = &cobra.Command{
	Use:   "applyFix [fix file] [database]",
	Short: "Applies the fix to the database",
	Args:  cobra.ExactArgs(2),
	Run:   runApplyFix,
}

var cmdPrintFix = &cobra.Command{
	Use:   "printFix [fix file]",
	Short: "Prints a fix file",
	Args:  cobra.ExactArgs(1),
	Run:   runPrintFix,
}

var cmdCheckFix = &cobra.Command{
	Use:   "checkFix [fix file] [missing csv]",
	Short: "Checks a fix file",
	Args:  cobra.ExactArgs(2),
	Run:   runCheckFix,
}

var cmdDumpFixHashes = &cobra.Command{
	Use:   "dumpFixHashes [fix file] [output file]",
	Short: "collects all the keys, sorts them, then dumps them in ASCII into the output file",
	Args:  cobra.ExactArgs(2),
	Run:   runDumpFixHashes,
}

func init() {
	cmd.AddCommand(
		cmdBuildTestDBs,
		cmdBuildSummary,
		cmdBuildDiff,
		cmdBuildMissing,
		cmdPrintDiff,
		cmdBuildFix,
		cmdPrintFix,
		cmdApplyFix,
		cmdPrintFix,
		cmdCheckFix,
		cmdDumpFixHashes,
	)
}

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

func check2(_ any, err error) { check(err) }

func OpenDB(dbName string) (*badger.DB, func()) {
	db, err := badger.Open(badger.DefaultOptions(dbName))
	checkf(err, "opening db %v", dbName)
	return db, func() { checkf(db.Close(), "closing db %v", dbName) }
}

// Read an 8 byte, uint64 value and return it.
// As a side effect, the first 8 bytes of buff hold the value
func read8(f *os.File, buff []byte, tag string) uint64 {
	if buff == nil {
		buff = make([]byte, 8)
	}
	r, err := f.Read(buff[:8]) // Read 64 bits
	checkf(err, "%s: failed to read 64 bit int", tag)
	if r != 8 {
		fatalf("%s: failed to read a full 64 bit value", tag)
	}
	return binary.BigEndian.Uint64(buff[:8])
}

// Read 32 bytes into a given buffer
func read32(f *os.File, buff []byte, tag string) {
	r, err := f.Read(buff[:32]) // Read 32
	checkf(err, "%s: failed to read address", tag)
	if r != 32 {
		fatalf("%s: failed to read a full 32 byte value", tag)
	}
}

func read(f *os.File, buff []byte) {
	r, err := f.Read(buff) // Read 32
	checkf(err, "failed to read value")
	if r != len(buff) {
		fatalf("failed to read the full value")
	}
}

// Write the integer value out as 8 bytes
func write8(f *os.File, i interface{}) {
	var buff [8]byte
	switch v := i.(type) {
	case uint64:
		binary.BigEndian.PutUint64(buff[:], v)
	case int:
		binary.BigEndian.PutUint64(buff[:], uint64(v))
	}
	i, err := f.Write(buff[:])
	checkf(err, "failed a write to fix file %s", f.Name())
	if i != 8 {
		fatalf("failed to write the 8 bytes to %s", f.Name())
	}
}

func write(f *os.File, buff []byte) {
	i, err := f.Write(buff)
	checkf(err, "failed a write to fix file %s", f.Name())
	if i != len(buff) {
		fatalf("failed to make a complete write of %d. wrote %d", len(buff), i)
	}
}
