// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import "bytes"

type valueAccessor[U any] interface {
	~func(U) *U
	Accessor[U]
}

type ReadOnlyAccessor[V, U any, A valueAccessor[U]] func(V) U

// val constructs an Accessor for U
func (ReadOnlyAccessor[V, U, A]) val() Accessor[U] { return A(func(u U) *U { return &u }) }

func (f ReadOnlyAccessor[V, U, A]) IsEmpty(v V) bool                  { return f.val().IsEmpty(f(v)) }
func (f ReadOnlyAccessor[V, U, A]) Equal(v, u V) bool                 { return f.val().Equal(f(v), f(u)) }
func (f ReadOnlyAccessor[V, U, A]) WriteTo(w *Writer, n uint, v V)    { f.val().WriteTo(w, n, f(v)) }
func (f ReadOnlyAccessor[V, U, A]) ToJSON(w *bytes.Buffer, v V) error { return f.val().ToJSON(w, f(v)) }

func (f ReadOnlyAccessor[V, U, A]) CopyTo(dst, src V)                    {}               // read-only, don't copy
func (f ReadOnlyAccessor[V, U, A]) ReadFrom(r *Reader, n uint, v V) bool { return false } // read-only, don't unmarshal
func (f ReadOnlyAccessor[V, U, A]) FromJSON(b []byte, v V) error         { return nil }   // read-only, don't unmarshal
