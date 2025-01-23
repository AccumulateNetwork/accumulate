// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package factom

import (
	"fmt"

	"github.com/FactomProject/factomd/common/adminBlock"
	"github.com/FactomProject/factomd/common/directoryBlock"
	"github.com/FactomProject/factomd/common/entryBlock"
	"github.com/FactomProject/factomd/common/entryCreditBlock"
	"github.com/FactomProject/factomd/common/factoid"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func ReadObjectFile(buff []byte, logger log.Logger, fn func(header *Header, object interface{})) error {
	if logger == nil {
		logWriter, err := logging.NewConsoleWriter("plain")
		if err != nil {
			panic(err)
		}
		logger, err = logging.NewTendermintLogger(zerolog.New(logWriter), "info", false)
		if err != nil {
			panic(err)
		}
	}

	for len(buff) > 0 {
		header := new(Header)
		buff = header.UnmarshalBinary(buff)
		switch header.Tag {
		case TagDBlock:
			dBlock := directoryBlock.NewDirectoryBlock(nil)
			if err := dBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal directory block: %w", err)
			} else {
				logger.Debug("Directory block", "height", dBlock.GetDatabaseHeight())
				fn(header, dBlock)
			}
		case TagABlock:
			aBlock := adminBlock.NewAdminBlock(nil)
			if err := aBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal admin block: %w", err)
			} else {
				logger.Debug("Admin block", "height", aBlock.GetDatabaseHeight())
				fn(header, aBlock)
			}
		case TagFBlock:
			fBlock := new(factoid.FBlock)
			if err := fBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal factoid block: %w", err)
			} else {
				logger.Debug("Factoid block", "height", fBlock.GetDatabaseHeight())
				fn(header, fBlock)
			}
		case TagECBlock:
			ecBlock := entryCreditBlock.NewECBlock()
			if err := ecBlock.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal entry credit block: %w", err)
			} else {
				logger.Debug("Entry credit block", "height", ecBlock.GetDatabaseHeight())
				fn(header, ecBlock)
			}
		case TagEBlock:
			eBlock := entryBlock.NewEBlock()
			if _, err := eBlock.UnmarshalBinaryData(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal entry block: %w", err)
			} else {
				logger.Debug("Entry block", "height", eBlock.GetDatabaseHeight())
				fn(header, eBlock)
			}
		case TagEntry:
			entry := new(entryBlock.Entry)
			if err := entry.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal entry: %w", err)
			} else {
				logger.Debug("Entry", "chain", logging.AsHex(entry.ChainID))
				fn(header, entry)
			}
		case TagTX:
			tx := new(factoid.Transaction)
			if err := tx.UnmarshalBinary(buff[:header.Size]); err != nil {
				return fmt.Errorf("unmarshal transaction: %w", err)
			} else {
				logger.Debug("Factoid transaction", "hash", logging.AsHex(tx.Txid.Bytes()))
				fn(header, tx)
			}
		default:
			logger.Error("Unknown object", "tag", header.Tag, "size", header.Size)
		}
		if int(header.Size) > len(buff) {
			return fmt.Errorf("invalid entry: size is greater than remaining buffer")
		}
		buff = buff[header.Size:]
	}
	return nil
}
