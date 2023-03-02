// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Field is a named accessor for a field of V.
type Field[V any] struct {
	Accessor[V]
	Name      string
	OmitEmpty bool
	Required  bool
}

// WriteTo writes the value's field to the writer, unless OmitEmpty is false and
// the field is empty.
func (f *Field[V]) WriteTo(w *Writer, n uint, v V) {
	if f.OmitEmpty && f.Accessor.IsEmpty(v) {
		return
	}
	f.Accessor.WriteTo(w, n, v)
}

// Accessor implements various functions for a value's field.
type Accessor[V any] interface {
	// IsEmpty checks if the field is empty.
	IsEmpty(V) bool

	// CopyTo copies the field from the source to the destination.
	CopyTo(dst, src V)

	// Equal checks if V's field and U's field are equal.
	Equal(v, u V) bool

	// WriteTo writes the value's field to the writer.
	WriteTo(w *Writer, n uint, v V)

	// ReadField reads the value's field from the reader.
	ReadFrom(r *Reader, n uint, v V) bool
}

// SliceField is an [Accessor] for a value's slice field.
type SliceField[V, U any, A sliceIndexAccessor[U]] func(v V) *[]U

// val constructs an Accessor for SliceIndex
func (SliceField[V, U, A]) val() Accessor[SliceIndex[U]] {
	return A(func(v SliceIndex[U]) *U { return &v.S[v.I] })
}

func (f SliceField[V, U, A]) IsEmpty(v V) bool {
	return len(*f(v)) == 0
}

func (f SliceField[V, U, A]) CopyTo(dst, src V) {
	g, a, b := f.val(), f(dst), f(src)
	*a = make([]U, len(*b))
	for i := range *b {
		g.CopyTo(SliceIndex[U]{*a, i}, SliceIndex[U]{*b, i})
	}
}

func (f SliceField[V, U, A]) Equal(v, u V) bool {
	g, a, b := f.val(), f(v), f(u)
	if len(*a) != len(*b) {
		return false
	}
	for i := range *b {
		if !g.Equal(SliceIndex[U]{*a, i}, SliceIndex[U]{*b, i}) {
			return false
		}
	}
	return true
}

func (f SliceField[V, U, A]) WriteTo(w *Writer, n uint, v V) {
	g, u := f.val(), f(v)
	for i := range *u {
		g.WriteTo(w, n, SliceIndex[U]{*u, i})
	}
}

func (f SliceField[V, U, A]) ReadFrom(r *Reader, n uint, v V) bool {
	g := f.val()
	var u []U
	var z U
	var i int
	for {
		u = append(u, z)
		if g.ReadFrom(r, n, SliceIndex[U]{u, i}) {
			i++
		} else {
			break
		}
	}

	if len(u) == 0 {
		return false
	}

	*f(v) = u[:i]
	return true
}

// StructField is an [Accessor] for a value's struct field.
type StructField[V any, U structPtr[W], W comparable] func(v V) U

func (f StructField[V, U, W]) IsEmpty(v V) bool {
	var z W
	return *(*W)(f(v)) == z
}

func (f StructField[V, U, W]) CopyTo(dst, src V) {
	*f(dst) = *f(src).CopyAsInterface().(U)
}

func (f StructField[V, U, W]) Equal(v, u V) bool {
	a, b := f(u), f(v)
	return a.Equal((*W)(b))
}

func (f StructField[V, U, W]) WriteTo(w *Writer, n uint, v V) {
	w.WriteValue(n, f(v).MarshalBinary)
}

func (f StructField[V, U, W]) ReadFrom(r *Reader, n uint, v V) bool {
	x := U(new(W))
	ok := r.ReadValue(n, x.UnmarshalBinaryFrom)
	if ok {
		*f(v) = *x
	}
	return ok
}

// StructPtrField is an [Accessor] for a value's struct pointer field.
type StructPtrField[V any, U structPtr[W], W any] func(v V) *U

func (f StructPtrField[V, U, W]) IsEmpty(v V) bool {
	return *f(v) == nil
}

func (f StructPtrField[V, U, W]) CopyTo(dst, src V) {
	*f(dst) = ((*f(src)).CopyAsInterface()).(U)
}

func (f StructPtrField[V, U, W]) Equal(v, u V) bool {
	a, b := f(u), f(v)
	if *a == *b {
		return true
	}
	if *a == nil || *b == nil {
		return false
	}
	return (*a).Equal((*W)(*b))
}

