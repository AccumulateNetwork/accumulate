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
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"crypto/ed25519"
)

// Entry represents a Factom Entry.
//
// Entry can be used to Get data when the Hash is known, or submit a new Entry
// to a given ChainID.
type Entry struct {
	// An Entry in EBlock.Entries after a successful call to EBlock.Get has
	// its ChainID, Hash, and Timestamp.
	ChainID   *Bytes32  `json:"chainid,omitempty"`
	Hash      *Bytes32  `json:"entryhash,omitempty"`
	Timestamp time.Time `json:"-"` // Established by EBlock

	// Entry.Get populates the Content and ExtIDs.
	ExtIDs  []Bytes `json:"extids"`
	Content Bytes   `json:"content"`

	// marshalBinaryCache is the binary data of the Entry. It is cached by
	// UnmarshalBinary so it can be re-used by MarshalBinary.
	marshalBinaryCache []byte
}

// ClearMarshalBinaryCache discards the cached MarshalBinary data.
//
// Subsequent calls to MarshalBinary will re-construct the data from the fields
// of the Entry.
func (e *Entry) ClearMarshalBinaryCache() {
	e.marshalBinaryCache = nil
}

// IsPopulated returns true if e has already been successfully populated by a
// call to Get.
func (e Entry) IsPopulated() bool {
	return e.ChainID != nil &&
		e.ExtIDs != nil &&
		e.Content != nil
}

// Get populates e with the Entry data for its e.Hash.
//
// If e.Hash is nil, an error will be returned.
//
// After a successful call e.Content, e.ExtIDs, and e.ChainID will be
// populated.
func (e *Entry) Get(ctx context.Context, c *Client) error {
	if e.IsPopulated() {
		return nil
	}

	if e.Hash == nil {
		return fmt.Errorf("Hash is nil")
	}

	params := struct {
		Hash *Bytes32 `json:"hash"`
	}{Hash: e.Hash}
	var result struct {
		Data Bytes `json:"data"`
	}

	if err := c.FactomdRequest(ctx, "raw-data", params, &result); err != nil {
		return err
	}
	return e.UnmarshalBinary(result.Data)
}

type chainFirstEntryParams struct {
	Entry *Entry `json:"firstentry"`
}
type composeChainParams struct {
	Chain chainFirstEntryParams `json:"chain"`
	EC    ECAddress             `json:"ecpub"`
}
type composeEntryParams struct {
	Entry *Entry    `json:"entry"`
	EC    ECAddress `json:"ecpub"`
}

type composeJRPC struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}
type composeResult struct {
	Commit composeJRPC `json:"commit"`
	Reveal composeJRPC `json:"reveal"`
}
type commitResult struct {
	TxID *Bytes32
}

// Create queries factom-walletd to compose e as a new Entry, and then queries
// factomd to commit and reveal the new Entry or new Chain, if e.ChainID ==
// nil.
//
// The given ec must exist in factom-walletd's keystore.
//
// If successful, the commit transaction ID is returned and e.Hash and
// e.ChainID will be populated.
func (e *Entry) Create(ctx context.Context, c *Client, ec ECAddress) (Bytes32, error) {
	var params interface{}
	var method string

	if e.ChainID == nil {
		method = "compose-chain"
		params = composeChainParams{
			Chain: chainFirstEntryParams{Entry: e},
			EC:    ec,
		}
	} else {
		method = "compose-entry"
		params = composeEntryParams{Entry: e, EC: ec}
	}
	result := composeResult{}

	if err := c.WalletdRequest(ctx, method, params, &result); err != nil {
		return Bytes32{}, err
	}
	if len(result.Commit.Method) == 0 {
		return Bytes32{}, fmt.Errorf("Wallet request error: method: %#v", method)
	}

	var commit commitResult
	if err := c.FactomdRequest(ctx,
		result.Commit.Method, result.Commit.Params, &commit); err != nil {
		return Bytes32{}, err
	}

	if err := c.FactomdRequest(ctx,
		result.Reveal.Method, result.Reveal.Params, e); err != nil {
		return Bytes32{}, err
	}
	return *commit.TxID, nil
}

