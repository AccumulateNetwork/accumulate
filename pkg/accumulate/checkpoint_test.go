// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulate_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestCreateCheckpoint(t *testing.T) {
	t.Skip("Manual")

	old, err := os.Open("../../.nodes/mainnet/dnn/config/genesis.snap")
	require.NoError(t, err)
	defer old.Close()

	newf, err := os.Create("checkpoint-mainnet.snap")
	require.NoError(t, err)
	defer newf.Close()

	r, err := snapshot.Open(old)
	require.NoError(t, err)

	w, err := snapshot.Create(newf)
	require.NoError(t, err)

	temp := database.OpenInMemory(nil).Begin(true)
	defer temp.Discard()

	type RecordPos struct {
		Key     *record.Key
		Section int
		Offset  uint64
	}

	var accounts []*url.URL
	var records []*RecordPos
	accountSeen := map[[32]byte]bool{}
	recordSeen := map[[32]byte]bool{}
	for i, s := range r.Sections {
		switch s.Type() {
		case snapshot.SectionTypeHeader:
			// Copy the header
			rd, err := s.Open()
			require.NoError(t, err)
			wr, err := w.OpenRaw(s.Type())
			require.NoError(t, err)

			_, err = io.Copy(wr, rd)
			require.NoError(t, err)
			require.NoError(t, wr.Close())

		case snapshot.SectionTypeBPT,
			snapshot.SectionTypeRawBPT:
			// Read the BPT
			rd, err := r.OpenBPT(i)
			require.NoError(t, err)

			for {
				rec, err := rd.Read()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)
				require.NoError(t, temp.BPT().Insert(rec.Key, rec.Value))
			}

		case snapshot.SectionTypeRecords:
			// Copy records
			rd, err := r.OpenRecords(i)
			require.NoError(t, err)

			var wr *snapshot.SectionWriter
			for {
				rec, err := rd.Read()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)

				// Include DN accounts
				u, ok := rec.Key.Get(1).(*url.URL)
				if !ok || rec.Key.Get(0) != "Account" || !protocol.BelongsToDn(u) {
					continue
				}

				// Exclude block ledgers
				const ledger = "/ledger/"
				if strings.HasPrefix(u.Path, ledger) && len(u.Path) > len(ledger) {
					continue
				}

				// Exclude mark points
				if rec.Key.Len() > 3 && rec.Key.Get(rec.Key.Len()-2) == "States" {
					continue
				}

				// Prevent duplicates (for snapshots made prior to #3410)
				if !recordSeen[rec.Key.Hash()] {
					recordSeen[rec.Key.Hash()] = true
				} else {
					continue
				}

				// Open the destination
				if wr == nil {
					wr, err = w.OpenRaw(snapshot.SectionTypeRecords)
					require.NoError(t, err)
				}

				offset, err := wr.Seek(0, io.SeekCurrent)
				require.NoError(t, err)
				require.NoError(t, wr.WriteValue(rec))

				records = append(records, &RecordPos{
					Key:     rec.Key,
					Section: wr.SectionNumber(),
					Offset:  uint64(offset),
				})

				if !accountSeen[u.AccountID32()] {
					accountSeen[u.AccountID32()] = true
					accounts = append(accounts, u)
				}
			}
			if wr != nil {
				require.NoError(t, wr.Close())
			}
		}
	}

	// Record BPT proofs
	{
		wr, err := w.OpenRaw(snapshot.SectionTypeBPT)
		require.NoError(t, err)

		for _, u := range accounts {
			account := temp.Account(u)

			hash, err := temp.BPT().Get(account.Key())
			require.NoError(t, err)

			proof, err := account.BptReceipt()
			require.NoError(t, err)

			require.NoError(t, wr.WriteValue(&snapshot.RecordEntry{
				Key:     account.Key(),
				Value:   hash[:],
				Receipt: proof,
			}))

			require.Equal(t, proof.Start, hash[:], "Proof must start with account hash")
			require.Equal(t, proof.Anchor, r.Header.RootHash[:], "Proof must end with state hash")
			require.True(t, proof.Validate(nil), "Proof must be valid")
		}
		require.NoError(t, wr.Close())
	}

	// Write the record index
	{
		sort.Slice(records, func(i, j int) bool {
			a, b := records[i].Key.Hash(), records[j].Key.Hash()
			return bytes.Compare(a[:], b[:]) < 0
		})

		wr, err := w.OpenIndex()
		require.NoError(t, err)

		for _, r := range records {
			err = wr.Write(snapshot.RecordIndexEntry{
				Key:     r.Key.Hash(),
				Section: r.Section,
				Offset:  r.Offset,
			})
			require.NoError(t, err)
		}

		require.NoError(t, wr.Close())
	}
}