func (f StructPtrField[V, U, W]) WriteTo(w *Writer, n uint, v V) {
	w.WriteValue(n, (*f(v)).MarshalBinary)
}

func (f StructPtrField[V, U, W]) ReadFrom(r *Reader, n uint, v V) bool {
	x := U(new(W))
	ok := r.ReadValue(n, x.UnmarshalBinaryFrom)
	if ok {
		*f(v) = x
	}
	return ok
}

// EnumField is an [Accessor] for a value's enum field.
type EnumField[V any, U enumSet[W], W enumGet] func(v V) *W

// HashField is an [Accessor] for a value's [32]byte field.
type HashField[V any] func(v V) *[32]byte

// IntField is an [Accessor] for a value's int64 field.
type IntField[V any] func(v V) *int64

// UintField is an [Accessor] for a value's uint64 field.
type UintField[V any] func(v V) *uint64

// FloatField is an [Accessor] for a value's float64 field.
type FloatField[V any] func(v V) *float64

// BoolField is an [Accessor] for a value's bool field.
type BoolField[V any] func(v V) *bool

// TimeField is an [Accessor] for a value's time.Time field.
type TimeField[V any] func(v V) *time.Time

// BytesField is an [Accessor] for a value's []byte field.
type BytesField[V any] func(v V) *[]byte

// StringField is an [Accessor] for a value's string field.
type StringField[V any] func(v V) *string

// DurationField is an [Accessor] for a value's duration field.
type DurationField[V any] func(v V) *time.Duration

// BigIntField is an [Accessor] for a value's big int field.
type BigIntField[V any] func(v V) *big.Int

// UrlField is an [Accessor] for a value's Url field.
type UrlField[V any] func(v V) *url.URL

// TxIDField is an [Accessor] for a value's TxID field.
type TxIDField[V any] func(v V) *url.TxID

func (f EnumField[V, U, W]) IsEmpty(v V) bool { return (*f(v)).GetEnumValue() == 0 }
func (f HashField[V]) IsEmpty(v V) bool       { return isEmpty(f, v) }
func (f IntField[V]) IsEmpty(v V) bool        { return isEmpty(f, v) }
func (f UintField[V]) IsEmpty(v V) bool       { return isEmpty(f, v) }
func (f FloatField[V]) IsEmpty(v V) bool      { return isEmpty(f, v) }
func (f BoolField[V]) IsEmpty(v V) bool       { return isEmpty(f, v) }
func (f TimeField[V]) IsEmpty(v V) bool       { return isEmpty(f, v) }
func (f BytesField[V]) IsEmpty(v V) bool      { return isEmpty(f, v) }
func (f StringField[V]) IsEmpty(v V) bool     { return isEmpty(f, v) }
func (f DurationField[V]) IsEmpty(v V) bool   { return isEmpty(f, v) }
func (f BigIntField[V]) IsEmpty(v V) bool     { return f(v).Sign() == 0 }
func (f UrlField[V]) IsEmpty(v V) bool        { return isEmpty(f, v) }
func (f TxIDField[V]) IsEmpty(v V) bool       { return isEmpty(f, v) }

func (f EnumField[V, U, W]) CopyTo(dst, src V) { *f(dst) = *f(src) }
func (f HashField[V]) CopyTo(dst, src V)       { *f(dst) = *f(src) }
func (f IntField[V]) CopyTo(dst, src V)        { *f(dst) = *f(src) }
func (f UintField[V]) CopyTo(dst, src V)       { *f(dst) = *f(src) }
func (f FloatField[V]) CopyTo(dst, src V)      { *f(dst) = *f(src) }
func (f BoolField[V]) CopyTo(dst, src V)       { *f(dst) = *f(src) }
func (f TimeField[V]) CopyTo(dst, src V)       { *f(dst) = *f(src) }
func (f BytesField[V]) CopyTo(dst, src V)      { *f(dst) = *f(src) }
func (f StringField[V]) CopyTo(dst, src V)     { *f(dst) = *f(src) }
func (f DurationField[V]) CopyTo(dst, src V)   { *f(dst) = *f(src) }
func (f BigIntField[V]) CopyTo(dst, src V)     { *f(dst) = *f(src) }
func (f UrlField[V]) CopyTo(dst, src V)        { *f(dst) = *f(src) }
func (f TxIDField[V]) CopyTo(dst, src V)       { *f(dst) = *f(src) }