// ComposeCreate composes and submits an entry to factomd by calling e.Compose
// and then c.Commit and c.Reveal.
//
// This does not make any calls to factom-walletd.
//
// The e.Hash will be populated if not nil.
//
// If e.ChainID == nil, a new chain will be created, and e.ChainID will be
// populated.
//
// If successful, the Transaction ID is returned.
func (e *Entry) ComposeCreate(
	ctx context.Context, c *Client, es EsAddress) (Bytes32, error) {

	commit, reveal, txID, err := e.Compose(es)
	if err != nil {
		return Bytes32{}, fmt.Errorf("factom.Entry.Compose(): %w", err)
	}

	if err := c.Commit(ctx, commit); err != nil {
		return txID, fmt.Errorf("factom.Client.Commit(): %w", err)
	}
	if err := c.Reveal(ctx, reveal); err != nil {
		return txID, fmt.Errorf("factom.Client.Reveal(): %w", err)
	}

	return txID, nil
}

// Commit sends an entry or new chain commit to factomd.
func (c *Client) Commit(ctx context.Context, commit []byte) error {
	var method string
	switch len(commit) {
	case EntryCommitSize:
		method = "commit-entry"
	case ChainCommitSize:
		method = "commit-chain"
	default:
		return fmt.Errorf("invalid commit length")
	}

	params := struct {
		Commit Bytes `json:"message"`
	}{Commit: commit}

	if err := c.FactomdRequest(ctx, method, params, nil); err != nil {
		return err
	}
	return nil
}

// Reveal reveals an entry or new chain entry to factomd.
func (c *Client) Reveal(ctx context.Context, reveal []byte) error {
	params := struct {
		Reveal Bytes `json:"entry"`
	}{Reveal: reveal}
	if err := c.FactomdRequest(ctx, "reveal-entry", params, nil); err != nil {
		return err
	}
	return nil
}

// Compose generates the commit and reveal data required to submit an Entry to
// factomd with Client.Commit and Client.Reveal. The Transaction ID is also
// returned.
//
// The e.Hash will be populated if not nil.
//
// If e.ChainID == nil, a new chain will be created, and e.ChainID will be
// populated.
//
// If the reveal is already available to the caller, use GenerateCommit to
// create the commit without recreating the reveal, which is simply the raw
// data of an Entry.
func (e *Entry) Compose(es EsAddress) (
	commit []byte, reveal []byte, txID Bytes32, err error) {

	newChain := e.ChainID == nil

	if newChain {
		e.ChainID = new(Bytes32)
		*e.ChainID = ComputeChainID(e.ExtIDs)
	}

	reveal, err = e.MarshalBinary()
	if err != nil {
		err = fmt.Errorf("factom.Entry.MarshalBinary(): %w", err)
		return
	}

	if e.Hash == nil {
		e.Hash = new(Bytes32)
		*e.Hash = ComputeEntryHash(reveal)
	}

	commit, txID = GenerateCommit(es, reveal, e.Hash, newChain)
	return
}

const (
	EntryCommitSize = 1 + // version
		6 + // timestamp
		32 + // entry hash
		1 + // ec cost
		32 + // ec pub
		64 // sig
	ChainCommitSize = EntryCommitSize +
		32 + // chain id hash
		32 // commit weld
)

