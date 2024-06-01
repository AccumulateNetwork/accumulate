// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/util/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

var cmdSnapIndex = &cobra.Command{
	Use:   "index [snapshot]",
	Short: "Indexes the records of a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   indexSnapshot,
}

func init() {
	cmdSnap.AddCommand(cmdSnapIndex)
}

func indexSnapshot(_ *cobra.Command, args []string) {
	f, err := os.OpenFile(args[0], os.O_RDWR, 0)
	checkf(err, "open snapshot %s", args[0])
	defer safeClose(f)

	ver, err := snapshot.GetVersion(f)
	check(err)
	if ver != snapshot.Version2 {
		fatalf("cannot index a version %d snapshot", ver)
	}

	rd, err := snapshot.Open(f)
	check(err)

	var records []int
	for i, s := range rd.Sections {
		switch s.Type() {
		case snapshot.SectionTypeRecordIndex:
			fatalf("snapshot is already indexed")
		case snapshot.SectionTypeRecords:
			records = append(records, i)
		}
	}

	tmp, err := os.MkdirTemp("", "accumulate-index-snapshot-*")
	check(err)
	defer os.RemoveAll(tmp)

	const indexDataSize = 16
	index, err := indexing.OpenBucket(filepath.Join(tmp, "hash"), indexDataSize, true)
	check(err)
	defer safeClose(index)

	for _, i := range records {
		rd, err := rd.OpenRecords(i)
		check(err)

		for {
			pos, err := rd.Seek(0, io.SeekCurrent)
			check(err)
			entry, err := rd.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			check(err)

			var b [indexDataSize]byte
			binary.BigEndian.PutUint64(b[:], uint64(i))
			binary.BigEndian.PutUint64(b[8:], uint64(pos))
			check(index.Write(entry.Key.Hash(), b[:]))
		}
	}

	wr, err := snapshot.Append(f)
	check(err)
	x, err := wr.OpenIndex()
	check(err)
	defer safeClose(x)

	for i := 0; i < 256; i++ {
		entries, err := index.Read(byte(i))
		check(err)

		sort.Slice(entries, func(i, j int) bool {
			return bytes.Compare(entries[i].Hash[:], entries[j].Hash[:]) < 0
		})

		for _, e := range entries {
			check(x.Write(snapshot.RecordIndexEntry{
				Key:     e.Hash,
				Section: int(binary.BigEndian.Uint64(e.Value)),
				Offset:  binary.BigEndian.Uint64(e.Value[8:]),
			}))
		}
	}
}
