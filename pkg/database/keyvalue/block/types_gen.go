// Code generated by gitlab.com/accumulatenetwork/core/schema. DO NOT EDIT.

package block

import (
	record "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

type endBlockEntry struct {
}

var wendBlockEntry = widget.ForCompositePtr(widget.Fields[endBlockEntry]{
	{Name: "type", ID: 1, Widget: widget.ForTag[*entryType]("type", (*endBlockEntry).Type)},
}, widget.Identity[**endBlockEntry])

func (endBlockEntry) Type() entryType { return entryTypeEndBlock }

// Copy returns a copy of the endBlockEntry.
func (v *endBlockEntry) Copy() *endBlockEntry {
	var u *endBlockEntry
	wendBlockEntry.CopyTo(&u, &v)
	return u
}

// EqualendBlockEntry returns true if V is equal to U.
func (v *endBlockEntry) Equal(u *endBlockEntry) bool {
	return wendBlockEntry.Equal(&v, &u)
}

// MarshalBinary marshals the endBlockEntry to JSON.
func (v *endBlockEntry) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(&v, wendBlockEntry)
}

// UnmarshalJSON unmarshals the endBlockEntry from JSON.
func (v *endBlockEntry) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(&v, wendBlockEntry, b)
}

// MarshalBinary marshals the endBlockEntry to bytes using [binary].
func (v *endBlockEntry) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(&v, wendBlockEntry)
}

// MarshalBinary marshals the endBlockEntry to a [binary.Encoder].
func (v *endBlockEntry) MarshalBinaryV2(enc *binary.Encoder) error {
	return wendBlockEntry.MarshalBinary(enc, &v)
}

// UnmarshalBinary unmarshals the endBlockEntry from bytes using [binary].
func (v *endBlockEntry) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(&v, wendBlockEntry, b)
}

// UnmarshalBinary unmarshals the endBlockEntry from a [binary.Decoder].
func (v *endBlockEntry) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wendBlockEntry.UnmarshalBinary(dec, &v)
}

// TODO type entry interface {}

var wentry = widget.ForUnion[*entryType](
	"type",
	entry.Type,
	map[entryType]widget.UnionMemberFields[entry]{
		entryTypeStartBlock: &widget.UnionMember[entry, startBlockEntry]{
			Fields: wstartBlockEntry.Fields,
		},
		entryTypeRecord: &widget.UnionMember[entry, recordEntry]{
			Fields: wrecordEntry.Fields,
		},
		entryTypeEndBlock: &widget.UnionMember[entry, endBlockEntry]{
			Fields: wendBlockEntry.Fields,
		},
	},
	widget.Identity[*entry])

// copyEntry returns a copy of the entry.
func copyEntry(v entry) entry {
	var u entry
	wentry.CopyTo(&u, &v)
	return u
}

// equalEntry returns true if A and B are equal.
func equalEntry(a, b entry) bool {
	return wentry.Equal(&a, &b)
}

// unmarshalEntryJSON unmarshals a entry from JSON.
func unmarshalEntryJSON(b []byte) (entry, error) {
	var v entry
	err := widget.UnmarshalJSON(&v, wentry, b)
	return v, err
}

// unmarshalEntryBinary unmarshals a entry from bytes using [binary].
func unmarshalEntryBinary(b []byte) (entry, error) {
	var v entry
	err := widget.UnmarshalBinary(&v, wentry, b)
	return v, err
}

// unmarshalEntryBinary unmarshals a entry from a [binary.Decoder].
func unmarshalEntryBinaryV2(dec *binary.Decoder) (entry, error) {
	var v entry
	err := wentry.UnmarshalBinary(dec, &v)
	return v, err
}

type entryType uint64

const (
	entryTypeEndBlock   entryType = 3
	entryTypeRecord     entryType = 2
	entryTypeStartBlock entryType = 1
)

var wentryType = widget.ForEnum[*entryType](widget.Identity[*entryType])

// SetByName looks up a entryType by name.
func (v *entryType) SetByName(s string) (ok bool) {
	*v, ok = sentryType.ByName(s)
	return
}

// SetByValue looks up a entryType by value.
func (v *entryType) SetByValue(i int64) (ok bool) {
	*v, ok = sentryType.ByValue(i)
	return
}

// String returns the label or name of the entryType.
func (v entryType) String() string {
	return sentryType.String(v)
}

// MarshalBinary marshals the entryType to JSON.
func (v entryType) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(&v, wentryType)
}

// UnmarshalJSON unmarshals the entryType from JSON.
func (v *entryType) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(v, wentryType, b)
}

// MarshalBinary marshals the entryType to bytes using [binary].
func (v entryType) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(&v, wentryType)
}

// MarshalBinary marshals the entryType to a [binary.Encoder].
func (v entryType) MarshalBinaryV2(enc *binary.Encoder) error {
	return wentryType.MarshalBinary(enc, &v)
}

// UnmarshalBinary unmarshals the entryType from bytes using [binary].
func (v *entryType) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(v, wentryType, b)
}

// UnmarshalBinary unmarshals the entryType from a [binary.Decoder].
func (v *entryType) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wentryType.UnmarshalBinary(dec, v)
}

type fileHeader struct {
	Ordinal uint64
}

var wfileHeader = widget.ForCompositePtr(widget.Fields[fileHeader]{
	{Name: "ordinal", ID: 1, Widget: widget.ForUint(func(v *fileHeader) *uint64 { return &v.Ordinal })},
}, widget.Identity[**fileHeader])

// Copy returns a copy of the fileHeader.
func (v *fileHeader) Copy() *fileHeader {
	var u *fileHeader
	wfileHeader.CopyTo(&u, &v)
	return u
}

