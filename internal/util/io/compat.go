// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil2

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
)

type Buffer = ioutil.Buffer
type SectionReader = ioutil.SectionReader
type SectionWriter = ioutil.SectionWriter

func NewBuffer(b []byte) *Buffer { return ioutil.NewBuffer(b) }

func NewSectionReader(rd SectionReader, start, end int64) (*io.SectionReader, error) {
	return ioutil.NewSectionReader(rd, start, end)
}

func NewSectionWriter(wr io.WriteSeeker, start, end int64) (*SectionWriter, error) {
	return ioutil.NewSectionWriter(wr, start, end)
}