// GenerateCommit generates a commit message signed by es for the given
// entrydata and hash.
//
// The entrydata must be the valid raw data encoding of an Entry, which can be
// obtained using Entry.MarshalBinary.
//
// The hash must be the valid Entry Hash, which is anything that
// Entry.UnmarshalBinary can parse without error and can be obtained using
// ComputeEntryHash.
//
// If newChain is true, then the commit will be a new Chain commit. The ChainID
// will be pulled from the entrydata.
//
// If successful, the commit and Entry Transaction ID will be returned.
//      txID == sha256(commit[:len(commit)-96])
//
// If either entrydata or hash is not valid, the return values will be invalid
// and panics may occur. It is up to the caller to ensure that the entrydata
// and hash are valid.
//
// This allows the caller to manage the memory associated with the entrydata
// and hash, rather than having to regenerate it repeatedly using
// Entry.MarshalBinary. For a higher level API see the functions Entry.Compose,
// Entry.ComposeCreate, and Entry.Create.
//
// The commit message data format is as follows:
//      [Version (0x00)] +
//	[Timestamp in ms (6 bytes BE)] +
//      (if newChain)
//	        [ChainID Hash, sha256d(ChainID) (Bytes32)] +
//	        [Commit Weld, sha256d(hash|chainID) (Bytes32)] +
//	[Entry Hash (Bytes32)] +
//	[EC Cost (1 byte)] +
//	[EC Public Key (32 Bytes)] +
//	[Signature of data up to and including EC Cost (64 Bytes)]
func GenerateCommit(es EsAddress, entrydata []byte, hash *Bytes32,
	newChain bool) ([]byte, Bytes32) {

	commitSize := EntryCommitSize
	if newChain {
		commitSize = ChainCommitSize
	}

	commit := make([]byte, commitSize)

	i := 1 // Skip version byte

	// ms is a timestamp salt in milliseconds.
	ms := time.Now().Unix()*1e3 + rand.Int63n(1000)
	putInt48BE(commit[i:], ms)
	i += 6

	if newChain {
		chainID := entrydata[1 : 1+len(Bytes32{})]
		// ChainID Hash
		chainIDHash := sha256d(chainID)
		i += copy(commit[i:], chainIDHash[:])

		// Commit Weld sha256d(entryhash | chainid)
		weld := sha256d(append(hash[:], chainID[:]...))
		i += copy(commit[i:], weld[:])
	}

	// Entry Hash
	i += copy(commit[i:], hash[:])

	cost, _ := EntryCost(len(entrydata), newChain)
	commit[i] = byte(cost)
	i++

	txID := sha256.Sum256(commit[:i])

	// Public Key
	signedDataSize := i
	i += copy(commit[i:], es.PublicKey())

	// Signature
	sig := ed25519.Sign(es.PrivateKey(), commit[:signedDataSize])
	copy(commit[i:], sig)

	return commit, txID
}

// NewChainCost is the fixed added cost of creating a new chain.
const NewChainCost = 10

// EntryCost returns the required Entry Credit cost for an entry with encoded
// length equal to size. An error is returned if size exceeds 10275.
//
// Set newChain to true to add the NewChainCost.
func EntryCost(size int, newChain bool) (uint8, error) {
	if size < EntryHeaderSize {
		return 0, fmt.Errorf("invalid size")
	}
	size -= EntryHeaderSize
	if size > 10240 {
		return 0, fmt.Errorf("Entry cannot be larger than 10KB")
	}
	cost := uint8(size / 1024)
	if size%1024 > 0 {
		cost++
	}
	if cost < 1 {
		cost = 1
	}
	if newChain {
		cost += NewChainCost
	}
	return cost, nil
}

// Cost returns the EntryCost of e, using e.MarshalBinaryLen().
//
// If e.ChainID == nil, the NewChainCost is added.
func (e Entry) Cost() (uint8, error) {
	return EntryCost(e.MarshalBinaryLen(), e.ChainID == nil)
}

// MarshalBinaryLen returns the total encoded length of e.
func (e Entry) MarshalBinaryLen() int {
	extIDTotalSize := len(e.ExtIDs) * 2 // Two byte len(ExtID) per ExtID
	for _, extID := range e.ExtIDs {
		extIDTotalSize += len(extID)
	}
	return EntryHeaderSize + extIDTotalSize + len(e.Content)
}

