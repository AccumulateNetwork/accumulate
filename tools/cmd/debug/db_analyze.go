// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
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

func init() {
	cmdDb.AddCommand(cmdDbAnalyze)
}

func analyzeDatabases(_ *cobra.Command, args []string) {
	local, remote := openDbUrl(args[0], false)

	var didProgress bool
	progress := func(s string) {
		if !didProgress {
			fmt.Println(s)
			didProgress = true
			return
		}
		fmt.Printf("\033[A\r\033[K%s\n", s)
	}

	if remote != nil {
		// Analyze remote
		check(analyzeRemote(remote, progress))
		return
	}

	// Analyze local
	defer func() { check(local.Close()) }()
	batch := local.Begin(nil, false)
	defer batch.Discard()
	check(analyzeDb(batch, progress))
}

func analyzeRemote(addr net.Addr, progress func(string)) error {
	return withRemoteKeyValueStore(addr, func(src *remote.Store) error {
		return analyzeDb(src, progress)
	})
}

func analyzeDb(src keyvalue.Store, progress func(string)) error {
	progress("Analyzing...")
	t := time.NewTicker(time.Second / 2)
	defer t.Stop()

	var count int
	n := new(node)
	err := src.ForEach(func(key *record.Key, value []byte) error {
		select {
		case <-t.C:
			progress(fmt.Sprintf("Analyzing %d...", count))
		default:
		}
		count++

		b, _ := key.MarshalBinary()
		n.Add(key, int64(len(b)+len(value)))
		return nil
	})
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
