package factom

import (
	"fmt"
	"log"

	"github.com/FactomProject/factomd/common/adminBlock"
	"github.com/FactomProject/factomd/common/directoryBlock"
	"github.com/FactomProject/factomd/common/entryBlock"
	"github.com/FactomProject/factomd/common/entryCreditBlock"
	"github.com/FactomProject/factomd/common/factoid"
)

func ReadObjectFile(buff []byte, fn func(header *Header, object interface{})) error {
	var lastHeight uint32
	for len(buff) > 0 {
		header := new(Header)
		buff = header.UnmarshalBinary(buff)
		switch header.Tag {
		case TagDBlock:
			dBlock := directoryBlock.NewDirectoryBlock(nil)
			if err := dBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal directory block: %w", err)
			} else {
				lastHeight = dBlock.GetHeader().GetDBHeight()
				fn(header, dBlock)
			}
		case TagABlock:
			aBlock := adminBlock.NewAdminBlock(nil)
			if err := aBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				// Why?
				log.Printf("Ht %d Admin size %d: %v\n", lastHeight, header.Size, err)
				// return fmt.Errorf("unmarshal admin block: %w", err)
			} else {
				fn(header, aBlock)
			}
		case TagFBlock:
			fBlock := new(factoid.FBlock)
			if err := fBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal factoid block: %w", err)
			} else {
				fn(header, fBlock)
			}
		case TagECBlock:
			ecBlock := entryCreditBlock.NewECBlock()
			if err := ecBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal entry credit block: %w", err)
			} else {
				fn(header, ecBlock)
			}
		case TagEBlock:
			eBlock := entryBlock.NewEBlock()
			if _, err := eBlock.UnmarshalBinaryData(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal entry block: %w", err)
			} else {
				fn(header, eBlock)
			}
		case TagEntry:
			entry := new(entryBlock.Entry)
			if err := entry.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal entry: %w", err)
			} else {
				fn(header, entry)
			}
		case TagTX:
			tx := new(factoid.Transaction)
			if err := tx.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal transaction: %w", err)
			} else {
				fn(header, tx)
			}
		default:
			return fmt.Errorf("unknown object %v", header.Tag)
		}
		buff = buff[header.Size:]
	}
	return nil
}
