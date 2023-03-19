// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"bytes"
	"io"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
)

type Buffer = ioutil2.Buffer
type SectionReader = ioutil2.SectionReader
type GenesisOptions = genesis.InitOpts

func Genesis(file io.WriteSeeker, opts GenesisOptions) ([]byte, error) {
	return genesis.Init(file, opts)
}

func NewSummaryGenesisBuilder(file io.WriteSeeker, time time.Time, partitionIds []string) (*SummaryGenesisBuilder, error) {
	header := new(snapshot.Header)
	header.Height = 1
	header.Timestamp = time
	header.PartitionSnapshotIDs = partitionIds
	w, err := snapshot.Create(file, header)
	if err != nil {
		return nil, err
	}
	return &SummaryGenesisBuilder{w}, nil
}

type SummaryGenesisBuilder struct {
	w *snapshot.Writer
}

func (b *SummaryGenesisBuilder) Add(data []byte) error {
	w, err := b.w.Open(snapshot.SectionTypeSnapshot)
	if err != nil {
		return err
	}
	_, err = bytes.NewBuffer(data).WriteTo(w)
	if err != nil {
		return err
	}
	return w.Close()
}
