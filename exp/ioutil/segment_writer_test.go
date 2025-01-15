// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil_test

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
)

func TestSegmentedWriterAppend(t *testing.T) {
	buf := new(ioutil.Buffer)
	w := ioutil.NewSegmentedWriter[segType](buf)

	addSection := func(typ segType, count int) {
		b := make([]byte, count)
		_, err := io.ReadFull(rand.Reader, b)
		require.NoError(t, err)

		w, err := w.Open(typ)
		require.NoError(t, err)
		_, err = w.Write(b)
		require.NoError(t, err)
		err = w.Close()
		require.NoError(t, err)
	}

	// Write two sections
	addSection(segTypeA, 10)
	addSection(segTypeB, 20)

	// Reopen for appending, write another section
	w, _, err := ioutil.AppendToSegmented[segType](buf)
	require.NoError(t, err)

	addSection(segTypeC, 30)

	// Verify
	r := ioutil.NewSegmentedReader[segType](buf)
	var segments []*ioutil.Segment[segType, *segType]
	for {
		s, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		segments = append(segments, s)
	}

	require.Len(t, segments, 3)
	require.Equal(t, segTypeA, segments[0].Type())
	require.Equal(t, segTypeB, segments[1].Type())
	require.Equal(t, segTypeC, segments[2].Type())
	require.Equal(t, int64(10), segments[0].Size())
	require.Equal(t, int64(20), segments[1].Size())
	require.Equal(t, int64(30), segments[2].Size())
}

type segType int

const (
	segTypeA segType = iota + 1
	segTypeB
	segTypeC
)

func (t segType) String() string              { return fmt.Sprint(int(t)) }
func (t segType) GetEnumValue() uint64        { return uint64(t) }
func (t *segType) SetEnumValue(v uint64) bool { *t = segType(v); return true }
