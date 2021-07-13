// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package factom

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulated/scratch/factom/varintf"
)

// FBlock represents a Factoid Block.
type FBlock struct {
	// Computed Fields
	KeyMR       *Bytes32
	LedgerKeyMR *Bytes32

	// Header Fields
	BodyMR          *Bytes32
	PrevKeyMR       *Bytes32
	PrevLedgerKeyMR *Bytes32

	ECExchangeRate uint64
	Height         uint32

	// Expansion is the expansion space in the FBlock. If we do not parse
	// the expansion, we just store the raw bytes.
	Expansion Bytes

	// Number of bytes contained in the body
	bodySize uint32

	// Body Fields
	Transactions []Transaction

	// Timestamp is established by the DBlock. It is only populated if the
	// FBlock was unmarshaled from within a DBlock.
	Timestamp time.Time

	// Other fields
	//
	// End of Minute transaction heights. They mark the height of the first
	// tx index of the NEXT period. This tx may not exist. The Coinbase
	// transaction is considered to be in the first period. Factom's
	// periods will initially be a minute long, and there will be 10 of
	// them. This may change in the future.
	endOfPeriod [10]int

	// marshalBinaryCache is the binary data of the FBlock. It is cached by
	// UnmarshalBinary so it can be re-used by MarshalBinary.
	marshalBinaryCache []byte
}

// ClearMarshalBinaryCache discards the cached MarshalBinary data.
//
// Subsequent calls to MarshalBinary will re-construct the data from the fields
// of the FBlock.
func (fb *FBlock) ClearMarshalBinaryCache() {
	fb.marshalBinaryCache = nil
}

// IsPopulated returns true if fb has already been successfully populated by a
// call to Get. IsPopulated returns false if fb.Transactions is nil.
func (fb FBlock) IsPopulated() bool {
	return len(fb.Transactions) > 0 && // FBlocks always contain at least a coinbase
		fb.BodyMR != nil &&
		fb.PrevKeyMR != nil &&
		fb.PrevLedgerKeyMR != nil
}

// Get queries factomd for the Factoid Block at fb.Header.Height or fb.KeyMR.
// After a successful call, the Transactions will all be populated.
func (fb *FBlock) Get(ctx context.Context, c *Client) (err error) {
	if fb.IsPopulated() {
		return nil
	}

	if fb.KeyMR != nil {
		params := struct {
			Hash *Bytes32 `json:"hash"`
		}{Hash: fb.KeyMR}
		var result struct {
			Data Bytes `json:"data"`
		}
		if err := c.FactomdRequest(ctx, "raw-data", params, &result); err != nil {
			return err
		}
		return fb.UnmarshalBinary(result.Data)
	}

	params := struct {
		Height uint32 `json:"height"`
	}{fb.Height}
	result := struct {
		// We will ignore all the other fields, and just unmarshal from the raw.
		RawData Bytes `json:"rawdata"`
	}{}
	if err := c.FactomdRequest(ctx, "fblock-by-height", params, &result); err != nil {
		return err
	}

	return fb.UnmarshalBinary(result.RawData)
}

const (
	// FBlockHeaderMinSize is the minimum expected FBlock Header Size.
	FBlockHeaderMinSize = 32 + // Factoid ChainID
		32 + // BodyMR
		32 + // PrevKeyMR
		32 + // PrevLedgerKeyMR
		8 + // EC Exchange Rate
		4 + // DB Height
		1 + // Header Expansion size (varint)
		0 + // Header Expansion Area (Min 0)
		4 + // Transaction Count
		4 // Body Size
)

const (
	// FBlockMinuteMarker is the byte used to indicate the end of a minute
	// in the FBlock.
	FBlockMinuteMarker = 0x00
)

