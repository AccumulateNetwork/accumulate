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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestFindMissingTxns(t *testing.T) {
	fmt.Println("Hash, Partition")
	for _, s := range []string{"dn", "apollo", "yutu", "chandrayaan"} {
		rd, ver := openSnapshotFile("/home/firelizzard/src/Accumulate/accumulate/.nodes/restore/" + s + "-genesis.json")
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
						"ScratchChain":
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
			fmt.Printf("%x, %s\n", h, part)
		}
	}
}

func TestCheckFix(t *testing.T) {
	f, err := os.Open("/home/firelizzard/src/Accumulate/accumulate/missing.csv")
	require.NoError(t, err)
	defer f.Close()

	dbs := map[string]*database.Batch{}
	for _, s := range []string{"directory", "apollo", "yutu", "chandrayaan"} {
		db, err := database.OpenBadger("/home/firelizzard/src/Accumulate/accumulate/.nodes/restore/"+s+".db", nil)
		require.NoError(t, err)
		defer f.Close()
		batch := db.Begin(false)
		defer batch.Discard()
		dbs[s] = batch
	}

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	r.ReuseRecord = true
	_, err = r.Read() // Skip the header
	require.NoError(t, err)

	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
		}
		require.Len(t, rec, 2)

		b, err := hex.DecodeString(rec[0])
		require.NoError(t, err)

		_, err = dbs[strings.ToLower(rec[1])].Message2(b).Main().Get()
		switch {
		case err == nil:
			// Yay!
		case errors.Is(err, errors.NotFound):
			fmt.Printf("Missing %x from %s\n", b, rec[1])
			t.Fail()
		default:
			require.NoError(t, err)
		}
	}
}
