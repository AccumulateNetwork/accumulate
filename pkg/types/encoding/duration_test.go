// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A duration must survive the wire format. It did not: the split rounded
// instead of truncating, so any duration with a fractional part of half a
// second or more encoded a negative remainder as unsigned and came back wrong
// -- 1.5s as 500ms, 900ms as -100ms (#4267).
func TestSplitDurationRoundTrips(t *testing.T) {
	cases := []time.Duration{
		0, time.Nanosecond, time.Millisecond, 100 * time.Millisecond,
		499 * time.Millisecond, 500 * time.Millisecond, 501 * time.Millisecond,
		900 * time.Millisecond, time.Second, 1200 * time.Millisecond,
		1500 * time.Millisecond, 2500 * time.Millisecond, 3 * time.Second,
		time.Minute + 750*time.Millisecond, 24 * time.Hour,
	}
	for _, d := range cases {
		sec, ns := SplitDuration(d)
		require.Less(t, ns, uint64(time.Second), "%v: the remainder must be under a second", d)
		require.Equal(t, d, time.Duration(sec)*time.Second+time.Duration(ns), "%v does not round trip", d)
	}

	r := rand.New(rand.NewSource(1))
	for i := 0; i < 10000; i++ {
		d := time.Duration(r.Int63n(int64(100 * time.Hour)))
		sec, ns := SplitDuration(d)
		require.Equal(t, d, time.Duration(sec)*time.Second+time.Duration(ns), "%v does not round trip", d)
	}

	// Not representable, and must not wrap
	for _, d := range []time.Duration{-1, -time.Second, -1500 * time.Millisecond} {
		sec, ns := SplitDuration(d)
		require.Zero(t, sec)
		require.Zero(t, ns)
	}
}

// The same, through the writer and reader rather than the split alone.
func TestDurationThroughTheCodec(t *testing.T) {
	for _, d := range []time.Duration{900 * time.Millisecond, 1500 * time.Millisecond, 2500 * time.Millisecond, 3 * time.Second} {
		buf := new(bytes.Buffer)
		w := NewWriter(buf)
		w.WriteDuration(1, d)
		_, _, err := w.Reset(nil)
		require.NoError(t, err)

		r := NewReader(bytes.NewReader(buf.Bytes()))
		got, ok := r.ReadDuration(1)
		require.True(t, ok)
		require.Equal(t, d, got)
	}
}