// EqualfileHeader returns true if V is equal to U.
func (v *fileHeader) Equal(u *fileHeader) bool {
	return wfileHeader.Equal(&v, &u)
}

// MarshalBinary marshals the fileHeader to JSON.
func (v *fileHeader) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(&v, wfileHeader)
}

// UnmarshalJSON unmarshals the fileHeader from JSON.
func (v *fileHeader) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(&v, wfileHeader, b)
}

// MarshalBinary marshals the fileHeader to bytes using [binary].
func (v *fileHeader) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(&v, wfileHeader)
}

// MarshalBinary marshals the fileHeader to a [binary.Encoder].
func (v *fileHeader) MarshalBinaryV2(enc *binary.Encoder) error {
	return wfileHeader.MarshalBinary(enc, &v)
}

// UnmarshalBinary unmarshals the fileHeader from bytes using [binary].
func (v *fileHeader) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(&v, wfileHeader, b)
}

// UnmarshalBinary unmarshals the fileHeader from a [binary.Decoder].
func (v *fileHeader) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wfileHeader.UnmarshalBinary(dec, &v)
}

type recordEntry struct {
	Key     *record.Key
	KeyHash [32]byte
	Length  int64
}

var wrecordEntry = widget.ForCompositePtr(widget.Fields[recordEntry]{
	{Name: "type", ID: 1, Widget: widget.ForTag[*entryType]("type", (*recordEntry).Type)},
	{Name: "key", ID: 2, Widget: widget.ForValue(func(v *recordEntry) **record.Key { return &v.Key })},
	{Name: "keyHash", ID: 3, Widget: widget.ForHash(func(v *recordEntry) *[32]byte { return &v.KeyHash })},
	{Name: "length", ID: 4, Widget: widget.ForInt(func(v *recordEntry) *int64 { return &v.Length })},
}, widget.Identity[**recordEntry])

func (recordEntry) Type() entryType { return entryTypeRecord }

// Copy returns a copy of the recordEntry.
func (v *recordEntry) Copy() *recordEntry {
	var u *recordEntry
	wrecordEntry.CopyTo(&u, &v)
	return u
}

// EqualrecordEntry returns true if V is equal to U.
func (v *recordEntry) Equal(u *recordEntry) bool {
	return wrecordEntry.Equal(&v, &u)
}

// MarshalBinary marshals the recordEntry to JSON.
func (v *recordEntry) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(&v, wrecordEntry)
}

// UnmarshalJSON unmarshals the recordEntry from JSON.
func (v *recordEntry) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(&v, wrecordEntry, b)
}

// MarshalBinary marshals the recordEntry to bytes using [binary].
func (v *recordEntry) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(&v, wrecordEntry)
}

// MarshalBinary marshals the recordEntry to a [binary.Encoder].
func (v *recordEntry) MarshalBinaryV2(enc *binary.Encoder) error {
	return wrecordEntry.MarshalBinary(enc, &v)
}

// UnmarshalBinary unmarshals the recordEntry from bytes using [binary].
func (v *recordEntry) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(&v, wrecordEntry, b)
}

// UnmarshalBinary unmarshals the recordEntry from a [binary.Decoder].
func (v *recordEntry) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wrecordEntry.UnmarshalBinary(dec, &v)
}

type startBlockEntry struct {
	ID   uint64
	Part uint64
}

var wstartBlockEntry = widget.ForCompositePtr(widget.Fields[startBlockEntry]{
	{Name: "type", ID: 1, Widget: widget.ForTag[*entryType]("type", (*startBlockEntry).Type)},
	{Name: "iD", ID: 2, Widget: widget.ForUint(func(v *startBlockEntry) *uint64 { return &v.ID })},
	{Name: "part", ID: 3, Widget: widget.ForUint(func(v *startBlockEntry) *uint64 { return &v.Part })},
}, widget.Identity[**startBlockEntry])

func (startBlockEntry) Type() entryType { return entryTypeStartBlock }

// Copy returns a copy of the startBlockEntry.
func (v *startBlockEntry) Copy() *startBlockEntry {
	var u *startBlockEntry
	wstartBlockEntry.CopyTo(&u, &v)
	return u
}

// EqualstartBlockEntry returns true if V is equal to U.
func (v *startBlockEntry) Equal(u *startBlockEntry) bool {
	return wstartBlockEntry.Equal(&v, &u)
}

// MarshalBinary marshals the startBlockEntry to JSON.
func (v *startBlockEntry) MarshalJSON() ([]byte, error) {
	return widget.MarshalJSON(&v, wstartBlockEntry)
}

// UnmarshalJSON unmarshals the startBlockEntry from JSON.
func (v *startBlockEntry) UnmarshalJSON(b []byte) error {
	return widget.UnmarshalJSON(&v, wstartBlockEntry, b)
}

// MarshalBinary marshals the startBlockEntry to bytes using [binary].
func (v *startBlockEntry) MarshalBinary() ([]byte, error) {
	return widget.MarshalBinary(&v, wstartBlockEntry)
}

// MarshalBinary marshals the startBlockEntry to a [binary.Encoder].
func (v *startBlockEntry) MarshalBinaryV2(enc *binary.Encoder) error {
	return wstartBlockEntry.MarshalBinary(enc, &v)
}

// UnmarshalBinary unmarshals the startBlockEntry from bytes using [binary].
func (v *startBlockEntry) UnmarshalBinary(b []byte) error {
	return widget.UnmarshalBinary(&v, wstartBlockEntry, b)
}

// UnmarshalBinary unmarshals the startBlockEntry from a [binary.Decoder].
func (v *startBlockEntry) UnmarshalBinaryV2(dec *binary.Decoder) error {
	return wstartBlockEntry.UnmarshalBinary(dec, &v)
}
