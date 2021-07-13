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

package fat107

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/Factom-Asset-Tokens/factom"
	"golang.org/x/sync/errgroup"
)

// Metadata describes the Data from a Data Store Chain.
type Metadata struct {
	// The sha256d hash of the Data.
	DataHash *factom.Bytes32 `json:"-"`

	// The First Entry of the Data Store Chain, from which this Metadata
	// was parsed.
	Entry factom.Entry `json:"-"`

	// The Version of the Data Store protocol.
	Version string `json:"data-store"`

	// The total uncompressed size of the Data. If there are no compression
	// settings, this is what is actually stored on the Data Store Chain.
	Size uint64 `json:"size"`

	// Optional compression settings describing how the Data is stored.
	*Compression `json:"compression,omitempty"`

	// The Entry Hash of the first DBI Entry that describing the Data.
	DBIStart *factom.Bytes32 `json:"dbi-start"`

	// Optional additional JSON containing application defined Metadata.
	AppMetadata json.RawMessage `json:"metadata,omitempty"`
}

// Compression describes compression settings for how the Data is stored.
type Compression struct {
	// Compression format used on the data. May be "gzip" or "zlib".
	Format string `json:"format"`

	// The size of the compressed data. This is what is actually stored on
	// the Data Store Chain.
	Size uint64 `json:"size"`
}

// Current Protocol and Version.
const (
	Protocol = "data-store"
	Version  = "1.0"
)

// NameIDs constructs the valid Data Store NameIDs for data with the given
// dataHash and namespace.
//
// Pass the result to factom.ComputeChainID to derive the Data Store Chain ID.
func NameIDs(dataHash *factom.Bytes32, namespace ...factom.Bytes) []factom.Bytes {
	return append([]factom.Bytes{[]byte(Protocol), dataHash[:]}, namespace...)
}

// Lookup the Metadata for a given Data Store chainID.
func Lookup(ctx context.Context, c *factom.Client,
	chainID *factom.Bytes32) (Metadata, error) {

	// Get the first Entry in the Chain...

	// Get the first EBlock in the Chain.
	firstEB := factom.EBlock{ChainID: chainID}
	if err := firstEB.GetFirst(ctx, c); err != nil {
		return Metadata{}, err
	}

	// Get the First Entry in the EBlock.
	firstE := firstEB.Entries[0]
	if err := firstE.Get(ctx, c); err != nil {
		return Metadata{}, err
	}

	// Parse the First Entry and return the Metadata or any error.
	return ParseEntry(firstE)
}

// ParseEntry attempts to parse e as the First Entry from a Data Store Chain.
func ParseEntry(e factom.Entry) (Metadata, error) {

	// Validate and parse ExtIDs

	// The Entry must have at least 2 ExtIDs.
	if len(e.ExtIDs) < 2 {
		return Metadata{}, fmt.Errorf("invalid len(ExtIDs)")
	}

	// The first ExtID must declare the Protocol
	if string(e.ExtIDs[0]) != Protocol {
		return Metadata{}, fmt.Errorf("ExtIDs[0]: invalid protocol")
	}

	// The second ExtID must be a 32 bytes hash.
	if len(e.ExtIDs[1]) != 32 {
		return Metadata{}, fmt.Errorf("ExtIDs[1]: invalid data hash length")
	}
	var dataHash factom.Bytes32
	copy(dataHash[:], e.ExtIDs[1])

	// Parse the JSON.
	md := Metadata{DataHash: &dataHash, Entry: e}
	if err := json.Unmarshal(e.Content, &md); err != nil {
		return Metadata{}, err
	}

	// Validate the version.
	if md.Version != Version {
		return Metadata{}, fmt.Errorf(`Content: unsupported "version"`)
	}

	// Zero size data is prohibited.
	if md.Size == 0 {
		return Metadata{}, fmt.Errorf(`Content: invalid "size"`)
	}

	// We must have a DBI Start Hash.
	if md.DBIStart == nil {
		return Metadata{}, fmt.Errorf(`Content: missing "dbi-start"`)
	}

	// Validate optional compression settings.
	if md.Compression != nil {

		// Only support "zlib" and "gzip".
		switch strings.ToLower(md.Format) {
		case "zlib", "gzip":
		default:
			return Metadata{}, fmt.Errorf(
				`Content: unsupported "compression"."format"`)
		}

		// Zero size data is prohibited.
		if md.Compression.Size == 0 {
			return Metadata{}, fmt.Errorf(
				`Content: invalid "compression"."size"`)
		}
	}

	return md, nil
}

const (
	MaxDBIEHashCount       = factom.EntryMaxDataSize / 32
	MaxLinkedDBIEHashCount = (factom.EntryMaxDataSize - 32 - 2) / 32
)

