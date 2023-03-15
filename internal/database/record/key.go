// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// A Key is the key for a record.
type Key []interface{}

// Append creates a child key of this key.
func (k Key) Append(v ...interface{}) Key {
	l := make(Key, len(k)+len(v))
	n := copy(l, k)
	copy(l[n:], v)
	return l
}

// Hash converts the record key to a storage key.
func (k Key) Hash() storage.Key {
	return storage.MakeKey(k...)
}

// String returns a human-readable string for the key.
func (k Key) String() string {
	s := make([]string, len(k))
	for i, v := range k {
		switch v := v.(type) {
		case []byte:
			s[i] = hex.EncodeToString(v)
		case [32]byte:
			s[i] = hex.EncodeToString(v[:])
		default:
			s[i] = fmt.Sprint(v)
		}
	}
	return strings.Join(s, ".")
}

// // MarshalJSON is implemented so keys are formatted nicely by zerolog.
// func (k Key) MarshalJSON() ([]byte, error) {
// 	return json.Marshal(k.String())
// }

func (k Key) Copy() Key {
	l := make(Key, len(k))
	copy(l, k)
	return l
}

func (k Key) Equal(l Key) bool {
	if len(k) != len(l) {
		return false
	}
	for i := range k {
		if !keyPartsEqual(k[i], l[i]) {
			return false
		}
	}
	return true
}

func (k *Key) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.Write(encoding.MarshalUint(uint64(len(*k))))
	w := encoding.NewWriter(buf)

	// This is an abuse but 🤷 it works
	for _, v := range *k {
		p, err := asKeyPart(v)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		p.WriteBinary(w)
	}

	_, _, err := w.Reset(nil)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (k *Key) UnmarshalBinary(b []byte) error {
	return k.UnmarshalBinaryFrom(bytes.NewBuffer(b))
}

func (k *Key) UnmarshalBinaryFrom(rd io.Reader) error {
	br, ok := rd.(encoding.BytesReader)
	if !ok {
		br = bufio.NewReader(rd)
	}

	n, err := binary.ReadUvarint(br)
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}
	*k = make(Key, n)

	// This is an abuse but 🤷 it works
	r := encoding.NewReader(br)
	for i := uint64(0); i < n; i++ {
		v, err := binary.ReadUvarint(br)
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}
		p, err := newKeyPart(typeCode(v))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		p.ReadBinary(r)
		(*k)[i] = p.Value()
	}

	_, err = r.Reset(nil)
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}
	return nil
}

func (k Key) MarshalJSON() ([]byte, error) {
	parts := make([]map[string]any, len(k))
	for i, v := range k {
		p, err := asKeyPart(v)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		parts[i] = map[string]any{p.Type().String(): p}
	}
	return json.Marshal(parts)
}

func (k *Key) UnmarshalJSON(b []byte) error {
	var parts []map[string]json.RawMessage
	err := json.Unmarshal(b, &parts)
	if err != nil {
		return err
	}

	*k = make(Key, len(parts))
	for i, p := range parts {
		if len(p) != 1 {
			return errors.EncodingError.WithFormat("invalid key part: expected { typ: val }, got %v", parts)
		}

		var s string
		var b json.RawMessage
		for s, b = range p {
		}

		typ, ok := typeCodeByName(s)
		if !ok {
			return errors.EncodingError.WithFormat("%q is not a valid type code", s)
		}

		kp, err := newKeyPart(typ)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		err = json.Unmarshal(b, &kp)
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}

		(*k)[i] = kp.Value()
	}
	return nil
}
