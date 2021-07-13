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
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"
)

// EBlock represents a Factom Entry Block.
type EBlock struct {
	// DBlock.Get populates the ChainID, KeyMR, Height and Timestamp.
	ChainID   *Bytes32
	KeyMR     *Bytes32  // Computed
	Timestamp time.Time // Established by DBlock
	Height    uint32

	FullHash *Bytes32 // Computed

	// Unmarshaled
	PrevKeyMR    *Bytes32
	PrevFullHash *Bytes32
	BodyMR       *Bytes32
	Sequence     uint32
	ObjectCount  uint32

	// EBlock.Get populates the Entries with their Hash and Timestamp.
	Entries []Entry

	// marshalBinaryCache is the binary data of the EBlock. It is cached by
	// UnmarshalBinary so it can be re-used by MarshalBinary.
	marshalBinaryCache []byte
}

// ClearMarshalBinaryCache discards the cached MarshalBinary data.
//
// Subsequent calls to MarshalBinary will re-construct the data from the fields
// of the EBlock.
func (eb *EBlock) ClearMarshalBinaryCache() {
	eb.marshalBinaryCache = nil
}

// IsPopulated returns true if eb has already been populated by a successful
// call to Get.
func (eb EBlock) IsPopulated() bool {
	return len(eb.Entries) > 0 &&
		eb.ChainID != nil &&
		eb.PrevKeyMR != nil &&
		eb.PrevFullHash != nil &&
		eb.BodyMR != nil &&
		eb.FullHash != nil &&
		eb.ObjectCount > 1
}

// Get populates eb with the Entry Block corresponding to eb.KeyMR, if not nil,
// and otherwise the chain head for eb.ChainID.
//
// If both eb.KeyMR and eb.ChainID are nil, an error will be returned.
//
// After a successful call, all of eb's data will be populated, except the
// eb.Timestamp, which is established by the DBlock with the same Height. All
// eb.Entries will be populated with their Hash, ChainID, Timestamp, and
// Height.
func (eb *EBlock) Get(ctx context.Context, c *Client) error {
	if eb.IsPopulated() {
		return nil
	}

	// If we don't have a KeyMR, fetch the chain head KeyMR.
	if eb.KeyMR == nil {
		_, err := eb.GetChainHead(ctx, c)
		if err != nil {
			return err
		}
		if eb.KeyMR == nil {
			return fmt.Errorf("missing chain head")
		}
	}

	params := struct {
		KeyMR *Bytes32 `json:"hash"`
	}{KeyMR: eb.KeyMR}
	var result struct {
		Data Bytes `json:"data"`
	}

	if err := c.FactomdRequest(ctx, "raw-data", params, &result); err != nil {
		return err
	}
	return eb.UnmarshalBinary(result.Data)
}

// GetChainHead populates eb.KeyMR with the chain head for eb.ChainID, the
// latest EBlock KeyMR, if it exists.
//
// The returned boolean indicates whether the chain is in the process list.
//
// If the Chain does not exist, a jsonrpc2/v14.Error will be returned
// indicating a Missing Chain Head.
//
// If the Chain creation is pending, true will be returned, but the eb.KeyMR
// will not be populated.
func (eb *EBlock) GetChainHead(ctx context.Context, c *Client) (bool, error) {

	if eb.ChainID == nil {
		return false, fmt.Errorf("no ChainID specified")
	}

	params := struct {
		ChainID *Bytes32 `json:"chainid"`
	}{ChainID: eb.ChainID}
	var result struct {
		KeyMR              string `json:"chainhead"`
		ChainInProcessList bool   `json:"chaininprocesslist"`
	}

	var keyMR Bytes32
	if err := c.FactomdRequest(ctx, "chain-head", params, &result); err != nil {
		return result.ChainInProcessList, err
	}

	if len(result.KeyMR) > 0 {
		if err := keyMR.UnmarshalText([]byte(result.KeyMR)); err != nil {
			return result.ChainInProcessList, err
		}
		eb.KeyMR = &keyMR
	}

	return result.ChainInProcessList, nil
}

// GetEntries calls eb.Get and then calls Get on each Entry in eb.Entries.
//
// Entries are downloaded concurrently.
func (eb *EBlock) GetEntries(ctx context.Context, c *Client) error {
	if err := eb.Get(ctx, c); err != nil {
		return err
	}

	n := runtime.NumCPU()
	if len(eb.Entries) < n {
		n = len(eb.Entries)
	}

	entries := make(chan *Entry, n)

	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < n; i++ {
		g.Go(func() error {
			for e := range entries {
				if err := e.Get(ctx, c); err != nil {
					return err
				}
			}
			return nil
		})
	}
	for i := range eb.Entries {
		select {
		case entries <- &eb.Entries[i]:
		case <-ctx.Done():
		}
	}
	close(entries)

	return g.Wait()
}

