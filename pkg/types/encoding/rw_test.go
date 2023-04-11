// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding_test

import (
	"crypto/sha256"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func pipe() (*ioutil2.Buffer, *Reader, *Writer) {
	buf := new(ioutil2.Buffer)
	return buf, NewReader(buf), NewWriter(buf)
}

func wrOk(t *testing.T, buf *ioutil2.Buffer, w *Writer) {
	_, _, err := w.Reset(nil)
	require.NoError(t, err)
	_, err = buf.Seek(0, io.SeekStart)
	require.NoError(t, err)
}

func rdOk(t *testing.T, r *Reader) {
	_, err := r.Reset(nil)
	require.NoError(t, err)
}

func TestEmptyObject(t *testing.T) {
	buf, r, w := pipe()

	// Write nothing
	wrOk(t, buf, w)

	// Write extra data
	extra := []byte{4, 5, 6, 7}
	_, _ = buf.Write(extra)

	// Attempt to read something
	r.ReadUint(1)
	r.ReadUint(2)
	rdOk(t, r)

	// Verify the the extra data is ignored
	b, err := r.ReadAll()
	require.NoError(t, err)
	require.Empty(t, b)
	require.Equal(t, extra, buf.Bytes())
}

func TestZeroExtension(t *testing.T) {
	buf, r, w := pipe()

	// Write something
	w.WriteUint(1, 77)
	_, _, err := w.Reset(nil)
	require.NoError(t, err)

	// Write zeros
	zeros := make([]byte, 4)
	_, _ = buf.Write(zeros)

	// Attempt to read something
	r.ReadUint(1)
	r.ReadUint(2)
	r.ReadUint(3)
	rdOk(t, r)

	// Verify the zeros will be captured as extra data
	b, err := r.ReadAll()
	require.NoError(t, err)
	require.Equal(t, zeros, b)
}

func TestTypes(t *testing.T) {
	t.Run("Hash", func(t *testing.T) {
		buf, r, w := pipe()

		v := sha256.Sum256([]byte("test"))
		w.WriteHash(1, &v)
		wrOk(t, buf, w)

		u, ok := r.ReadHash(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, &v, u)
	})

	t.Run("Int", func(t *testing.T) {
		t.Run("Positive", func(t *testing.T) {
			buf, r, w := pipe()

			var v int64 = 123
			w.WriteInt(1, v)
			wrOk(t, buf, w)

			u, ok := r.ReadInt(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})

		t.Run("Negative", func(t *testing.T) {
			buf, r, w := pipe()

			var v int64 = -123
			w.WriteInt(1, v)
			wrOk(t, buf, w)

			u, ok := r.ReadInt(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})
	})

	t.Run("Uint", func(t *testing.T) {
		buf, r, w := pipe()

		var v uint64 = 123
		w.WriteUint(1, v)
		wrOk(t, buf, w)

		u, ok := r.ReadUint(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("Bool", func(t *testing.T) {
		t.Run("True", func(t *testing.T) {
			buf, r, w := pipe()

			var v = true
			w.WriteBool(1, v)
			wrOk(t, buf, w)

			u, ok := r.ReadBool(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})

		t.Run("False", func(t *testing.T) {
			buf, r, w := pipe()

			var v = false
			w.WriteBool(1, v)
			wrOk(t, buf, w)

			u, ok := r.ReadBool(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})

		t.Run("Invalid", func(t *testing.T) {
			buf, r, w := pipe()

			w.WriteUint(1, 2)
			wrOk(t, buf, w)

			_, ok := r.ReadBool(1)
			require.False(t, ok)
			_, err := r.Reset(nil)
			require.Error(t, err)
		})
	})

	t.Run("Time", func(t *testing.T) {
		buf, r, w := pipe()

		var v = time.Now().UTC()
		s := v.Format(time.RFC3339)
		v, err := time.Parse(time.RFC3339, s)
		require.NoError(t, err)

		w.WriteTime(1, v)
		wrOk(t, buf, w)

		u, ok := r.ReadTime(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u.UTC())
	})

	t.Run("Bytes", func(t *testing.T) {
		buf, r, w := pipe()

		var v = []byte{1, 2, 3}
		w.WriteBytes(1, v)
		wrOk(t, buf, w)

		u, ok := r.ReadBytes(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("String", func(t *testing.T) {
		buf, r, w := pipe()

		var v = "foo"
		w.WriteString(1, v)
		wrOk(t, buf, w)

		u, ok := r.ReadString(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("Duration", func(t *testing.T) {
		buf, r, w := pipe()

		var v = time.Hour + 23*time.Minute + 456*time.Nanosecond
		w.WriteDuration(1, v)
		wrOk(t, buf, w)

		u, ok := r.ReadDuration(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("BigInt", func(t *testing.T) {
		buf, r, w := pipe()

		var v = big.NewInt(789)
		w.WriteBigInt(1, v)
		wrOk(t, buf, w)

		u, ok := r.ReadBigInt(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("Url", func(t *testing.T) {
		buf, r, w := pipe()

		var v = &url.URL{Authority: "foo", Path: "/bar"}
		w.WriteUrl(1, v)
		wrOk(t, buf, w)

		u, ok := r.ReadUrl(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, u.String(), v.String())
	})
}

func TestFieldSkip(t *testing.T) {
	buf, r, w := pipe()

	w.WriteInt(2, 1)      // Write field 2
	wrOk(t, buf, w)       // (ok)
	_, ok := r.ReadInt(1) // Read field 1 -> not found
	require.False(t, ok)  //
	_, ok = r.ReadInt(2)  // Read field 2 -> found
	require.True(t, ok)   //
	rdOk(t, r)            // (ok)
}

func TestFieldRepeat(t *testing.T) {
	buf, r, w := pipe()

	w.WriteInt(3, 1)      // Write
	w.WriteInt(3, 2)      // Write
	wrOk(t, buf, w)       // (ok)
	_, ok := r.ReadInt(3) // Read -> found
	require.True(t, ok)   //
	_, ok = r.ReadInt(3)  // Read -> found
	require.True(t, ok)   //
	_, ok = r.ReadInt(3)  // Read -> not found
	require.False(t, ok)  //
	rdOk(t, r)            // (ok)
}

func TestFieldOutOfOrder(t *testing.T) {
	buf, r, w := pipe()

	w.WriteInt(2, 0)       // Write field 2
	w.WriteInt(1, 0)       // Write field 1
	wrOk(t, buf, w)        // (ok)
	_, ok := r.ReadInt(1)  // Read field 1
	require.False(t, ok)   //
	_, ok = r.ReadInt(2)   // Read field 2
	require.True(t, ok)    //
	_, ok = r.ReadInt(3)   // Read field 3
	require.False(t, ok)   //
	_, err := r.Reset(nil) // Error
	require.Error(t, err)  //
}