func (f EnumField[V, U, W]) Equal(v, u V) bool { return *f(v) == *f(u) }
func (f HashField[V]) Equal(v, u V) bool       { return *f(v) == *f(u) }
func (f IntField[V]) Equal(v, u V) bool        { return *f(v) == *f(u) }
func (f UintField[V]) Equal(v, u V) bool       { return *f(v) == *f(u) }
func (f FloatField[V]) Equal(v, u V) bool      { return *f(v) == *f(u) }
func (f BoolField[V]) Equal(v, u V) bool       { return *f(v) == *f(u) }
func (f TimeField[V]) Equal(v, u V) bool       { return *f(v) == *f(u) }
func (f BytesField[V]) Equal(v, u V) bool      { return bytes.Equal(*f(v), *f(u)) }
func (f StringField[V]) Equal(v, u V) bool     { return *f(v) == *f(u) }
func (f DurationField[V]) Equal(v, u V) bool   { return *f(v) == *f(u) }
func (f BigIntField[V]) Equal(v, u V) bool     { return f(v).Cmp(f(u)) == 0 }
func (f UrlField[V]) Equal(v, u V) bool        { return *f(v) == *f(u) }
func (f TxIDField[V]) Equal(v, u V) bool       { return *f(v) == *f(u) }

func (f EnumField[V, U, W]) WriteTo(w *Writer, n uint, v V) { w.WriteEnum(n, *f(v)) }
func (f HashField[V]) WriteTo(w *Writer, n uint, v V)       { w.WriteHash(n, f(v)) }
func (f IntField[V]) WriteTo(w *Writer, n uint, v V)        { w.WriteInt(n, *f(v)) }
func (f UintField[V]) WriteTo(w *Writer, n uint, v V)       { w.WriteUint(n, *f(v)) }
func (f FloatField[V]) WriteTo(w *Writer, n uint, v V)      { w.WriteFloat(n, *f(v)) }
func (f BoolField[V]) WriteTo(w *Writer, n uint, v V)       { w.WriteBool(n, *f(v)) }
func (f TimeField[V]) WriteTo(w *Writer, n uint, v V)       { w.WriteTime(n, *f(v)) }
func (f BytesField[V]) WriteTo(w *Writer, n uint, v V)      { w.WriteBytes(n, *f(v)) }
func (f StringField[V]) WriteTo(w *Writer, n uint, v V)     { w.WriteString(n, *f(v)) }
func (f DurationField[V]) WriteTo(w *Writer, n uint, v V)   { w.WriteDuration(n, *f(v)) }
func (f BigIntField[V]) WriteTo(w *Writer, n uint, v V)     { w.WriteBigInt(n, f(v)) }
func (f UrlField[V]) WriteTo(w *Writer, n uint, v V)        { w.WriteUrl(n, f(v)) }
func (f TxIDField[V]) WriteTo(w *Writer, n uint, v V)       { w.WriteTxid(n, f(v)) }

func (f HashField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdDeref(r.ReadHash, n, f, v) }
func (f IntField[V]) ReadFrom(r *Reader, n uint, v V) bool    { return rdVal(r.ReadInt, n, f, v) }
func (f UintField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdVal(r.ReadUint, n, f, v) }
func (f FloatField[V]) ReadFrom(r *Reader, n uint, v V) bool  { return rdVal(r.ReadFloat, n, f, v) }
func (f BoolField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdVal(r.ReadBool, n, f, v) }
func (f TimeField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdVal(r.ReadTime, n, f, v) }
func (f BytesField[V]) ReadFrom(r *Reader, n uint, v V) bool  { return rdVal(r.ReadBytes, n, f, v) }
func (f StringField[V]) ReadFrom(r *Reader, n uint, v V) bool { return rdVal(r.ReadString, n, f, v) }
func (f DurationField[V]) ReadFrom(r *Reader, n uint, v V) bool {
	return rdVal(r.ReadDuration, n, f, v)
}
func (f BigIntField[V]) ReadFrom(r *Reader, n uint, v V) bool { return rdDeref(r.ReadBigInt, n, f, v) }
func (f UrlField[V]) ReadFrom(r *Reader, n uint, v V) bool    { return rdDeref(r.ReadUrl, n, f, v) }
func (f TxIDField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdDeref(r.ReadTxid, n, f, v) }

// EnumPtrField is an [Accessor] for a value's enum pointer field.
type EnumPtrField[V any, U enumSet[W], W enumGet] func(v V) **W

// HashPtrField is an [Accessor] for a value's [32]byte pointer field.
type HashPtrField[V any] func(v V) **[32]byte