// MarshalBinary returns the raw Entry data for e. This will return an error if
// !e.IsPopulated(). The data format is as follows.
//
//      [Version byte (0x00)] +
//      [ChainID (Bytes32)] +
//      [Total ExtID encoded length (uint16 BE)] +
//      [ExtID 0 length (uint16)] + [ExtID 0 (Bytes)] +
//      ... +
//      [ExtID X length (uint16)] + [ExtID X (Bytes)] +
//      [Content (Bytes)]
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e Entry) MarshalBinary() ([]byte, error) {
	if len(e.marshalBinaryCache) > 0 {
		return e.marshalBinaryCache, nil
	}

	if e.ChainID == nil {
		return nil, fmt.Errorf("missing ChainID")
	}

	totalSize := e.MarshalBinaryLen()
	if totalSize > EntryMaxTotalSize {
		return nil, fmt.Errorf("length exceeds %v", EntryMaxTotalSize)
	}

	// Header, version byte 0x00
	data := make([]byte, totalSize)
	i := 1
	i += copy(data[i:], e.ChainID[:])
	binary.BigEndian.PutUint16(data[i:i+2],
		uint16(totalSize-len(e.Content)-EntryHeaderSize))
	i += 2

	// Payload
	for _, extID := range e.ExtIDs {
		n := len(extID)
		binary.BigEndian.PutUint16(data[i:i+2], uint16(n))
		i += 2
		i += copy(data[i:], extID)
	}
	copy(data[i:], e.Content)

	return data, nil
}

// EntryHeaderSize is the exact length of an Entry header.
const EntryHeaderSize = 1 + // version
	32 + // chain id
	2 // total len

// EntryMaxDataSize is the maximum data length of an Entry.
const EntryMaxDataSize = 10240

// EntryMaxTotalSize is the maximum total encoded length of an Entry.
const EntryMaxTotalSize = EntryMaxDataSize + EntryHeaderSize

// UnmarshalBinary unmarshals raw entry data into e.
//
// If e.ChainID is not nil, it must equal the ChainID described in the data.
//
// If e.Hash is not nil, it must equal ComputeEntryHash(data).
//
// Like json.Unmarshal, if e.ExtIDs or e.Content are preallocated, they are
// reset to length zero and then appended to.
//
// The data must encode a valid Entry. Entries are encoded as follows:
//
//      [Version byte (0x00)] +
//      [ChainID (Bytes32)] +
//      [Total ExtID encoded length (uint16 BE)] +
//      [ExtID 0 length (uint16)] + [ExtID 0 (Bytes)] +
//      ... +
//      [ExtID X length (uint16)] + [ExtID X (Bytes)] +
//      [Content (Bytes)]
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e *Entry) UnmarshalBinary(data []byte) error {

	if len(data) < EntryHeaderSize || len(data) > EntryMaxTotalSize {
		return fmt.Errorf("invalid length")
	}

	if data[0] != 0x00 {
		return fmt.Errorf("invalid version byte")
	}

	i := 1 // Skip version byte.

	var chainID Bytes32
	i += copy(chainID[:], data[i:i+len(e.ChainID)])
	if e.ChainID != nil {
		if *e.ChainID != chainID {
			return fmt.Errorf("invalid ChainID")
		}
	} else {
		e.ChainID = &chainID
	}

	extIDTotalSize := int(binary.BigEndian.Uint16(data[i : i+2]))
	if extIDTotalSize == 1 || EntryHeaderSize+extIDTotalSize > len(data) {
		return fmt.Errorf("invalid ExtIDs length")
	}
	i += 2

	e.ExtIDs = e.ExtIDs[0:0]

	for i < EntryHeaderSize+extIDTotalSize {
		extIDSize := int(binary.BigEndian.Uint16(data[i : i+2]))
		if i+2+extIDSize > EntryHeaderSize+extIDTotalSize {
			return fmt.Errorf("error parsing ExtIDs")
		}
		i += 2

		e.ExtIDs = append(e.ExtIDs, Bytes(data[i:i+extIDSize]))
		i += extIDSize
	}

	if e.Content == nil {
		e.Content = data[i:]
	} else {
		e.Content = append(e.Content[0:0], data[i:]...)
	}

	// Verify Hash, if set, otherwise populate it.
	hash := ComputeEntryHash(data)
	if e.Hash != nil {
		if *e.Hash != hash {
			return fmt.Errorf("invalid hash")
		}
	} else {
		e.Hash = &hash
	}

	// Cache data for efficient marshaling.
	e.marshalBinaryCache = data

	return nil
}