// UnmarshalBinary unmarshals raw directory block data.
//
// Header
//      [Factoid Block ChainID (Bytes32{31:0x0f})] +
//      [BodyMR (Bytes32)] +
//      [PrevKeyMR (Bytes32)] +
//      [PrevLedgerKeyMR (Bytes32)] +
//      [Exchange Rate (8 bytes)] +
//      [DB Height (4 bytes)] +
//      [Header Expansion size (Bytes)] +
//      [Header Expansion Area (Bytes)] +
//      [Transaction Count (4 bytes)] +
//      [Body Size (4 bytes)] +
//
// Body
//      [Tx 0 (Bytes)] +
//      ... +
//      [Tx N (Bytes)] +
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#factoid-block
func (fb *FBlock) UnmarshalBinary(data []byte) (err error) {
	if len(data) < FBlockHeaderMinSize {
		return fmt.Errorf("insufficient length")
	}

	if bytes.Compare(data[:32], fBlockChainID[:]) != 0 {
		return fmt.Errorf("invalid factoid chainid")
	}

	i := 32

	fb.BodyMR = new(Bytes32)
	i += copy(fb.BodyMR[:], data[i:])

	fb.PrevKeyMR = new(Bytes32)
	i += copy(fb.PrevKeyMR[:], data[i:])

	fb.PrevLedgerKeyMR = new(Bytes32)
	i += copy(fb.PrevLedgerKeyMR[:], data[i:])

	fb.ECExchangeRate = binary.BigEndian.Uint64(data[i : i+8])
	i += 8

	fb.Height = binary.BigEndian.Uint32(data[i : i+4])
	i += 4

	expansionSize, read := varintf.Decode(data[i:])
	if read < 0 {
		return fmt.Errorf("expansion size is not a valid varint")
	}
	i += read

	// sanity check, if the expansion size is greater than all the data we
	// have, less 8 bytes for the tx count and body size, then the
	// expansion size was bogus.
	if expansionSize > uint64(len(data[i:])-8) {
		return fmt.Errorf("expansion size is larger than remaining data")
	}
	// This should be a safe cast to int, as the size is never > max int
	// For these type assertions to fail on a 32 bit system, we would need a
	// 4gb factoid block.
	fb.Expansion = data[i : i+int(expansionSize)]
	i += int(expansionSize)

	txCount := binary.BigEndian.Uint32(data[i : i+4])
	i += 4
	fb.bodySize = binary.BigEndian.Uint32(data[i : i+4])
	i += 4

	// If the declared txCount would require
	if int(txCount*TransactionMinTotalSize) > len(data) {
		return fmt.Errorf("unreasonable Transaction count")

	}

	// Header is all data we've read so far
	headerHash := sha256.Sum256(data[:i])
	bodyMRElements := make([][]byte, int(txCount)+len(fb.endOfPeriod))
	bodyLedgerMRElements := make([][]byte, int(txCount)+len(fb.endOfPeriod))

	fb.Transactions = make([]Transaction, txCount)
	var period int
	for c := range fb.Transactions {
		// Before each fct tx, we need to see if there is a marker byte that
		// indicates a minute marker
		for data[i] == FBlockMinuteMarker {
			if period > len(fb.endOfPeriod) {
				return fmt.Errorf("too many minute markers")
			}
			fb.endOfPeriod[period] = c
			bodyMRElements[c+period] = []byte{FBlockMinuteMarker}
			bodyLedgerMRElements[c+period] = []byte{FBlockMinuteMarker}
			period++ // The next period encountered will be the next minute
			i++
		}

		tx := &fb.Transactions[c]
		if err := tx.UnmarshalBinary(data[i:]); err != nil {
			return err
		}
		read := tx.MarshalBinaryLen()

		tx.Timestamp = fb.Timestamp.Add(time.Duration(period) * MinuteDuration)

		// Append the elements for MR calculation
		bodyMRElements[c+period] = data[i : i+read]
		// Calc the signature size
		var sigSize int
		for _, o := range fb.Transactions[c].Signatures {
			sigSize += len(o.RCD) + len(o.Signature)
		}
		bodyLedgerMRElements[c+period] = data[i : i+(read-sigSize)]

		i += read
	}

	// Finish the minute markers
	for period < len(fb.endOfPeriod) {
		period++
		idx := int(txCount) + period - 1
		bodyMRElements[idx] = []byte{FBlockMinuteMarker}
		bodyLedgerMRElements[idx] = []byte{FBlockMinuteMarker}
	}

	// If we have not hit the end of our periods, a single byte will remain
	for period < len(fb.endOfPeriod) {
		fb.endOfPeriod[period] = int(txCount)
		period++ // The next period encountered will be the next minute
		i++
	}

	// Merkle Root Calculations
	bodyMR, err := ComputeFBlockBodyMR(bodyMRElements)
	if err != nil {
		return err
	}

	bodyLedgerMR, err := ComputeFBlockBodyMR(bodyLedgerMRElements)
	if err != nil {
		return err
	}

	// Set out computed fields
	keyMr, err := ComputeFBlockKeyMR([][]byte{headerHash[:], bodyMR[:]})
	if err != nil {
		return err
	}
	ledgerMr, err := ComputeFBlockKeyMR([][]byte{bodyLedgerMR[:], headerHash[:]})
	if err != nil {
		return err
	}

	if fb.KeyMR == nil {
		fb.KeyMR = &keyMr
	} else if keyMr != *fb.KeyMR {
		return fmt.Errorf("invalid keyMR")
	}

	fb.LedgerKeyMR = &ledgerMr
	fb.marshalBinaryCache = data

	return nil
}