// IsFirst returns true if this is the first EBlock in its chain, indicated by
// eb.PrevKeyMR.IsZero().
//
// If eb is not populated, eb.IsFirst will always return false.
func (eb EBlock) IsFirst() bool {
	return eb.IsPopulated() && eb.PrevKeyMR.IsZero()
}

// Prev returns the EBlock preceding eb, an EBlock with its KeyMR initialized
// to eb.PrevKeyMR and ChainID initialized to eb.ChainID.
//
// If eb is the first Entry Block in the chain, then eb is returned. If eb is
// not populated, the returned EBlock will be its zero value.
func (eb EBlock) Prev() EBlock {
	if !eb.IsPopulated() {
		return EBlock{}
	}
	if eb.IsFirst() {
		return eb
	}
	return EBlock{ChainID: eb.ChainID, KeyMR: eb.PrevKeyMR}
}

// GetPrevAll returns a slice of all preceding EBlocks, in order from eb to the
// first EBlock in the chain. So the 0th element of the returned slice is
// always equal to eb. If eb is the first EBlock in the chain, then it is the
// only element in the slice. Like Get, if eb does not have a KeyMR, the chain
// head KeyMR is queried first.
//
// If you are only interested in obtaining the first entry block in eb's chain,
// and not all of the intermediary ones, then use GetFirst.
func (eb EBlock) GetPrevAll(ctx context.Context, c *Client) ([]EBlock, error) {
	if err := eb.Get(ctx, c); err != nil {
		return nil, err
	}
	return eb.GetPrevN(ctx, c, eb.Sequence+1)
}

// GetPrevBackTo returns a slice of all preceding EBlocks, in order from eb
// back to, but not including, keyMR. So the 0th element of the returned slice
// is always equal to eb.
//
// Get is first called on eb. So if eb does not have a KeyMR, the chain head
// KeyMR is queried first.
//
// If *eb.KeyMR == *keyMR then ([]EBlock{}, nil) is returned.
//
// If *eb.PrevKeyMR == *keyMR, then it is the only element in the slice.
//
// If the beginning of the chain is reached without finding keyMR, then
// fmt.Errorf("end of chain") is returned.
func (eb EBlock) GetPrevBackTo(
	ctx context.Context, c *Client, keyMR *Bytes32) ([]EBlock, error) {

	if err := eb.Get(ctx, c); err != nil {
		return nil, err
	}
	if *eb.KeyMR == *keyMR {
		return []EBlock{}, nil
	}
	eblocks := []EBlock{eb}
	for {
		if *eb.PrevKeyMR == *keyMR {
			return eblocks, nil
		}
		if eb.IsFirst() {
			return nil, fmt.Errorf("end of chain")
		}
		eb = eb.Prev()
		if err := eb.Get(ctx, c); err != nil {
			return nil, err
		}
		eblocks = append(eblocks, eb)
	}
}

// GetPrevN returns a slice of n prev EBlocks, in reverse order, including eb.
// So the 0th element of the returned slice is always equal to eb.
//
// Get is first called on eb. So if eb does not have a KeyMR, the chain head
// KeyMR is queried first.
//
// If n == 0, then ([]EBlock{}, nil) is returned.
//
// If n == 1, then it is the only element in the slice.
//
// If the beginning of the chain is reached before n EBlocks then
// fmt.Errorf("end of chain") is returned.
func (eb EBlock) GetPrevN(ctx context.Context, c *Client,
	n uint32) ([]EBlock, error) {
	if n == 0 {
		return nil, nil
	}

	if err := eb.Get(ctx, c); err != nil {
		return nil, err
	}

	if n > eb.Sequence+1 {
		return nil, fmt.Errorf("end of chain")
	}

	eblocks := make([]EBlock, n)

	eblocks[0] = eb
	for i := range eblocks {
		if i == 0 {
			continue
		}
		eb := &eblocks[i]
		*eb = eblocks[i-1].Prev()
		if err := eb.Get(ctx, c); err != nil {
			return nil, err
		}
	}
	return eblocks, nil
}

