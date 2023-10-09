// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/exp/slog"
)

type LogAsHex interface {
	Slice(i, j int) LogAsHex
}

type LogAsHexValue []byte

var _ json.Marshaler = LogAsHexValue{}
var _ slog.LogValuer = LogAsHexValue{}

func (v LogAsHexValue) MarshalJSON() ([]byte, error) {
	b := make([]byte, hex.EncodedLen(len(v)))
	hex.Encode(b, v)
	return json.Marshal(strings.ToUpper(string(b)))
}

func (v LogAsHexValue) LogValue() slog.Value {
	return slog.StringValue(hex.EncodeToString(v))
}

func (v LogAsHexValue) Slice(i, j int) LogAsHex {
	if i < 0 {
		i = 0
	}
	if i > len(v) {
		i = len(v)
	}
	if j < 0 {
		j = 0
	}
	if j > len(v) {
		j = len(v)
	}
	return v[i:j]
}

type LogAsHexSlice []LogAsHex

func (v LogAsHexSlice) Slice(i, j int) LogAsHex {
	u := make(LogAsHexSlice, len(v))
	for i, v := range v {
		u[i] = v.Slice(i, j)
	}
	return u
}

//go:inline
func AsHex(v interface{}) LogAsHex {
	switch v := v.(type) {
	case []byte:
		u := make(LogAsHexValue, len(v))
		copy(u, v)
		return u
	case [32]byte:
		return LogAsHexValue(v[:])
	case *[32]byte:
		return LogAsHexValue(v[:])
	case string:
		return LogAsHexValue(v)
	case interface{ Bytes() []byte }:
		return LogAsHexValue(v.Bytes())
	case fmt.Stringer:
		return LogAsHexValue(v.String())
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		v := make(LogAsHexSlice, rv.Len())
		for i := range v {
			v[i] = AsHex(rv.Index(i).Interface())
		}
		return v
	}

	return LogAsHexValue(fmt.Sprint(v))
}

type LogWithFormat struct {
	Format string
	Values []interface{}
}

func (v LogWithFormat) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf(v.Format, v.Values...))
}

//go:inline
func WithFormat(format string, values ...interface{}) LogWithFormat {
	return LogWithFormat{format, values}
}