// IntPtrField is an [Accessor] for a value's int64 pointer field.
type IntPtrField[V any] func(v V) **int64

// UintPtrField is an [Accessor] for a value's uint64 pointer field.
type UintPtrField[V any] func(v V) **uint64

// FloatPtrField is an [Accessor] for a value's float64 pointer field.
type FloatPtrField[V any] func(v V) **float64

// BoolPtrField is an [Accessor] for a value's bool pointer field.
type BoolPtrField[V any] func(v V) **bool

// TimePtrField is an [Accessor] for a value's time.Time pointer field.
type TimePtrField[V any] func(v V) **time.Time

// BytesPtrField is an [Accessor] for a value's []byte pointer field.
type BytesPtrField[V any] func(v V) **[]byte

// StringPtrField is an [Accessor] for a value's string pointer field.
type StringPtrField[V any] func(v V) **string

// DurationPtrField is an [Accessor] for a value's Duration pointer field.
type DurationPtrField[V any] func(v V) **time.Duration

// BigIntPtrField is an [Accessor] for a value's big int pointer field.
type BigIntPtrField[V any] func(v V) **big.Int

// UrlPtrField is an [Accessor] for a value's Url pointer field.
type UrlPtrField[V any] func(v V) **url.URL

// TxIDPtrField is an [Accessor] for a value's TxID pointer field.
type TxIDPtrField[V any] func(v V) **url.TxID

func (f EnumPtrField[V, U, W]) IsEmpty(v V) bool { return (**f(v)).GetEnumValue() == 0 }
func (f HashPtrField[V]) IsEmpty(v V) bool       { return *f(v) == nil }
func (f IntPtrField[V]) IsEmpty(v V) bool        { return *f(v) == nil }
func (f UintPtrField[V]) IsEmpty(v V) bool       { return *f(v) == nil }
func (f FloatPtrField[V]) IsEmpty(v V) bool      { return *f(v) == nil }
func (f BoolPtrField[V]) IsEmpty(v V) bool       { return *f(v) == nil }
func (f TimePtrField[V]) IsEmpty(v V) bool       { return *f(v) == nil }
func (f BytesPtrField[V]) IsEmpty(v V) bool      { return *f(v) == nil }
func (f StringPtrField[V]) IsEmpty(v V) bool     { return *f(v) == nil }
func (f DurationPtrField[V]) IsEmpty(v V) bool   { return *f(v) == nil }
func (f BigIntPtrField[V]) IsEmpty(v V) bool     { return *f(v) == nil }
func (f UrlPtrField[V]) IsEmpty(v V) bool        { return *f(v) == nil }
func (f TxIDPtrField[V]) IsEmpty(v V) bool       { return *f(v) == nil }

func (f EnumPtrField[V, U, W]) CopyTo(dst, src V) { *f(dst) = *f(src) }
func (f HashPtrField[V]) CopyTo(dst, src V)       { cpPtr(f, dst, src) }
func (f IntPtrField[V]) CopyTo(dst, src V)        { cpPtr(f, dst, src) }
func (f UintPtrField[V]) CopyTo(dst, src V)       { cpPtr(f, dst, src) }
func (f FloatPtrField[V]) CopyTo(dst, src V)      { cpPtr(f, dst, src) }
func (f BoolPtrField[V]) CopyTo(dst, src V)       { cpPtr(f, dst, src) }
func (f TimePtrField[V]) CopyTo(dst, src V)       { cpPtr(f, dst, src) }
func (f BytesPtrField[V]) CopyTo(dst, src V)      { cpPtr(f, dst, src) }
func (f StringPtrField[V]) CopyTo(dst, src V)     { cpPtr(f, dst, src) }
func (f DurationPtrField[V]) CopyTo(dst, src V)   { cpPtr(f, dst, src) }
func (f BigIntPtrField[V]) CopyTo(dst, src V)     { cpPtr(f, dst, src) }
func (f UrlPtrField[V]) CopyTo(dst, src V)        { cpPtr(f, dst, src) }
func (f TxIDPtrField[V]) CopyTo(dst, src V)       { cpPtr(f, dst, src) }

