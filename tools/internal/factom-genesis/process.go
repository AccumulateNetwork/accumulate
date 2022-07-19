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
	"github.com/FactomProject/factomd/common/adminBlock"
	"github.com/FactomProject/factomd/common/directoryBlock"
	"github.com/FactomProject/factomd/common/entryBlock"
	"github.com/FactomProject/factomd/common/entryCreditBlock"
	"github.com/FactomProject/factomd/common/factoid"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
	header := new(Header)
	dBlock := directoryBlock.NewDirectoryBlock(nil)
	aBlock := adminBlock.NewAdminBlock(nil)
	fBlock := new(factoid.FBlock)
	ecBlock := entryCreditBlock.NewECBlock()
	eBlock := entryBlock.NewEBlock()

	for Open() {
		_, _, _, _, _ = dBlock, aBlock, fBlock, ecBlock, eBlock

		t := time.Now()
		transactions := map[[32]byte][]*protocol.Transaction{}
		var count int
		for len(buff) > 0 {
			buff = header.UnmarshalBinary(buff)
			switch header.Tag {
			case TagDBlock:
				if err := dBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
					panic("Bad Directory block")
				}
			case TagABlock:
				if err := aBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
					log.Printf("Ht %d Admin size %d %v \n",
						dBlock.GetHeader().GetDBHeight(), header.Size, err)
				}
			case TagFBlock:
				if err := fBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
					panic("Bad Factoid block")
				}
			case TagECBlock:
				if err := ecBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
					panic("Bad Entry Credit block")
				}
			case TagEBlock:
				if _, err := eBlock.UnmarshalBinaryData(buff[:header.Size]); err != nil {
					panic("Bad Entry Block block")
				}
			case TagEntry:
				entry := new(entryBlock.Entry)
				if err := entry.UnmarshalBinary(buff[:header.Size]); err != nil {
					panic("Bad Entry")
				}
				qEntry := &f2.Entry{
					ChainID: entry.ChainID.String(),
					ExtIDs:  entry.ExternalIDs(),
					Content: entry.GetContent(),
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
				count++

			case TagTX:
				tx := new(factoid.Transaction)
				if err := tx.UnmarshalBinary(buff[:header.Size]); err != nil {
					panic("Bad Transaction")
				}
			default:
				panic("Unknown TX encountered")
			}
			buff = buff[header.Size:]
		}
		fmt.Printf("Constructed %d transactions for %d accounts in %v\n", count, len(transactions), time.Since(t))

		// Submit the first transaction in one block, then all the rest in blocks of 100
		blocks := make([][]*protocol.Transaction, 2)
		block := 1
		const blockSize = 50
		for _, transactions := range transactions {
			blocks[0] = append(blocks[0], transactions[0])
			// blocks[block] = append(blocks[block], transactions[1:]...)
			for _, txn := range transactions[1:] {
				if len(blocks[block]) >= blockSize {
					block++
					blocks = append(blocks, nil)
				}
				blocks[block] = append(blocks[block], txn)
			}
		}

		// Submit the blocks
		timestamp := uint64(time.Now().UnixMilli())
		allEnvelopes := make([]*protocol.Envelope, 0, count)
		for i, block := range blocks {
			var t time.Time
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

			t = time.Now()
			fmt.Printf("Executing block %d with %d transactions...", i+1, len(block))
			simul.MustSubmitAndExecuteBlock(envelopes...)
			fmt.Printf(" took %v\n", time.Since(t))
		}

		// Wait for everything to complete
		st, txn := simul.WaitForTransactions(delivered, allEnvelopes...)
		for i, st := range st {
			if !st.Failed() {
				continue
			}

			fmt.Printf("Transaction %x for %v failed: %v\n", txn[i].GetHash()[:4], txn[i].Header.Principal, st.Error)
		}
	}
}
