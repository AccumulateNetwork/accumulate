// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"io"

	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

type ReaderSection = snapshot.ReaderSection
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
	snapshot.Reader
}

type Writer struct {
	snapshot.Writer
}

func NewReader(file ioutil2.SectionReader) *Reader {
	return &Reader{*snapshot.NewReader(file)}
}

func NewWriter(w io.WriteSeeker) *Writer {
	return &Writer{*snapshot.NewWriter(w)}
}
