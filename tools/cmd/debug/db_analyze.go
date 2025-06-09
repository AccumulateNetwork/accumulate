// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var cmdDbAnalyze = &cobra.Command{
	Use:   "analyze [database]",
	Short: "Analyze a database",
	Example: `` +
		`  debug db analyze badger:///path/to/source.db` + "\n" +
		`  debug db analyze unix:///path/to/analyze.sock` + "\n",
	Args: cobra.ExactArgs(1),
	Run:  analyzeDatabases,
}

var cmdDbAnalyzeScan = &cobra.Command{
	Use:   "scan [database]",
	Short: "Dump JSON records that can be used to build an analysis tree",
	Args:  cobra.ExactArgs(1),
	Run:   analyzeScan,
}

var cmdDbAnalyzeLog = &cobra.Command{
	Use:   "log [file]",
	Short: "Analyze a log made by the scan command",
	Args:  cobra.ExactArgs(1),
	Run:   analyzeLog,
}

var flagDbAnalyze = struct {
	Accounts bool
}{}

func init() {
	cmdDb.AddCommand(cmdDbAnalyze)
	cmdDbAnalyze.AddCommand(cmdDbAnalyzeScan, cmdDbAnalyzeLog)

	cmdDbAnalyze.PersistentFlags().BoolVar(&flagDbAnalyze.Accounts, "accounts", false, "Scan accounts via the BPT instead of scanning the entire database")
}

func analyzeDatabases(_ *cobra.Command, args []string) {
	local, remote := openDbUrl(args[0], false)

	progress := newProgress()
	if remote != nil {
		// Analyze remote
		check(analyzeRemote(remote, false, progress))
		return
	}

	// Analyze local
	defer func() { check(local.Close()) }()
	batch := local.Begin(nil, false)
	defer batch.Discard()
	check(analyzeDb(batch, false, progress))
}

func analyzeScan(_ *cobra.Command, args []string) {
	local, remote := openDbUrl(args[0], false)

	progress := func(s string, a ...any) {}
	if remote != nil {
		// Analyze remote
		check(analyzeRemote(remote, true, progress))
		return
	}

	// Analyze local
	defer func() { check(local.Close()) }()
	batch := local.Begin(nil, false)
	defer batch.Discard()
	check(analyzeDb(batch, true, progress))
}

func analyzeLog(_ *cobra.Command, args []string) {
	f, err := os.Open(args[0])
	check(err)
	defer func() { f.Close() }()

	// Count the number of entries
	progress := newProgress()
	progress("Counting...")
	var buf [32 * 1024]byte
	var count int
	for {
		n, err := f.Read(buf[:])
		if errors.Is(err, io.EOF) {
			break
		}
		check(err)
		count += bytes.Count(buf[:n], []byte("\n"))
	}

	_, err = f.Seek(0, io.SeekStart)
	check(err)

	tick := time.NewTicker(time.Second / 2)
	defer tick.Stop()

	progress("Analyzing...")
	n := new(node)
	dec := json.NewDecoder(f)
	for i := 0; ; i++ {
		var v analyzeLogRecord
		err := dec.Decode(&v)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			slog.Warn("Invalid record", "error", err)
			continue
		}
		n.Add(v.Key, v.Size)

		select {
		case <-tick.C:
			progress("Analyzing [%d/%d] %v", i+1, count, v.Key)
		default:
		}
	}

	n.Print()
}

func analyzeRemote(addr net.Addr, scan bool, progress func(string, ...any)) error {
	return withRemoteKeyValueStore(addr, func(src *remote.Store) error {
		return analyzeDb(src, scan, progress)
	})
}

type analyzeLogRecord struct {
	Key  *record.Key `json:"key"`
	Size int64       `json:"size"`
}

