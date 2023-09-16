// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestFindMissingTxns(t *testing.T) {
	for _, s := range []string{"directory", "apollo", "yutu", "chandrayaan"} {
		f, err := os.Create("../../../.nodes/restore/" + s + "-missing.csv")
		require.NoError(t, err)
		defer f.Close()

		rd, ver := openSnapshotFile("../../../.nodes/restore/" + s + "-genesis.json")
		if c, ok := rd.(io.Closer); ok {
			defer c.Close()
		}
		require.True(t, snapshot.Version2 == ver)

		wantMsg := map[[32]byte]bool{}
		gotMsg := map[[32]byte]bool{}

		r, err := snapshot.Open(rd)
		require.NoError(t, err)

		part, ok := protocol.ParsePartitionUrl(r.Header.SystemLedger.Url)
		require.True(t, ok)

		_ = part
		_ = wantMsg

		for i, s := range r.Sections {
			if s.Type() != snapshot.SectionTypeRecords {
				continue
			}
			rr, err := r.OpenRecords(i)
			require.NoError(t, err)

			for {
				re, err := rr.Read()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					check(err)
				}

				switch re.Key.Get(0) {
				case "Account":
					// Skip LDAs because Factom data
					id, _ := protocol.ParseLiteDataAddress(re.Key.Get(1).(*url.URL))
					if id != nil {
						continue
					}

					switch re.Key.Get(2) {
					case "MainChain",
						"SignatureChain",
						"ScratchChain",
						"AnchorSequenceChain":
						switch re.Key.Get(3) {
						case "Head", "States":
							ms := new(merkle.State)
							require.NoError(t, ms.UnMarshal(re.Value))
							for _, h := range ms.HashList {
								wantMsg[*(*[32]byte)(h)] = true
							}
						}
					}

				case "Message",
					"Transaction":
					if re.Key.Get(2) == "Main" {
						gotMsg[re.Key.Get(1).([32]byte)] = true
					}
				}
			}
		}

		for h := range gotMsg {
			delete(wantMsg, h)
		}

		for h := range wantMsg {
			kh := record.NewKey("Transaction", h, "Main").Hash()
			fmt.Fprintf(f, "%x\n", kh[:])
		}
	}
}

func TestCheckFix(t *testing.T) {
	for _, part := range []string{"directory", "apollo", "yutu", "chandrayaan"} {
		f, err := os.Open("../../../.nodes/restore/" + part + "-missing.csv")
		require.NoError(t, err)
		defer f.Close()

		db, err := badger.New("../../../.nodes/restore/" + part + ".db")
		require.NoError(t, err)
		defer db.Close()
		batch := db.Begin(nil, false)
		defer batch.Discard()

		r := csv.NewReader(f)
		r.TrimLeadingSpace = true
		r.ReuseRecord = true

		for {
			rec, err := r.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)
			}
			require.Len(t, rec, 1)

			b, err := hex.DecodeString(rec[0])
			require.NoError(t, err)

			_, err = batch.Get(record.NewKey(*(*record.KeyHash)(b)))
			switch {
			case err == nil:
				// Yay!
			case errors.Is(err, errors.NotFound):
				fmt.Printf("Missing %x from %s\n", b, part)
				t.Fail()
			default:
				require.NoError(t, err)
			}
		}
	}
}