// MarshalBinary marshals the FBlock into its binary form. If the FBlock was
// orignally Unmarshaled, then the cached data is re-used, so this is
// efficient. See ClearMarshalBinaryCache.
func (fb *FBlock) MarshalBinary() ([]byte, error) {
	if fb.marshalBinaryCache != nil {
		return fb.marshalBinaryCache, nil
	}

	if !fb.IsPopulated() {
		return nil, fmt.Errorf("not populated")
	}

	// Header
	expansionSize := varintf.Encode(uint64(len(fb.Expansion)))
	data := make([]byte, FBlockHeaderMinSize+
		len(expansionSize)+len(fb.Expansion)+int(fb.bodySize))

	var i int
	i += copy(data[i:], fBlockChainID[:])
	i += copy(data[i:], fb.BodyMR[:])
	i += copy(data[i:], fb.PrevKeyMR[:])
	i += copy(data[i:], fb.PrevLedgerKeyMR[:])

	binary.BigEndian.PutUint64(data[i:], fb.ECExchangeRate)
	i += 8
	binary.BigEndian.PutUint32(data[i:], fb.Height)
	i += 4
	i += copy(data[i:], expansionSize)
	// Currently all expansion bytes are stored in the Expansion.
	i += copy(data[i:], fb.Expansion)
	binary.BigEndian.PutUint32(data[i:], uint32(len(fb.Transactions)))
	i += 4
	binary.BigEndian.PutUint32(data[i:], fb.bodySize)
	i += 4

	// Body
	var period int
	for c, transaction := range fb.Transactions {
		for period < len(fb.endOfPeriod) && // If minute marked remain to be written
			fb.endOfPeriod[period] > 0 && // If the period markers are actually set (ignore otherwise)
			c == fb.endOfPeriod[period] { // This TX is the market point
			data[i] = FBlockMinuteMarker
			period++
			i++
		}

		tData, err := transaction.MarshalBinary()
		if err != nil {
			return nil, err
		}
		i += copy(data[i:], tData)
	}

	for period < len(fb.endOfPeriod) {
		data[i] = FBlockMinuteMarker
		i++
		period++
	}

	return data, nil
}

// ComputeFullHash computes the full hash of the FBlock.
func (fb FBlock) ComputeFullHash() (Bytes32, error) {
	data, err := fb.MarshalBinary()
	if err != nil {
		return Bytes32{}, err
	}
	return sha256.Sum256(data), nil
}