func analyzeDb(src keyvalue.Store, scan bool, progress func(string, ...any)) error {
	progress("Analyzing...")
	t := time.NewTicker(time.Second / 2)
	defer t.Stop()

	n := new(node)

	add := func(key *record.Key, size int64) {
		if !scan {
			n.Add(key, size)
			return
		}

		b, err := json.Marshal(analyzeLogRecord{key, size})
		if err == nil {
			fmt.Printf("%s\n", b)
		} else {
			slog.Warn("Error marshalling record", "error", err)
		}
	}

	var err error
	if flagDbAnalyze.Accounts {
		db := coredb.New(memory.NewChangeSet(memory.ChangeSetOptions{
			Get: src.Get,
		}), nil)

		var count int
		_, err = db.Collect(&ioutil.Discard{}, nil, &coredb.CollectOptions{
			Predicate: func(r database.Record) (bool, error) {
				// Don't count the BPT
				if r.Key().Get(0) == "BPT" {
					return false, nil
				}

				// Skip chains
				if _, ok := r.(*coredb.Chain2); ok {
					return false, nil
				}

				// Print progress
				count++
				select {
				case <-t.C:
					switch r.Key().Get(0) {
					case "Account":
						h := r.Key().SliceJ(2).Hash()
						progress("\033[A\r\033[KScanning (%d) [%x] %v\n", count, h[:4], r.Key())

					case "Message", "Transaction":
						progress("\033[A\r\033[KScanning (%d) %v\n", count, r.Key())
					}
				default:
				}

				// Walk composite records
				v, ok := r.(database.Value)
				if !ok {
					return true, nil
				}

				// Get the binary data
				u, _, err := v.GetValue()
				if err != nil {
					return false, errors.UnknownError.WithFormat("get record value: %w", err)
				}
				b, err := u.MarshalBinary()
				if err != nil {
					return false, errors.EncodingError.WithFormat("marshal record value: %w", err)
				}

				// Record
				add(r.Key(), int64(len(b)))

				// Don't actually collect
				return false, nil
			},
		})

	} else {
		var count int
		err = src.ForEach(func(key *record.Key, value []byte) error {
			select {
			case <-t.C:
				progress("Analyzing %d...", count)
			default:
			}
			count++

			b, _ := key.MarshalBinary()
			add(key, int64(len(b)+len(value)))
			return nil
		})
	}
	if err != nil {
		return err
	}

	n.Print()
	return nil
}

type node struct {
	prefix   string
	children map[string]*node
	size     int64
	depth    int
	versions int
}

func (n *node) get(s *record.Key) *node {
	if n.depth >= s.Len() {
		return n
	}

	if n.children == nil {
		n.children = map[string]*node{}
	}
	ss := s.SliceI(n.depth).SliceJ(1).String()
	m, ok := n.children[ss]
	if ok {
		return m
	}

	m = &node{depth: n.depth + 1, prefix: s.SliceJ(n.depth + 1).String()}
	n.children[ss] = m
	return m
}

func (n *node) Versions(s *record.Key) int {
	if n.depth >= s.Len() {
		return n.versions
	}
	return n.get(s).Versions(s)
}

func (n *node) Add(s *record.Key, z int64) {
	n.size += z
	if n.depth >= s.Len() {
		n.versions++
		return
	}

	n.get(s).Add(s, z)
}

func (n *node) Print() {
	if n.children == nil {
		return
	}

	if n.prefix == "Message" {
		fmt.Printf("%v   %v\n", byteCountIEC(n.size), n.prefix)
		return
	}

	var keys []string
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n.children[k].Print()
	}
	fmt.Printf("%v   %v\n", byteCountIEC(n.size), n.prefix)
}

func byteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		s := fmt.Sprintf("%d", b)
		s = strings.Repeat(" ", 4-len(s)) + s
		return fmt.Sprintf("%v   B", s)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= 9999; n /= unit {
		div *= unit
		exp++
	}
	s := fmt.Sprintf("%.0f", float64(b)/float64(div))
	s = strings.Repeat(" ", 4-len(s)) + s
	return fmt.Sprintf("%s %ciB", s, "KMGTPE"[exp])
}

func newProgress() func(string, ...any) {
	var didProgress bool
	return func(s string, a ...any) {
		if !didProgress {
			fmt.Printf(s, a...)
			didProgress = true
			return
		}
		fmt.Fprintf(os.Stderr, "\033[A\r\033[K%s\n", fmt.Sprintf(s, a...))
	}
}
