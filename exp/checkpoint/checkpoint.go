// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package checkpoint

import (
	"bytes"
	_ "embed"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Checkpoint struct {
	snap       *snapshot.Reader
	observer   coredb.Observer
	db         *coredb.Batch
	bptSection snapshot.RecordReader
	bptOffsets map[[32]byte]int64
}

func LoadMainNet() (*Checkpoint, error) {
	return load(accumulate.MainNetCheckpoint())
}

func load(snapData ioutil.SectionReader, stateHash [32]byte) (*Checkpoint, error) {
	var err error
	c := new(Checkpoint)

	// Open the snapshot
	c.snap, err = snapshot.Open(snapData)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot: %w", err)
	}

	// Open it as a database
	store, err := c.snap.AsStore()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("init store: %w", err)
	}
	c.observer = execute.NewDatabaseObserver()
	c.db = coredb.NewBatch("snapshot", store, false, nil)

	// Open the BPT section
	c.bptSection, err = c.snap.OpenBPT(-1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open BPT section: %w", err)
	}

	// Construct an offset table
	c.bptOffsets = map[[32]byte]int64{}
	for {
		// Get the current offset
		offset, err := c.bptSection.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("read BPT section (1): %w", err)
		}

		// Read the next entry
		r, err := c.bptSection.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.UnknownError.WithFormat("read BPT section (2): %w", err)
		}

		// Record it
		c.bptOffsets[r.Key.Hash()] = offset

		// Verify it has a receipt
		if r.Receipt == nil {
			return nil, errors.InternalError.With("BPT entry: receipt is missing")
		}
		if !r.Receipt.Validate(nil) {
			return nil, errors.InternalError.With("BPT entry: receipt is invalid")
		}
		if !bytes.Equal(r.Receipt.Start, r.Value) {
			return nil, errors.InternalError.With("BPT entry: receipt start does not match recorded hash")
		}
		if !bytes.Equal(r.Receipt.Anchor, stateHash[:]) {
			return nil, errors.InternalError.With("BPT entry: receipt anchor does not match state hash")
		}
	}

	return c, nil
}

func (c *Checkpoint) Account(u *url.URL) *accountRecord {
	a := c.db.Account(u)
	return &accountRecord{c, a}
}

type accountRecord struct {
	*Checkpoint
	account *coredb.Account
}

func (v *accountRecord) Main() values.Value[protocol.Account] {
	return &accountMain{v, v.account.Main()}
}

type accountMain struct {
	*accountRecord
	values.Value[protocol.Account]
}

func (v *accountMain) check() error {
	// Load the BPT entry
	bptOff, ok := v.bptOffsets[v.account.Key().Hash()]
	if !ok {
		return (*database.NotFoundError)(v.account.Key())
	}
	bpt, err := v.bptSection.ReadAt(bptOff)
	if err != nil {
		return errors.UnknownError.WithFormat("read BPT entry: %w", err)
	}
	if bpt.Key.Hash() != v.account.Key().Hash() {
		return errors.InternalError.With("invalid BPT entry: key does not match")
	}

	// Calculate the account's state hash and verify it against the BPT entry
	hasher, err := v.observer.DidChangeAccount(v.db, v.account)
	if err != nil {
		return errors.InternalError.WithFormat("construct state hash: %w", err)
	}
	if !bytes.Equal(hasher.MerkleHash(), bpt.Value) {
		return errors.InternalError.With("BPT entry does not match account state")
	}

	// [Load] validates the BPT proof so we can skip that here
	return nil
}

func (v *accountMain) Get() (protocol.Account, error) {
	err := v.check()
	if err != nil {
		return nil, err
	}
	return v.Value.Get()
}

func (v *accountMain) GetAs(target any) error {
	err := v.check()
	if err != nil {
		return err
	}
	return v.Value.GetAs(target)
}
