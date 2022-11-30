// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
)

var listCmd = &cobra.Command{
	Use:   "list <directory>",
	Short: "List snapshots",
	Args:  cobra.ExactArgs(1),
	Run:   listSnapshots,
}

var listFlag = struct {
	Serve string
}{}

func init() {
	cmd.AddCommand(listCmd)
	listCmd.Flags().StringVarP(&listFlag.Serve, "serve", "s", "", "Run a server")
}

func listSnapshots(_ *cobra.Command, args []string) {
	if listFlag.Serve != "" {
		ln, err := net.Listen("tcp", listFlag.Serve)
		check(err)
		fmt.Printf("Serving on %v\n", ln.Addr())
		s := new(http.Server)
		s.Addr = ln.Addr().String()
		s.Handler = listSnapshotServer(args[0])
		s.ReadHeaderTimeout = 1 * time.Second
		log.Fatal(s.Serve(ln))
	}

	snapDir := args[0]
	entries, err := os.ReadDir(snapDir)
	checkf(err, "read directory")

	wr := tabwriter.NewWriter(os.Stdout, 3, 4, 2, ' ', 0)
	defer wr.Flush()

	fmt.Fprint(wr, "HEIGHT\tHASH\tFILE\n")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !core.SnapshotMajorRegexp.MatchString(entry.Name()) {
			continue
		}

		filename := filepath.Join(snapDir, entry.Name())
		f, err := os.Open(filename)
		checkf(err, "open snapshot %s", entry.Name())
		defer f.Close()

		header, _, err := snapshot.Open(f)
		checkf(err, "open snapshot %s", entry.Name())

		fmt.Fprintf(wr, "%d\t%x\t%s\n", header.Height, header.RootHash, entry.Name())
	}
}

type listSnapshotServer string

func (s listSnapshotServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	snapDir := string(s)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		log.Printf("Read directory: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var headers []*snapshot.Header
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !core.SnapshotMajorRegexp.MatchString(entry.Name()) {
			continue
		}

		filename := filepath.Join(snapDir, entry.Name())
		f, err := os.Open(filename)
		if err != nil {
			log.Printf("Open snapshot %s: %v", entry.Name(), err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer f.Close()

		header, _, err := snapshot.Open(f)
		if err != nil {
			log.Printf("Open snapshot %s: %v", entry.Name(), err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		headers = append(headers, header)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(headers)
	if err != nil {
		log.Printf("Marshal: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
