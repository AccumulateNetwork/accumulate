// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type segmentType uint64

func (v segmentType) String() string              { return fmt.Sprint(uint64(v)) }
func (v segmentType) GetEnumValue() uint64        { return uint64(v) }
func (v *segmentType) SetEnumValue(u uint64) bool { *v = segmentType(u); return true }

func TestSegmentedFile(t *testing.T) {
	buf := new(Buffer)
	wr := NewSegmentedWriter[segmentType](buf)

	swr, err := wr.Open(segmentType(1))
	require.NoError(t, err)
	_, err = swr.Write([]byte("foo"))
	require.NoError(t, err)
	require.NoError(t, swr.Close())

	swr, err = wr.Open(segmentType(5))
	require.NoError(t, err)
	_, err = swr.Write([]byte("bar"))
	require.NoError(t, err)
	require.NoError(t, swr.Close())

	buf = NewBuffer(buf.Bytes())
	rd := NewSegmentedReader[segmentType](buf)

	s, err := rd.Next()
	require.NoError(t, err)
	require.Equal(t, segmentType(1), s.Type())
	srd, err := s.Open()
	require.NoError(t, err)
	b, err := io.ReadAll(srd)
	require.NoError(t, err)
	require.Equal(t, "foo", string(b))

	s, err = rd.Next()
	require.NoError(t, err)
	require.Equal(t, segmentType(5), s.Type())
	srd, err = s.Open()
	require.NoError(t, err)
	b, err = io.ReadAll(srd)
	require.NoError(t, err)
	require.Equal(t, "bar", string(b))
}