// GetFirst finds the first Entry Block in eb's chain, and populates eb as
// such.
//
// GetFirst avoids allocating any new EBlocks by reusing eb to traverse up to
// the first entry block.
func (eb *EBlock) GetFirst(ctx context.Context, c *Client) error {
	for ; !eb.IsFirst(); *eb = eb.Prev() {
		if err := eb.Get(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

// EBlockHeaderSize is the exact length of an EBlock header.
const EBlockHeaderSize = 32 + // [ChainID (Bytes32)] +
	32 + // [BodyMR (Bytes32)] +
	32 + // [PrevKeyMR (Bytes32)] +
	32 + // [PrevFullHash (Bytes32)] +
	4 + // [EB Sequence (uint32 BE)] +
	4 + // [DB Height (uint32 BE)] +
	4 // [Entry Count (uint32 BE)]

// EBlockObjectSize is the length of an EBlock body object, which may be an
// Entry hash or minute marker.
const EBlockObjectSize = 32

// EBlockMinBodySize is the minimum length of the body of an EBlock, which must
// include at least on Entry hash and one minute marker.
const EBlockMinBodySize = EBlockObjectSize * 2

// EBlockMaxBodySize is the maximum length of the body of an EBlock, which is
// determined by the largest ObjectCount that can be stored in 4 bytes.
const EBlockMaxBodySize = math.MaxUint32 * EBlockObjectSize

// EBlockMinTotalSize is the minimum total length of an EBlock.
const EBlockMinTotalSize = EBlockHeaderSize + EBlockMinBodySize

// EBlockMaxTotalSize is the maximum total length of an EBlock.
const EBlockMaxTotalSize = EBlockHeaderSize + EBlockMaxBodySize

// min10Marker is frequently reused for comparison in UnmarshalBinary.
var min10Marker = Bytes32{31: 10}

// UnmarshalBinary unmarshals raw EBlock data, verifies the BodyMR, and
// populates eb.FullHash and eb.KeyMR, if nil. If eb.KeyMR is populated, it is
// verified. The following format is expected for data.
//
// Header
//      [ChainID (Bytes32)] +
//      [BodyMR (Bytes32)] +
//      [PrevKeyMR (Bytes32)] +
//      [PrevFullHash (Bytes32)] +
//      [EB Sequence (uint32 BE)] +
//      [DB Height (uint32 BE)] +
//      [Object Count (uint32 BE)]
//
// Body
//      [Object 0 (Bytes32)] // entry hash or minute marker +
//      ... +
//      [Object N (Bytes32)]
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry-block
func (eb *EBlock) UnmarshalBinary(data []byte) error {
	if uint64(len(data)) < EBlockMinTotalSize ||
		uint64(len(data)) > EBlockMaxTotalSize {
		return fmt.Errorf("invalid length")
	}

	var chainID Bytes32
	i := copy(chainID[:], data)
	if eb.ChainID != nil {
		if *eb.ChainID != chainID {
			return fmt.Errorf("invalid ChainID")
		}
	} else {
		eb.ChainID = &chainID
	}

	eb.BodyMR = new(Bytes32)
	i += copy(eb.BodyMR[:], data[i:])

	eb.PrevKeyMR = new(Bytes32)
	i += copy(eb.PrevKeyMR[:], data[i:])

	eb.PrevFullHash = new(Bytes32)
	i += copy(eb.PrevFullHash[:], data[i:])

	eb.Sequence = binary.BigEndian.Uint32(data[i : i+4])
	i += 4

	eb.Height = binary.BigEndian.Uint32(data[i : i+4])
	i += 4

	eb.ObjectCount = binary.BigEndian.Uint32(data[i : i+4])
	i += 4

	if len(data[i:]) != int(eb.ObjectCount*32) {
		return fmt.Errorf("invalid length")
	}

	// Parse objects and mark indexes of minute markers.
	objects := make([][]byte, eb.ObjectCount)
	minuteMarkerID := make([]int, 10)
	var numMins int
	for oi := range objects {
		objects[oi] = data[i : i+len(Bytes32{})]
		i += len(Bytes32{})

		if bytes.Compare(objects[oi], min10Marker[:]) <= 0 {
			minute := int(objects[oi][len(Bytes32{})-1])
			minuteMarkerID[minute-1] = oi
			numMins++
		}
	}

	// The last element must be a minute marker.
	if bytes.Compare(objects[len(objects)-1], min10Marker[:]) > 0 {
		return fmt.Errorf("invalid minute marker %v ",
			objects[len(objects)-1])
	}
	if numMins == 0 {
		return fmt.Errorf("no minute marker")
	}

	// Populate Entries from objects.
	eb.Entries = make([]Entry, int(eb.ObjectCount)-numMins)

	// ei indexes into eb.Entries. oi indexes into objects.
	var ei, oi int

	for min, markerID := range minuteMarkerID {
		if markerID == 0 {
			continue
		}

		// Set ts to the eb.Timestamp + the minute offset.
		ts := eb.Timestamp.Add(time.Duration(min+1) * MinuteDuration)

		// Populate EBlocks up to this minute marker.
		for ; oi < markerID; oi++ {
			e := &eb.Entries[ei]
			ei++

			e.Hash = new(Bytes32)
			copy(e.Hash[:], objects[oi])

			e.ChainID = eb.ChainID
			e.Timestamp = ts
		}

		// Skip over the minute marker.
		oi++
	}

	// Verify BodyMR.
	bodyMR, err := ComputeEBlockBodyMR(objects)
	if err != nil {
		return err
	}
	if *eb.BodyMR != bodyMR {
		return fmt.Errorf("invalid BodyMR")
	}

	// Compute KeyMR
	headerHash := ComputeEBlockHeaderHash(data)
	keyMR := ComputeKeyMR(&headerHash, &bodyMR)

	// Verify KeyMR, if set, otherwise populate it.
	if eb.KeyMR != nil {
		if *eb.KeyMR != keyMR {
			return fmt.Errorf("invalid KeyMR")
		}
	} else {
		// Populate KeyMR.
		eb.KeyMR = &keyMR
	}

	// Populate FullHash.
	eb.FullHash = new(Bytes32)
	*eb.FullHash = ComputeFullHash(data)

	eb.marshalBinaryCache = data

	return nil
}

// MarshalBinary returns the raw EBlock data for eb. This will return an error
// if !eb.IsPopulated(). The data format is as follows.
//
// Header
//      [ChainID (Bytes32)] +
//      [BodyMR (Bytes32)] +
//      [PrevKeyMR (Bytes32)] +
//      [PrevFullHash (Bytes32)] +
//      [EB Sequence (uint32 BE)] +
//      [DB Height (uint32 BE)] +
//      [Object Count (uint32 BE)]
//
// Body
//      [Object 0 (Bytes32)] // entry hash or minute marker +
//      ... +
//      [Object N (Bytes32)]
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry-block
//
// If EBlock was populated by a call to MarshalBinary, and ClearMar
func (eb EBlock) MarshalBinary() ([]byte, error) {
	if eb.marshalBinaryCache != nil {
		return eb.marshalBinaryCache, nil
	}

	if !eb.IsPopulated() {
		return nil, fmt.Errorf("not populated")
	}

	data := make([]byte, eb.MarshalBinaryLen())
	i := copy(data, eb.ChainID[:])
	i += copy(data[i:], eb.BodyMR[:])
	i += copy(data[i:], eb.PrevKeyMR[:])
	i += copy(data[i:], eb.PrevFullHash[:])

	binary.BigEndian.PutUint32(data[i:], eb.Sequence)
	i += 4

	binary.BigEndian.PutUint32(data[i:], eb.Height)
	i += 4

	binary.BigEndian.PutUint32(data[i:], eb.ObjectCount)
	i += 4

	var lastMin = int(eb.Entries[0].Timestamp.Sub(eb.Timestamp).Minutes())
	var min int
	for _, e := range eb.Entries {
		min = int(e.Timestamp.Sub(eb.Timestamp).Minutes())
		if min < lastMin || min > 10 {
			return nil, fmt.Errorf("invalid entry timestamp")
		}
		if min > lastMin {
			data[i+len(Bytes32{})-1] = byte(lastMin)
			i += len(Bytes32{})
			lastMin = min
		}
		i += copy(data[i:], e.Hash[:])
	}

	// Insert final minute marker
	data[i+len(Bytes32{})-1] = byte(min)

	return data, nil
}

// MarshalBinaryLen returns the length of the binary encoding of eb,
//      EBlockHeaderSize + len(eb.ObjectCount)*len(Bytes32{})
func (eb EBlock) MarshalBinaryLen() int {
	return EBlockHeaderSize + int(eb.ObjectCount)*len(Bytes32{})
}

// SetTimestamp sets the EBlock timestamp and updates all Entry Timestamps
// relative to this new eb.Timestamp so that the Minute Markers will still be
// valid during BinaryMarshaling.
func (eb *EBlock) SetTimestamp(ts time.Time) {
	prevTs := eb.Timestamp
	for i := range eb.Entries {
		e := &eb.Entries[i]
		minute := e.Timestamp.Sub(prevTs)
		e.Timestamp = ts.Add(minute)
	}
	eb.Timestamp = ts
}
