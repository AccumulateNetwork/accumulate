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
	header := new(Header)
	dBlock := directoryBlock.NewDirectoryBlock(nil)
	aBlock := adminBlock.NewAdminBlock(nil)
	fBlock := new(factoid.FBlock)
	ecBlock := entryCreditBlock.NewECBlock()
	eBlock := entryBlock.NewEBlock()
	for len(buff) > 0 {
		buff = header.UnmarshalBinary(buff)
		switch header.Tag {
		case TagDBlock:
			if err := dBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal directory block: %w", err)
			} else {
				fn(header, dBlock)
			}
		case TagABlock:
			if err := aBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				// Why?
				log.Printf("Ht %d Admin size %d: %v\n", dBlock.GetHeader().GetDBHeight(), header.Size, err)
				// return fmt.Errorf("unmarshal admin block: %w", err)
			} else {
				fn(header, aBlock)
			}
		case TagFBlock:
			if err := fBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal factoid block: %w", err)
			} else {
				fn(header, fBlock)
			}
		case TagECBlock:
			if err := ecBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal entry credit block: %w", err)
			} else {
				fn(header, ecBlock)
			}
		case TagEBlock:
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
