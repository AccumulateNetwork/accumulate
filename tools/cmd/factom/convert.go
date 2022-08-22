package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/FactomProject/factomd/common/entryBlock"
	"github.com/FactomProject/factomd/common/interfaces"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom"
)

var cmdConvert = &cobra.Command{
	Use:   "convert [output database] [output snapshot] [input object file directory]",
	Short: "convert a Factom object dump to Accumulate",
	Args:  cobra.ExactArgs(3),
	Run:   convert,
}

func init() {
	cmd.AddCommand(cmdConvert)
}

func convert(_ *cobra.Command, args []string) {
	entryCount := 0
	FCTHeight := 0

	db, err := database.OpenBadger(args[0], nil)
	checkf(err, "output database")
	defer db.Close()

	output, err := os.Create(args[1])
	checkf(err, "output file")
	defer output.Close()

	start := time.Now().Unix()
	for {
		runTime := time.Now().Unix() - start
		if runTime > 0 {
			hours := runTime / 60 / 60
			minutes := (runTime - hours*60*60) / 60
			seconds := runTime % 60

			fmt.Printf("FCT Blocks\t\t%d\nEntries\t\t\t%d\nTime\t\t\t%d:%02d:%02d\n",
				FCTHeight, entryCount, hours, minutes, seconds)
			fmt.Printf("BLK/s\t\t\t%d\nEntries/s per Second\t%d\n\n",
				int64(FCTHeight)/runTime, int64(entryCount)/runTime)
		}

		filename := fmt.Sprintf("objects-%d.dat", FCTHeight)
		FCTHeight += 2000

		filename = filepath.Join(args[2], filename)

		input, err := ioutil.ReadFile(filename)
		checkf(err, "read %s", filename)

		type EntryData struct {
			EntryTime time.Time //derived timestamp of entry
			Entry     *entryBlock.Entry
		}

		// Create a map of all the entries in the object file
		entries := map[[32]byte][]*EntryData{}

		eblocks := map[[32]byte]*entryBlock.EBlock{}
		ebEntryIndex := map[[32]byte]int{} //duplicates tracker
		blockTime := time.Time{}
		blockHeight := uint32(0)

		err = factom.ReadObjectFile(input, func(header *factom.Header, object interface{}) {
			switch header.Tag {
			case factom.TagDBlock:
				idb, ok := object.(interfaces.IDirectoryBlock)
				if !ok {
					panic("expected directory block")
				}
				blockTime = idb.GetTimestamp().GetTime()
				blockHeight = idb.GetHeader().GetDBHeight()
				//reset index tracking map
				ebEntryIndex = map[[32]byte]int{}
			case factom.TagEBlock:
				eb, ok := object.(*entryBlock.EBlock)
				if !ok {
					panic("expected entry block")
				}
				id := eb.GetChainID().Fixed()
				eblocks[id] = eb
			case factom.TagEntry:
				entry, ok := object.(*entryBlock.Entry)
				if !ok {
					panic("expected entry")
				}
				id := entry.ChainID.Fixed()
				//look for entry in entry block to figure out minute, need to handle duplicate entries
				//since we don't have any more resolution we'll just divide the number of entries in the minute
				//min by the count to give us some arbitrary resolution in seconds.
				minute := byte(0)
				found := false
				entriesInMinute := 0
				var entryHash [32]byte

				//count the number of entries per minute for a given block
				var entryCountInMinute [10]byte
				j := 0
				for _, e := range eblocks[id].GetBody().GetEBEntries() {
					if e.IsMinuteMarker() {
						j++
						continue
					}
					entryCountInMinute[j]++
				}

				//now search for the time marker of the entry, handle duplicate entries
				for i, e := range eblocks[id].GetBody().GetEBEntries() {
					entryHash = entry.GetHash().Fixed()
					if e.IsMinuteMarker() {
						minute = e.ToMinute()
						entriesInMinute = 0
						continue
					} else if bytes.Compare(e.Bytes(), entryHash[:]) == 0 {
						//we have a match, but is it a duplicate? if so, continue on to look for next match
						if ebEntryIndex[entryHash] <= i {
							ebEntryIndex[entryHash] = i
							found = true
							break
						}
					}
					entriesInMinute++
				}

				ed := EntryData{blockTime, entry}
				if found {
					//entry timestamp down to the minute
					ed.EntryTime = ed.EntryTime.Add(time.Minute*time.Duration(minute) +
						//entry timestamp to order entry within minute.
						//Option1: use arbitrary sub-minute time based on index, but uniqueness is provided
						//time.Duration(float64(time.Minute)*float64(ebEntryIndex[entryHash])/float64(entryCountInMinute[minute]+1)))
						//Option2: simply use the index into the minute as a microsecond (this option looks much cleaner)
						time.Microsecond*time.Duration(ebEntryIndex[entryHash]+1))
				} else {
					fatalf("cannot find entry in entry block, %x at height %d", entry.GetHash().Bytes(), blockHeight)
				}

				entries[id] = append(entries[id], &ed)
			default:
				return
			}

		})
		checkf(err, "process object file")

		// For each chain ID
		for chainId, entriesAll := range entries {
			// Format the URL
			chainId := chainId // See docs/developer/rangevarref.md
			address, err := protocol.LiteDataAddress(chainId[:])
			checkf(err, "create LDA URL")

			for len(entriesAll) > 0 {
				var entries []*EntryData
				if len(entriesAll) > 1000 {
					entries = entriesAll[:1000]
					entriesAll = entriesAll[1000:]
				} else {
					entriesAll = entriesAll[:0]
				}

				// Commit each account separately so we don't exceed Badger's limits
				batch := db.Begin(true)
				account := batch.Account(address)

				// Create the LDA's main record if it doesn't exist
				var lda *protocol.LiteDataAccount
				err = account.Main().GetAs(&lda)
				switch {
				case err == nil:
					// Record exists
				case errors.Is(err, errors.StatusNotFound):
					// Create the record
					lda = new(protocol.LiteDataAccount)
					lda.Url = address
					err = account.Main().Put(lda)
					checkf(err, "store record")
				default:
					checkf(err, "load record")
				}

				// For each entry
				for _, e := range entries {
					entryCount++
					// Convert the entry and calculate the entry hash
					entry := factom.ConvertEntry(e.Entry).Wrap()
					entryHash, err := protocol.ComputeFactomEntryHashForAccount(chainId[:], entry.GetData())
					checkf(err, "calculate entry hash")

					// Construct a transaction
					txn := new(protocol.Transaction)
					txn.Header.Principal = address
					txn.Body = &protocol.WriteData{Entry: entry}

					// Each transaction needs to be unique so add a timestamp which is the timestamp from the Factom chain
					txn.Header.Memo = fmt.Sprintf("%v", e.EntryTime.UTC())

					// Construct the transaction result
					result := new(protocol.WriteDataResult)
					result.AccountID = chainId[:]
					result.AccountUrl = address
					result.EntryHash = *(*[32]byte)(entryHash)

					// Construct the transaction status
					status := new(protocol.TransactionStatus)
					status.TxID = txn.ID()
					status.Code = errors.StatusDelivered
					status.Result = result

					// Check if the transaction already exists
					txnrec := batch.Transaction(txn.GetHash())
					_, err = txnrec.Main().Get()
					switch {
					case err == nil:
						fatalf("Somehow we created a duplicate transaction")
					case errors.Is(err, errors.StatusNotFound):
						// Ok
					default:
						checkf(err, "check for duplicate transaction")
					}

					// Write everything to the database
					err = indexing.Data(batch, address).Put(entryHash, txn.GetHash())
					checkf(err, "add data index")

					err = txnrec.Main().Put(&database.SigOrTxn{Transaction: txn})
					checkf(err, "store transaction")

					err = txnrec.Status().Put(status)
					checkf(err, "store status")

					mainChain, err := account.MainChain().Get()
					checkf(err, "load main chain")
					err = mainChain.AddEntry(txn.GetHash(), false)
					checkf(err, "store main chain entry")
				}

				err = batch.Commit()
				checkf(err, "commit")
			}
		}

		// Do a GC after each object file to keep badger under control
		err = db.GC(0.5)
		if err != nil && !errors.Is(err, badger.ErrNoRewrite) {
			checkf(err, "compact database")
		}
	}

	// Create a snapshot
	check(db.View(func(batch *database.Batch) error {
		return batch.SaveFactomSnapshot(output)
	}))
}
