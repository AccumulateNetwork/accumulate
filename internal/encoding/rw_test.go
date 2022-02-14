package encoding_test

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

func pipe() (*Reader, *Writer) {
	buf := new(bytes.Buffer)
	return NewReader(buf), NewWriter(buf)
}

func wrOk(t *testing.T, w *Writer) {
	_, _, err := w.Reset(nil)
	require.NoError(t, err)
}

func rdOk(t *testing.T, r *Reader) {
	_, err := r.Reset(nil)
	require.NoError(t, err)
}

func TestTypes(t *testing.T) {
	t.Run("Hash", func(t *testing.T) {
		r, w := pipe()

		v := sha256.Sum256([]byte("test"))
		w.WriteHash(1, &v)
		wrOk(t, w)

		u, ok := r.ReadHash(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, &v, u)
	})

	t.Run("Int", func(t *testing.T) {
		t.Run("Positive", func(t *testing.T) {
			r, w := pipe()

			var v int64 = 123
			w.WriteInt(1, v)
			wrOk(t, w)

			u, ok := r.ReadInt(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})

		t.Run("Negative", func(t *testing.T) {
			r, w := pipe()

			var v int64 = -123
			w.WriteInt(1, v)
			wrOk(t, w)

			u, ok := r.ReadInt(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})
	})

	t.Run("Uint", func(t *testing.T) {
		r, w := pipe()

		var v uint64 = 123
		w.WriteUint(1, v)
		wrOk(t, w)

		u, ok := r.ReadUint(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("Bool", func(t *testing.T) {
		t.Run("True", func(t *testing.T) {
			r, w := pipe()

			var v = true
			w.WriteBool(1, v)
			wrOk(t, w)

			u, ok := r.ReadBool(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})

		t.Run("False", func(t *testing.T) {
			r, w := pipe()

			var v = false
			w.WriteBool(1, v)
			wrOk(t, w)

			u, ok := r.ReadBool(1)
			require.True(t, ok)
			rdOk(t, r)
			require.Equal(t, v, u)
		})

		t.Run("Invalid", func(t *testing.T) {
			r, w := pipe()

			w.WriteUint(1, 2)
			wrOk(t, w)

			_, ok := r.ReadBool(1)
			require.False(t, ok)
			_, err := r.Reset(nil)
			require.Error(t, err)
		})
	})

	t.Run("Time", func(t *testing.T) {
		r, w := pipe()

		var v = time.Now().UTC()
		s := v.Format(time.RFC3339)
		v, err := time.Parse(time.RFC3339, s)
		require.NoError(t, err)

		w.WriteTime(1, v)
		wrOk(t, w)

		u, ok := r.ReadTime(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u.UTC())
	})

	t.Run("Bytes", func(t *testing.T) {
		r, w := pipe()

		var v = []byte{1, 2, 3}
		w.WriteBytes(1, v)
		wrOk(t, w)

		u, ok := r.ReadBytes(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("String", func(t *testing.T) {
		r, w := pipe()

		var v = "foo"
		w.WriteString(1, v)
		wrOk(t, w)

		u, ok := r.ReadString(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("Duration", func(t *testing.T) {
		r, w := pipe()

		var v = time.Hour + 23*time.Minute + 456*time.Nanosecond
		w.WriteDuration(1, v)
		wrOk(t, w)

		u, ok := r.ReadDuration(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})

	t.Run("BigInt", func(t *testing.T) {
		r, w := pipe()

		var v = big.NewInt(789)
		w.WriteBigInt(1, v)
		wrOk(t, w)

		u, ok := r.ReadBigInt(1)
		require.True(t, ok)
		rdOk(t, r)
		require.Equal(t, v, u)
	})
}

func TestFieldSkip(t *testing.T) {
	r, w := pipe()

	w.WriteInt(2, 1)      // Write field 2
	wrOk(t, w)            // (ok)
	_, ok := r.ReadInt(1) // Read field 1 -> not found
	require.False(t, ok)  //
	_, ok = r.ReadInt(2)  // Read field 2 -> found
	require.True(t, ok)   //
	rdOk(t, r)            // (ok)
}

func TestFieldRepeat(t *testing.T) {
	r, w := pipe()

	w.WriteInt(3, 1)      // Write
	w.WriteInt(3, 2)      // Write
	wrOk(t, w)            // (ok)
	_, ok := r.ReadInt(3) // Read -> found
	require.True(t, ok)   //
	_, ok = r.ReadInt(3)  // Read -> found
	require.True(t, ok)   //
	_, ok = r.ReadInt(3)  // Read -> not found
	require.False(t, ok)  //
	rdOk(t, r)            // (ok)
}

func TestFieldOutOfOrder(t *testing.T) {
	r, w := pipe()

	w.WriteInt(2, 0)       // Write field 2
	w.WriteInt(1, 0)       // Write field 1
	wrOk(t, w)             // (ok)
	_, ok := r.ReadInt(1)  // Read field 1
	require.False(t, ok)   //
	_, ok = r.ReadInt(2)   // Read field 2
	require.True(t, ok)    //
	_, ok = r.ReadInt(3)   // Read field 3
	require.False(t, ok)   //
	_, err := r.Reset(nil) // Error
	require.Error(t, err)  //
}
