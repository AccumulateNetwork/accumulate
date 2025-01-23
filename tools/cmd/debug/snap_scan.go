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
	"os"
	"path/filepath"
	"time"

	"github.com/cometbft/cometbft/types"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
)

func openSnapshotFile(filename string) ioutil.SectionReader {
	if filepath.Ext(filename) == ".json" {
		fmt.Fprintf(os.Stderr, "Loading %s\n", filename)
		genDoc, err := types.GenesisDocFromFile(filename)
		checkf(err, "read %s", filename)

		var b []byte
		check(json.Unmarshal(genDoc.AppState, &b))
		return bytes.NewReader(b)

	} else {
		f, err := os.Open(filename)
		checkf(err, "open snapshot %s", filename)
		return f
	}
}

type snapshotScanArgs struct {
	Section   func(typ sv2.SectionType, size, offset int64) bool
	Header    func(version, height uint64, timestamp time.Time, rootHash [32]byte)
	Record    func(any)
	Consensus func(*cometbft.GenesisDoc)
}

func scanSnapshot(f ioutil.SectionReader, args snapshotScanArgs) {
	ver, err := sv2.GetVersion(f)
	check(err)

	switch ver {
	case sv1.Version1:
		scanSnapV1(f, args)
	case sv2.Version2:
		scanSnapV2(f, args)
	default:
		fatalf("I don't know how to handle snapshot version %d", ver)
	}
}

func scanSnapV2(f ioutil.SectionReader, args snapshotScanArgs) {
	r, err := sv2.Open(f)
	check(err)

	for i, s := range r.Sections {
		if args.Section != nil {
			if !args.Section(s.Type(), s.Size(), s.Offset()) {
				continue
			}
		}

		switch s.Type() {
		case sv2.SectionTypeHeader:
			if args.Header == nil {
				break
			}

			var height uint64
			var timestamp time.Time
			if r.Header.SystemLedger != nil {
				height, timestamp = r.Header.SystemLedger.Index, r.Header.SystemLedger.Timestamp
			}
			args.Header(r.Header.Version, height, timestamp, r.Header.RootHash)

		case sv2.SectionTypeRecords,
			sv2.SectionTypeBPT,
			sv2.SectionTypeRawBPT:
			if args.Record == nil {
				break
			}

			var rr sv2.RecordReader
			if s.Type() == sv2.SectionTypeRecords {
				rr, err = r.OpenRecords(i)
			} else {
				rr, err = r.OpenBPT(i)
			}
			check(err)

			for {
				re, err := rr.Read()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					check(err)
				}
				args.Record(re)
			}

		case sv2.SectionTypeRecordIndex:
			if args.Record == nil {
				break
			}

			rd, err := r.OpenIndex(i)
			check(err)

			for i, n := 0, rd.Count; i < n; i++ {
				e, err := rd.Read(i)
				check(err)
				args.Record(e)
			}

		case sv2.SectionTypeConsensus:
			if args.Consensus == nil {
				break
			}

			rd, err := s.Open()
			check(err)

			doc := new(cometbft.GenesisDoc)
			check(doc.UnmarshalBinaryFrom(rd))
			args.Consensus(doc)
		}
	}
}

func scanSnapV1(f ioutil.SectionReader, args snapshotScanArgs) {
	r := sv1.NewReader(f)
	s, err := r.Next()
	checkf(err, "find header")
	sr, err := s.Open()
	checkf(err, "open header")
	header := new(sv1.Header)
	_, err = header.ReadFrom(sr)
	checkf(err, "read header")

	ok := args.Section == nil || args.Section(sv2.SectionTypeHeader, s.Size(), s.Offset())
	if ok && args.Header != nil {
		args.Header(header.Version, header.Height, time.Time{}, header.RootHash)
	}

	_, err = f.Seek(0, io.SeekStart)
	check(err)

	check(sv1.Visit(f, scanVisitor(args)))
}

type scanVisitor snapshotScanArgs

func (v scanVisitor) VisitSection(s *sv1.ReaderSection) error {
	if v.Section == nil {
		return nil
	}

	if !v.Section(s.Type(), s.Size(), s.Offset()) {
		return sv1.ErrSkip
	}
	return nil
}

func (v scanVisitor) VisitAccount(r *sv1.Account, _ int) error {
	if r == nil || v.Record == nil {
		return nil
	}

	v.Record(r)
	return nil
}

func (v scanVisitor) VisitTransaction(r *sv1.Transaction, _ int) error {
	if r == nil || v.Record == nil {
		return nil
	}

	v.Record(r)
	return nil
}

func (v scanVisitor) VisitSignature(r *sv1.Signature, _ int) error {
	if r == nil || v.Record == nil {
		return nil
	}

	v.Record(r)
	return nil
}
