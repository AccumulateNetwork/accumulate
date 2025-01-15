// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

func TestDbClone(t *testing.T) {
	src := memory.New(nil).Begin(nil, false)
	dst := memory.New(nil).Begin(nil, false)

	// Note this documents what we want to do.  Of course, the
	// test actually skips sending files back and forth between good
	// nodes and bad nodes.
	buildTestDBs(t, 1e3, src, nil) // Build Test DBs

	// Clone
	require.NoError(t, cloneDb(dst, src, func(s string) { fmt.Println(s) }))

	// Verify
	checkDb(t, src, dst)
	checkDb(t, dst, src)
}

func TestDbCloneFromSnapshot(t *testing.T) {
	file, _ := accumulate.MainNetCheckpoint()
	src := openSnapshotAsDb(file).Begin(nil, false)
	dst := memory.New(nil).Begin(nil, false)

	// Clone
	require.NoError(t, cloneDb(dst, src, func(s string) { fmt.Println(s) }))

	// Verify
	checkDb(t, src, dst)
	checkDb(t, dst, src)
}

func TestDbSync(t *testing.T) {
	goodDB := memory.New(nil).Begin(nil, false) // A good db
	badDB := memory.New(nil).Begin(nil, false)  // a bad db with added, modified, and deleted key/value pairs

	// Note this documents what we want to do.  Of course, the
	// test actually skips sending files back and forth between good
	// nodes and bad nodes.
	buildTestDBs(t, 1e3, goodDB, badDB) // Build Test DBs

	// Sync
	require.NoError(t, syncDb(badDB, goodDB, t.TempDir(), func(s string) { fmt.Println(s) }))

	// Verify
	checkDb(t, goodDB, badDB)
	checkDb(t, badDB, goodDB)
}

func TestDbSyncRemote(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	goodDB := memory.New(nil).Begin(nil, false) // A good db
	badDB := memory.New(nil).Begin(nil, false)  // a bad db with added, modified, and deleted key/value pairs

	// Note this documents what we want to do.  Of course, the
	// test actually skips sending files back and forth between good
	// nodes and bad nodes.
	buildTestDBs(t, 1e3, goodDB, badDB) // Build Test DBs

	addr := &net.UnixAddr{Name: filepath.Join(t.TempDir(), "socket.sock"), Net: "unix"}
	errg, ctx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		return serveKeyValueStore(ctx, goodDB, addr)
	})

	// Sync
	err := fs.ErrNotExist
	for errors.Is(err, fs.ErrNotExist) {
		runtime.Gosched()
		_, err = os.Stat(addr.Name)
	}
	require.NoError(t, err)
	start := time.Now()
	require.NoError(t, syncRemote(badDB, addr, t.TempDir(), func(s string) { fmt.Println(s) }))
	fmt.Printf("Sync took %v", time.Since(start))
	cancel()
	require.NoError(t, errg.Wait())

	// Verify
	checkDb(t, goodDB, badDB)
	checkDb(t, badDB, goodDB)
}

func checkDb(t testing.TB, a, b keyvalue.Store) {
	t.Helper()
	require.NoError(t, a.ForEach(func(key *record.Key, expect []byte) error {
		actual, err := b.Get(key)
		require.NoError(t, err)
		require.Equal(t, expect, actual)
		return nil
	}))
}

func buildTestDBs(t testing.TB, numEntries int, gDB, bDB keyvalue.Beginner) (added, modified, missing uint64) {
	fmt.Println("Build Test DBs")
	var rh1, mrh common.RandHash              // rh adds the good key vale pairs
	mrh.SetSeed([]byte{1, 4, 67, 8, 3, 5, 7}) // mrh with a different seed, adds bad data

	start := time.Now()
	var total int
	var cntMod, cntDel, cntAdd int
	var lastPercent int

	fmt.Printf("\nGenerating databases with about %d entries:\n", numEntries)

	update := func(db keyvalue.Beginner, fn func(txn keyvalue.ChangeSet)) {
		batch := db.Begin(nil, true)
		defer batch.Discard()
		fn(batch)
		require.NoError(t, batch.Commit())
	}

	for i := 0; i < numEntries; i++ {
		size := rh1.GetIntN(512) + 128
		total += size
		value := rh1.GetRandBuff(size)

		// Write the good entries
		key := record.NewKey(i)
		update(gDB, func(txn keyvalue.ChangeSet) {
			require.NoError(t, txn.Put(key, value))
		})

		// Write the bad entries

		op := 0 // Match the good db
		switch {
		case i%57 == 0:
			op = 1 // Modify a key value pair
		case i%73 == 0:
			op = 2 // Delete a key value pair
		case i%33 == 0:
			op = 3 // Add a key value pair
		}

		// Write bad entries
		if bDB != nil {
			update(bDB, func(txn keyvalue.ChangeSet) {
				switch op {
				case 1:
					// Modify a key value pair
					v := make([]byte, len(value))
					copy(v, value)
					v[0]++
					require.NoError(t, txn.Put(key, v))
					cntMod++
				case 2:
					// Delete a key value pair
					cntDel++
				case 3:
					// Add an extra key value pair
					require.NoError(t, txn.Put(key, value))
					require.NoError(t, txn.Put(record.NewKey(i+numEntries), value))
					cntAdd++
				default:
					// Normal
					require.NoError(t, txn.Put(key, value))
				}
			})
		}

		percent := i * 100 / numEntries
		if percent > lastPercent {
			if percent%5 == 0 {
				fmt.Printf("%3d%%", percent)
			}
			lastPercent = percent
		}

	}
	fmt.Println(" 100%")
	fmt.Printf("\nFINAL: #keys: %d time: %v size: %d\n", numEntries, time.Since(start), total)
	fmt.Printf("\nThe test modified %d keys, deleted %d keys, and added %d keys.\n", cntMod, cntDel, cntAdd)
	fmt.Printf("\nAs far as a fix is concerned, the bad DB has:\n")
	fmt.Printf("Modified: %d Added: %d\n", cntMod+cntDel, cntAdd)
	return uint64(cntAdd), uint64(cntMod), uint64(cntDel)
}

func TestFindMissingTxns(t *testing.T) {
	t.Skip("Manual")
	for _, s := range []string{"directory", "apollo", "yutu", "chandrayaan"} {
		f, err := os.Create("../../../.nodes/restore/" + s + "-missing.csv")
		require.NoError(t, err)
		defer f.Close()

		rd := openSnapshotFile("../../../.nodes/restore/" + s + "-genesis.json")
		if c, ok := rd.(io.Closer); ok {
			defer c.Close()
		}

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
							require.NoError(t, ms.UnmarshalBinary(re.Value))
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
	t.Skip("Manual")
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
