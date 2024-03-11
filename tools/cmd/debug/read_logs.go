// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
)

var now = time.Now()

var readLogsCmd = &cobra.Command{
	Use:   "read-logs <file> [<file>...]",
	Short: "Read and filter JSON log files",
	Args:  cobra.MinimumNArgs(1),
}

var flagReadLogs = struct {
	After  string
	Before string
	Module string
}{}

func init() {
	cmd.AddCommand(readLogsCmd)

	readLogsCmd.Run = readLogs
	readLogsCmd.Flags().StringVar(&flagReadLogs.After, "after", "", "Filter events after this date or some duration ago")
	readLogsCmd.Flags().StringVar(&flagReadLogs.Before, "before", "", "Filter events before this date or some duration ago")
	readLogsCmd.Flags().StringVar(&flagReadLogs.Module, "module", "", "Filter events from this module")
}
func parseTime(flag string, s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	} else {
		fmt.Fprintln(os.Stdout, err)
	}

	d, err := time.ParseDuration(s)
	if err == nil {
		return now.Add(-d)
	} else {
		fmt.Fprintln(os.Stdout, err)
	}

	fatalf("%s: %q is not a date or duration", flag, s)
	panic("not reached")
}

func readLogs(_ *cobra.Command, args []string) {
	var before, after time.Time
	var filterBefore, filterAfter, filterModule bool
	if flagReadLogs.Before != "" {
		before, filterBefore = parseTime("--before", flagReadLogs.Before), true
	}
	if flagReadLogs.After != "" {
		after, filterAfter = parseTime("--after", flagReadLogs.After), true
	}
	filterModule = flagReadLogs.Module != ""

	if filterBefore && filterAfter && before.Before(after) {
		fatalf("--before is before --after")
	}

	type eventToPrint struct {
		data   []byte
		fileNo int
	}

	wg := new(sync.WaitGroup)
	ch := make(chan eventToPrint)

	var count uint64
	for i, f := range args {
		f, err := os.Open(f)
		check(err)
		defer f.Close()

		fileNo := i

		wg.Add(1)
		go func() {
			defer wg.Done()

			var event struct {
				Module string    `json:"module"`
				Time   time.Time `json:"time"`
			}
			r := bufio.NewReader(f)
			for {
				b, err := r.ReadBytes('\n')
				if errors.Is(err, io.EOF) {
					break
				}

				atomic.AddUint64(&count, 1)
				err = json.Unmarshal(b, &event)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid JSON payload: %sError: %v\n", b, err)
					continue
				}

				if filterModule && event.Module != flagReadLogs.Module {
					continue
				}

				if filterBefore && !event.Time.Before(before) {
					continue
				}

				if filterAfter && !event.Time.After(after) {
					continue
				}

				ch <- eventToPrint{data: b, fileNo: fileNo}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	var files []string
	for _, f := range args {
		files = append(files, filepath.Base(f))
	}

	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return
			}
			fmt.Printf(`{"file":"%s",%s`, files[e.fileNo], e.data[1:])
		case <-tick.C:
			fmt.Fprintf(os.Stderr, "%d events read\n", atomic.LoadUint64(&count))
		}
	}
}
