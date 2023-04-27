// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
)

type Reader = ioutil.SegmentedReader[SectionType, *SectionType]
type ReaderSection = ioutil.Segment[SectionType, *SectionType]
type Writer = ioutil.SegmentedWriter[SectionType, *SectionType]
type SectionWriter = ioutil.SegmentWriter[SectionType, *SectionType]

func NewReader(file ioutil2.SectionReader) *Reader {
	return ioutil.NewSegmentedReader[SectionType](file)
}

func NewWriter(w io.WriteSeeker) *Writer {
	return ioutil.NewSegmentedWriter[SectionType](w)
}
