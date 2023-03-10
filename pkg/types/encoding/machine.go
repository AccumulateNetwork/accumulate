// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Machine is an encoder machine that knows how to encode all the fields of a
// type.
type Machine[T any] struct {
	// Fields is a list of accessors for all of the field's types.
	Fields []*Field[T]

	// ExtraData returns the value's extra data field.
	ExtraData func(v T) *[]byte

	// Seen returns the value's seen fields field.
	Seen func(v T) *[]bool
}

// WriteTo writes the value's fields to the writer.
func (m *Machine[T]) WriteTo(w *Writer, v T) {
	for i, f := range m.Fields {
		f.WriteTo(w, uint(i+1), v)
	}
}

// MarshalBinary marshals the value's fields to binary.
func (m *Machine[T]) MarshalBinary(v T) ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := NewWriter(buffer)

	m.WriteTo(writer, v)

	_, _, err := writer.Reset(nil) // TODO field names
	if err != nil {
		return nil, Error{E: err}
	}
	buffer.Write(*m.ExtraData(v))
	return buffer.Bytes(), nil
}

// ReadFrom reads the value's fields from binary.
func (m *Machine[T]) ReadFrom(r *Reader, v T) {
	for i, f := range m.Fields {
		f.ReadFrom(r, uint(i+1), v)
	}
}

// Unmarshal unmarshals a value from binary.
func (m *Machine[T]) Unmarshal(b []byte, v T) error {
	return m.UnmarshalFrom(bytes.NewReader(b), v)
}

// UnmarshalFrom unmarshals a value from a reader.
func (m *Machine[T]) UnmarshalFrom(rd io.Reader, v T) error {
	reader := NewReader(rd)

	m.ReadFrom(reader, v)

	var err error
	*m.Seen(v), err = reader.Reset(nil) // TODO field names
	if err != nil {
		return Error{E: err}
	}
	*m.ExtraData(v), err = reader.ReadAll()
	if err != nil {
		return Error{E: err}
	}
	return nil
}

// ToJSON writes the value's fields as JSON to the builder.
func (m *Machine[T]) ToJSON(w *bytes.Buffer, v T) error {
	w.WriteRune('{')
	w2 := new(bytes.Buffer)
	var comma bool
	for _, f := range m.Fields {
		err := f.ToJSON(w2, v)
		if err != nil {
			return err
		}
		if w2.Len() == 0 {
			continue
		}
		if comma {
			w.WriteRune(',')
		}
		_, _ = w2.WriteTo(w)
		w2.Reset()
		comma = true
	}
	w.WriteRune('}')
	return nil
}

func (m *Machine[T]) JSONMarshal(v T) ([]byte, error) {
	w := new(bytes.Buffer)
	err := m.ToJSON(w, v)
	return w.Bytes(), err
}

// FromJSON reads the value's fields as JSON from bytes.
func (m *Machine[T]) FromJSON(b []byte, v T) error {
	var u map[string]json.RawMessage
	err := json.Unmarshal(b, &u)
	if err != nil {
		return err
	}

	w := make(map[string]json.RawMessage, len(u))
	for k, v := range u {
		w[strings.ToLower(k)] = v
	}

	for _, f := range m.Fields {
		b, ok := w[strings.ToLower(f.Name)]
		if !ok {
			continue
		}
		err = f.FromJSON(b, v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Machine[T]) JSONUnmarshal(b []byte, v T) error {
	return m.FromJSON(b, v)
}

// IsValid checks if the value's fields are valid.
func (m *Machine[T]) IsValid(v T) error {
	var errs []string

	seen := *m.Seen(v)

	for i, f := range m.Fields {
		if len(seen) > i && !seen[i] {
			errs = append(errs, "field "+f.Name+" is missing")
		} else if f.IsEmpty(v) {
			errs = append(errs, "field "+f.Name+" is not set")
		}
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

// Equal checks if V and U's fields are equal.
func (m *Machine[T]) Equal(v, u T) bool {
	// Check for reference equality
	if any(v) == any(u) {
		return true
	}

	// Check for nil
	var z T
	if any(v) == any(z) || any(u) == any(z) {
		return false
	}

	// Check fields
	for _, f := range m.Fields {
		if !f.Equal(v, u) {
			return false
		}
	}
	return true
}

// Copy constructs a value, copying all of the fields of an existing value.
func Copy[T any, PT interface{ *T }](m *Machine[PT], v PT) PT {
	if v == nil {
		return nil
	}
	u := PT(new(T))
	for _, f := range m.Fields {
		f.CopyTo(u, v)
	}
	return u
}
