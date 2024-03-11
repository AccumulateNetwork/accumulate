// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

type ReaderSection = ioutil.Segment[SectionType, *SectionType]
type SectionType = snapshot.SectionType

const (
	SectionTypeHeader         = snapshot.SectionTypeHeader
	SectionTypeAccounts       = snapshot.SectionTypeAccountsV1
	SectionTypeTransactions   = snapshot.SectionTypeTransactionsV1
	SectionTypeSignatures     = snapshot.SectionTypeSignaturesV1
	SectionTypeGzTransactions = snapshot.SectionTypeGzTransactionsV1
	SectionTypeSnapshot       = snapshot.SectionTypeSnapshot
)

type Reader struct {
	ioutil.SegmentedReader[SectionType, *SectionType]
}

type Writer struct {
	Logger logging.OptionalLogger
	ioutil.SegmentedWriter[SectionType, *SectionType]
}

func NewReader(file ioutil2.SectionReader) *Reader {
	return &Reader{*ioutil.NewSegmentedReader[SectionType](file)}
}

func NewWriter(w io.WriteSeeker) *Writer {
	return &Writer{SegmentedWriter: *ioutil.NewSegmentedWriter[SectionType](w)}
}