// Download all Data Block Index and Data Block Entries required to reconstruct
// the on chain data, and then decompresses the data if necessary before
// writing it to the given data io.Writer.
//
// The Data Block Entries are downloaded concurrently as they are loaded from
// the DBI.
//
// The sha256d hash of the data written to data, is verified.
func (m Metadata) Download(ctx context.Context, c *factom.Client, data io.Writer) error {

	// Get the on-chain size.
	size := m.Size
	if m.Compression != nil {
		size = m.Compression.Size
	}

	// Compute the expected DB Count.
	totalDBCount := int(size / factom.EntryMaxDataSize)
	if size%factom.EntryMaxDataSize > 0 {
		totalDBCount++
	}

	// cData will contain the on chain data. We preallocate this so that we
	// can have the Data Block Entries populate it directly as they are
	// downloaded concurrently.
	cData := make([]byte, size)

	// Pass along the Data Block Entries from the DBI to this channel.
	dbEs := make(chan factom.Entry, totalDBCount)

	// Download and process the Data Block Entries concurrently as they are
	// parsed from the DBI, which is downloaded below.
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < runtime.NumCPU(); i++ {
		g.Go(func() error {
			for dbE := range dbEs {
				origCap := cap(dbE.Content)
				if err := dbE.Get(ctx, c); err != nil {
					return err
				}
				// Ensure that the Content did not exceed the
				// original capacity of the underlying cData
				// slice, and that the Content is filled to
				// capacity of either the underlying cData
				// slice, or the Entry limit.
				if cap(dbE.Content) != origCap ||
					(len(dbE.Content) < factom.EntryMaxDataSize &&
						len(dbE.Content) != cap(dbE.Content)) {
					return fmt.Errorf(
						"invalid Data Block Entry Content")
				}
			}
			return nil
		})
	}

	// Download the DBI linked list and populate the Data Block Entry Hashes.

	// dbiBuf will hold the Content of the current DBI Entry.
	dbiBuf := bytes.NewBuffer(nil)

	// dbiEHash holds the Entry Hash for the next DBI Entry in the Linked
	// List.
	dbiEHash := *m.DBIStart

	for i := 0; i < totalDBCount; i++ {
		// If we have no Data Block Hashes to parse, download and
		// validate the next DBI Entry.
		if dbiBuf.Len() == 0 {
			// Download the next DBI Entry.
			dbiE := factom.Entry{Hash: &dbiEHash}
			if err := dbiE.Get(ctx, c); err != nil {
				return err
			}

			// Ensure there are no incomplete hashes.
			if len(dbiE.Content)%32 > 0 {
				return fmt.Errorf("invalid DBI Entry Content")
			}

			// dbCount is the number of Data Block Hashes in this
			// DBI Entry.
			dbCount := len(dbiE.Content) / 32

			// remaining is the number of Data Block Hashes that
			// still need to be parsed or downloaded.
			remaining := totalDBCount - i

			// If there are more remaining than can fit in a single
			// DBI Entry...
			if remaining > MaxDBIEHashCount {

				// Require exact number of Hashes
				if dbCount != MaxLinkedDBIEHashCount {
					return fmt.Errorf("invalid DBI Entry Content")
				}

				// Require a DBI Entry Link.
				if len(dbiE.ExtIDs) != 1 ||
					len(dbiE.ExtIDs[0]) != 32 {
					return fmt.Errorf(
						"missing or invalid DBI Entry link")
				}

				// Parse the next DBI Entry Hash.
				copy(dbiEHash[:], dbiE.ExtIDs[0])
			} else if dbCount != remaining {
				// Otherwise this DBI Entry must include all
				// remaining DB Hashes.
				fmt.Println("dbi E", i, dbCount, remaining)
				return fmt.Errorf("invalid DBI Entry Content")
			}

			// Set up the new dbiBuf to parse the DB Hashes from.
			dbiBuf = bytes.NewBuffer(dbiE.Content)
		}

		// Parse out the next Data Block Entry Hash.
		dbE := factom.Entry{Hash: new(factom.Bytes32)}
		dbiBuf.Read(dbE.Hash[:])

		// Set the Content of each Data Block so the Content will get
		// unmarshalled directly into the proper location within cData
		// when the Data Blocks are downloaded concurrently.
		cDataI := i * factom.EntryMaxDataSize
		dbE.Content = cData[cDataI:cDataI]

		dbEs <- dbE
	}
	close(dbEs)

	// Wait until all Data Block Entries are processed.
	if err := g.Wait(); err != nil {
		return err
	}

	dataBuf := io.Reader(bytes.NewBuffer(cData))

	// Decompress the data, if necessary
	if m.Compression != nil {
		switch strings.ToLower(m.Format) {
		case "gzip":
			r, err := gzip.NewReader(dataBuf)
			if err != nil {
				return err
			}
			defer r.Close()
			dataBuf = r
		case "zlib":
			r, err := zlib.NewReader(dataBuf)
			if err != nil {
				return err
			}
			defer r.Close()
			dataBuf = r
		default:
			panic(`invalid "compression"."format"`)
		}
	}

	// Compute the data hash and write to data.
	hash := sha256.New()
	data = io.MultiWriter(hash, data)

	if _, err := io.Copy(data, dataBuf); err != nil {
		return err
	}

	// Verify data hash
	if *m.DataHash != sha256.Sum256(hash.Sum(nil)) {
		return fmt.Errorf("invalid data hash")
	}

	return nil
}
