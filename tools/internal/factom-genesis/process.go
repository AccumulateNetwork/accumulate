package factom

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	f2 "github.com/FactomProject/factom"
	"github.com/FactomProject/factomd/common/entryBlock"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom"
)

const FileIncrement = 2000

var Buff [1000000000]byte // Buffer to read a file
var buff []byte           // Slice to process the buffer
var fileNumber int
var fileCnt int

// Open
// Open a FactomObjects file.  Returns false if all files have been
// processed.
func Open() bool {
	u, err := user.Current()
	if err != nil {
		panic("no user")
	}
	filename := filepath.Join(u.HomeDir, "tmp", "FactomObjects", fmt.Sprintf("objects-%d.dat", fileNumber))
	f, err := os.OpenFile(filename, os.O_RDONLY, 07666)
	if err != nil {
		log.Println("Done. ", fileCnt, " files processed")
		return false
	}
	fileNumber += FileIncrement
	fileCnt++
	n, err := f.Read(Buff[:])
	if err != nil {
		log.Println("Error reading buff : ", err.Error())
	}
	buff = Buff[:n]
	f.Close()
	log.Println("Processing ", filename, " Reading ", n, " bytes.")
	return true
}

//nolint:noprint
func Process() {
	t := time.Now().Unix()

	var entryCount int64
	var entriesSkipped int64
	for Open() {
		clock := time.Now().Unix() - t
		if clock > 0 {
			blocksPerSec := float64(FileIncrement) / float64(clock)
			entriesPerSec := float64(entryCount) / float64(clock)
			fmt.Printf("%4.1f Factom blocks/s & %8.1f Factom entries/s \n", blocksPerSec, entriesPerSec)

			Hrs := clock / 60 / 60
			Mins := (clock - Hrs*60*60) / 60
			Secs := clock - Hrs*60*60 - Mins*60
			fmt.Printf("Run time %d:%02d:%02d\n", Hrs, Mins, Secs)
		}

		transactions := map[[32]byte][]*protocol.Transaction{}

		err := factom.ReadObjectFile(buff, nil, func(_ *factom.Header, object interface{}) {
			switch object := object.(type) {
			case *entryBlock.Entry:
				qEntry := &f2.Entry{
					ChainID: object.ChainID.String(),
					ExtIDs:  object.ExternalIDs(),
					Content: object.GetContent(),
				}
				accountId, err := hex.DecodeString(qEntry.ChainID)
				if err != nil {
					log.Fatalf("cannot decode account id")
				}
				account, err := protocol.LiteDataAddress(accountId[:])
				if err != nil {
					log.Fatalf("error creating lite address %x, %v", accountId, err)
				}

				dataEntry := ConvertFactomDataEntryToLiteDataEntry(*qEntry)
				txn := ConstructWriteData(account, dataEntry)
				transactions[account.AccountID32()] = append(transactions[account.AccountID32()], txn)
				entryCount++
				if entryCount%10000 == 0 {
					fmt.Printf("Entries: %8d %8d\n", entryCount, entriesSkipped)
				}
			}
		})
		if err != nil {
			panic(err)
		}
		fmt.Printf("Entries: %8d %8d\n", entryCount, entriesSkipped)

		// Submit the first transaction in one block, then all the rest in blocks of 100
		blocks := make([][]*protocol.Transaction, 2)
		block := 1

		const blockSize = 10300
		size := 0
		tCnt := 0
		for _, transactions := range transactions {
			blocks[0] = append(blocks[0], transactions[0])
			// blocks[block] = append(blocks[block], transactions[1:]...)
			for _, txn := range transactions[1:] {
				tCnt++
				bTxn, _ := txn.MarshalBinary()
				size += len(bTxn)
				if size >= blockSize {
					block++
					blocks = append(blocks, nil)
					size = len(bTxn)
				}
				blocks[block] = append(blocks[block], txn)
			}
		}

		fmt.Printf("Submitting %d blocks with %d transactions.", len(blocks), tCnt)

		// Submit the blocks
		timestamp := uint64(time.Now().UnixMilli())
		allEnvelopes := make([]*protocol.Envelope, 0, entryCount)
		for i, block := range blocks {
			_, _ = i, t
			// t = time.Now()
			envelopes := make([]*protocol.Envelope, len(block))
			for j, txn := range block {
				envelopes[j] = acctesting.NewTransaction().
					WithPrincipal(txn.Header.Principal).
					WithTimestampVar(&timestamp).
					WithSigner(origin, 1).
					WithBody(txn.Body).
					Initiate(protocol.SignatureTypeED25519, key.PrivateKey).
					Build()
			}
			allEnvelopes = append(allEnvelopes, envelopes...)
			// fmt.Printf("Signed %d transactions in %v\n", len(block), time.Since(t))

			//fmt.Printf("Executing block %d with %d transactions...", i+1, len(block))
			simul.MustSubmitAndExecuteBlock(envelopes...)
			//fmt.Printf(" took %v\n", time.Since(t))

			if i%500 == 499 {
				// Compact badger
				simul.GC(0.5)

				fmt.Printf(" %d", len(blocks)-i)
				st, txn := simul.WaitForTransactions(delivered, allEnvelopes...)
				for i, st := range st {
					if !st.Failed() {
						continue
					}

					fmt.Printf("\nTransaction %x for %v failed: %v\n", txn[i].GetHash()[:4], txn[i].Header.Principal, st.Error)
				}
				allEnvelopes = allEnvelopes[:0]
			}
		}

		fmt.Printf(" %d", len(blocks))

		// Wait for everything to complete
		st, txn := simul.WaitForTransactions(delivered, allEnvelopes...)
		for i, st := range st {
			if !st.Failed() {
				continue
			}

			fmt.Printf("\nTransaction %x for %v failed: %v\n", txn[i].GetHash()[:4], txn[i].Header.Principal, st.Error)
		}

		fmt.Print(" Done\n")
	}
}