func (f EnumPtrField[V, U, W]) Equal(v, u V) bool { return *f(v) == *f(u) }
func (f HashPtrField[V]) Equal(v, u V) bool       { return eqPtr(f, v, u) }
func (f IntPtrField[V]) Equal(v, u V) bool        { return eqPtr(f, v, u) }
func (f UintPtrField[V]) Equal(v, u V) bool       { return eqPtr(f, v, u) }
func (f FloatPtrField[V]) Equal(v, u V) bool      { return eqPtr(f, v, u) }
func (f BoolPtrField[V]) Equal(v, u V) bool       { return eqPtr(f, v, u) }
func (f TimePtrField[V]) Equal(v, u V) bool       { return eqPtr(f, v, u) }
func (f StringPtrField[V]) Equal(v, u V) bool     { return eqPtr(f, v, u) }
func (f DurationPtrField[V]) Equal(v, u V) bool   { return eqPtr(f, v, u) }
func (f UrlPtrField[V]) Equal(v, u V) bool        { return eqPtr(f, v, u) }
func (f TxIDPtrField[V]) Equal(v, u V) bool       { return eqPtr(f, v, u) }

func (f EnumPtrField[V, U, W]) WriteTo(w *Writer, n uint, v V) { w.WriteEnum(n, **f(v)) }
func (f HashPtrField[V]) WriteTo(w *Writer, n uint, v V)       { wrRef(w.WriteHash, n, f, v) }
func (f IntPtrField[V]) WriteTo(w *Writer, n uint, v V)        { wrPtr(w.WriteInt, n, f, v) }
func (f UintPtrField[V]) WriteTo(w *Writer, n uint, v V)       { wrPtr(w.WriteUint, n, f, v) }
func (f FloatPtrField[V]) WriteTo(w *Writer, n uint, v V)      { wrPtr(w.WriteFloat, n, f, v) }
func (f BoolPtrField[V]) WriteTo(w *Writer, n uint, v V)       { wrPtr(w.WriteBool, n, f, v) }
func (f TimePtrField[V]) WriteTo(w *Writer, n uint, v V)       { wrPtr(w.WriteTime, n, f, v) }
func (f BytesPtrField[V]) WriteTo(w *Writer, n uint, v V)      { wrPtr(w.WriteBytes, n, f, v) }
func (f StringPtrField[V]) WriteTo(w *Writer, n uint, v V)     { wrPtr(w.WriteString, n, f, v) }
func (f DurationPtrField[V]) WriteTo(w *Writer, n uint, v V)   { wrPtr(w.WriteDuration, n, f, v) }
func (f BigIntPtrField[V]) WriteTo(w *Writer, n uint, v V)     { wrRef(w.WriteBigInt, n, f, v) }
func (f UrlPtrField[V]) WriteTo(w *Writer, n uint, v V)        { wrRef(w.WriteUrl, n, f, v) }
func (f TxIDPtrField[V]) WriteTo(w *Writer, n uint, v V)       { wrRef(w.WriteTxid, n, f, v) }

