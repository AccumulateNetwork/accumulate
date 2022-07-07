package factom

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"

	f2 "github.com/FactomProject/factom"
	"github.com/FactomProject/factomd/common/adminBlock"
	"github.com/FactomProject/factomd/common/directoryBlock"
	"github.com/FactomProject/factomd/common/entryBlock"
	"github.com/FactomProject/factomd/common/entryCreditBlock"
	"github.com/FactomProject/factomd/common/factoid"
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
		fmt.Println("Done. ", fileCnt, " files processed")
		return false
	}
	fileNumber += FileIncrement
	fileCnt++
	n, err := f.Read(Buff[:])
	buff = Buff[:n]
	f.Close()
	fmt.Println("Processing ", filename, " Reading ", n, " bytes.")
	return true
}

func Process(server string) {
	var cg ChainGang
	header := new(Header)
	dBlock := directoryBlock.NewDirectoryBlock(nil)
	aBlock := adminBlock.NewAdminBlock(nil)
	fBlock := new(factoid.FBlock)
	ecBlock := entryCreditBlock.NewECBlock()
	eBlock := entryBlock.NewEBlock()

	for Open() {
		_, _, _, _, _ = dBlock, aBlock, fBlock, ecBlock, eBlock
		for len(buff) > 0 {
			buff = header.UnmarshalBinary(buff)
			switch header.Tag {
			case TagDBlock:
				if err := dBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
					panic("Bad Directory block")
				}
			case TagABlock:
				if err := aBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
					fmt.Printf("Ht %d Admin size %d %v \n",
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
				factomChainData = make(map[[32]byte]*Queue)
				accountId, err := hex.DecodeString(qEntry.ChainID)
				if err != nil {
					log.Fatalf("cannot decode account id")
				}
				dataEntry := ConvertFactomDataEntryToLiteDataEntry(*qEntry)
				ch := cg.GetOrCreateChainWorker(server, (*[32]byte)(accountId), 1)
				ch <- dataEntry
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
		cg.Close()
	}
}