func (f HashPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdVal(r.ReadHash, n, f, v) }
func (f IntPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool    { return rdPtr(r.ReadInt, n, f, v) }
func (f UintPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdPtr(r.ReadUint, n, f, v) }
func (f FloatPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool  { return rdPtr(r.ReadFloat, n, f, v) }
func (f BoolPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdPtr(r.ReadBool, n, f, v) }
func (f TimePtrField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdPtr(r.ReadTime, n, f, v) }
func (f BytesPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool  { return rdPtr(r.ReadBytes, n, f, v) }
func (f StringPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool { return rdPtr(r.ReadString, n, f, v) }
func (f BigIntPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool { return rdVal(r.ReadBigInt, n, f, v) }
func (f UrlPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool    { return rdVal(r.ReadUrl, n, f, v) }
func (f TxIDPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool   { return rdVal(r.ReadTxid, n, f, v) }

func (f EnumField[V, U, W]) ReadFrom(r *Reader, n uint, v V) bool {
	x := U(new(W))
	ok := r.ReadEnum(n, x)
	if ok {
		*f(v) = *x
	}
	return ok
}

func (f EnumPtrField[V, U, W]) ReadFrom(r *Reader, n uint, v V) bool {
	x := U(new(W))
	ok := r.ReadEnum(n, x)
	if ok {
		*f(v) = (*W)(x)
	}
	return ok
}

func (f DurationPtrField[V]) ReadFrom(r *Reader, n uint, v V) bool {
	return rdPtr(r.ReadDuration, n, f, v)
}

func (f BytesPtrField[V]) Equal(v, u V) bool {
	a, b := f(u), f(v)
	if *a == *b {
		return true
	}
	if *a == nil || *b == nil {
		return false
	}
	return bytes.Equal(**a, **b)
}

func (f BigIntPtrField[V]) Equal(v, u V) bool {
	a, b := f(u), f(v)
	if *a == *b {
		return true
	}
	if *a == nil || *b == nil {
		return false
	}
	return (*a).Cmp(*b) == 0
}

func isEmpty[V, U any](f func(V) *U, v V) bool {
	var z U
	return any(*f(v)) == any(z)
}

func rdVal[V, U any](rd func(uint) (U, bool), n uint, f func(V) *U, v V) bool {
	x, ok := rd(n)
	if ok {
		*f(v) = x
	}
	return ok
}

func rdDeref[V, U any](rd func(uint) (*U, bool), n uint, f func(V) *U, v V) bool {
	x, ok := rd(n)
	if ok {
		*f(v) = *x
	}
	return ok
}

func wrRef[V, U any](wr func(uint, *U), n uint, f func(V) **U, v V) {
	if *f(v) != nil {
		wr(n, *f(v))
	}
}

func cpPtr[V, U any](f func(V) **U, dstv, srcv V) {
	dst, src := f(dstv), f(srcv)
	if *dst == *src {
		return
	}
	if *src == nil {
		*dst = nil
		return
	}
	*dst = new(U)
	**dst = **src
}

func eqPtr[U comparable, V any](f func(V) **U, u, v V) bool {
	a, b := f(u), f(v)
	if *a == *b {
		return true
	}
	if *a == nil || *b == nil {
		return false
	}
	return **a == **b
}

func wrPtr[V, U any](wr func(uint, U), n uint, f func(V) **U, v V) {
	if *f(v) != nil {
		wr(n, **f(v))
	}
}

func rdPtr[V, U any](rd func(uint) (U, bool), n uint, f func(V) **U, v V) bool {
	x, ok := rd(n)
	if ok {
		*f(v) = &x
	}
	return ok
}

// SliceIndex is a slice and an index.
type SliceIndex[U any] struct {
	S []U
	I int
}

// sliceIndexAccessor is a type constraint for an accessor of SliceIndex that is
// also a function.
type sliceIndexAccessor[U any] interface {
	~func(SliceIndex[U]) *U
	Accessor[SliceIndex[U]]
}

// structPointer is a type constraint for a struct type that implements
// BinaryValue with a pointer receiver.
type structPtr[V any] interface {
	*V
	BinaryValue
	Equal(*V) bool
}

// enumSet is a type constraint for an enum type that implements EnumValueSetter
// with a pointer receiver.
type enumSet[V any] interface {
	*V
	EnumValueSetter
}

// enumGet is a type constraint for an enum type that is comparable and
// implements EnumValueGetter.
type enumGet interface {
	comparable
	EnumValueGetter
}

// Make sure all the accessors actually implement the interface
var _ Accessor[struct{}] = HashField[struct{}](nil)
var _ Accessor[struct{}] = IntField[struct{}](nil)
var _ Accessor[struct{}] = UintField[struct{}](nil)
var _ Accessor[struct{}] = FloatField[struct{}](nil)
var _ Accessor[struct{}] = BoolField[struct{}](nil)
var _ Accessor[struct{}] = TimeField[struct{}](nil)
var _ Accessor[struct{}] = BytesField[struct{}](nil)
var _ Accessor[struct{}] = StringField[struct{}](nil)
var _ Accessor[struct{}] = DurationField[struct{}](nil)
var _ Accessor[struct{}] = BigIntField[struct{}](nil)
var _ Accessor[struct{}] = UrlField[struct{}](nil)
var _ Accessor[struct{}] = TxIDField[struct{}](nil)
var _ Accessor[struct{}] = HashPtrField[struct{}](nil)
var _ Accessor[struct{}] = IntPtrField[struct{}](nil)
var _ Accessor[struct{}] = UintPtrField[struct{}](nil)
var _ Accessor[struct{}] = FloatPtrField[struct{}](nil)
var _ Accessor[struct{}] = BoolPtrField[struct{}](nil)
var _ Accessor[struct{}] = TimePtrField[struct{}](nil)
var _ Accessor[struct{}] = BytesPtrField[struct{}](nil)
var _ Accessor[struct{}] = StringPtrField[struct{}](nil)
var _ Accessor[struct{}] = DurationPtrField[struct{}](nil)
var _ Accessor[struct{}] = BigIntPtrField[struct{}](nil)
var _ Accessor[struct{}] = UrlPtrField[struct{}](nil)
var _ Accessor[struct{}] = TxIDPtrField[struct{}](nil)
