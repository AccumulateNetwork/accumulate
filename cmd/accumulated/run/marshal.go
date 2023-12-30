// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gopkg.in/yaml.v3"
)

func (c *Config) LoadFrom(file string) error {
	var format func([]byte, any) error
	switch s := filepath.Ext(file); s {
	case ".toml", ".tml", ".ini":
		format = toml.Unmarshal
	case ".yaml", ".yml":
		format = yaml.Unmarshal
	case ".json":
		format = json.Unmarshal
	default:
		return errors.BadRequest.WithFormat("unknown file type %s", s)
	}

	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	c.file = file
	return c.Load(b, format)
}

func (c *Config) Load(b []byte, format func([]byte, any) error) error {
	var v any
	err := format(b, &v)
	if err != nil {
		return err
	}

	v = remap(v, kebab2camel)
	b, err = json.Marshal(v)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, c)
}

func (c *Config) Save() error {
	if c.file == "" {
		return errors.BadRequest.With("not loaded from a file")
	}
	return c.SaveTo(c.file)
}

func (c *Config) SaveTo(file string) error {
	var format func(any) ([]byte, error)
	switch s := filepath.Ext(file); s {
	case ".toml", ".tml", ".ini":
		format = MarshalTOML
	case ".yaml", ".yml":
		format = yaml.Marshal
	case ".json":
		format = json.Marshal
	default:
		return errors.BadRequest.WithFormat("unknown file type %s", s)
	}

	b, err := c.Marshal(format)
	if err != nil {
		return err
	}

	return os.WriteFile(file, b, 0700)
}

func MarshalTOML(a any) ([]byte, error) {
	b := new(bytes.Buffer)
	e := toml.NewEncoder(b)
	err := e.Encode(a)
	return b.Bytes(), err
}

func (c *Config) Marshal(format func(any) ([]byte, error)) ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	var v any
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}

	v = remap(v, camel2kebab)
	return format(v)
}

func remap(v any, fn func(string) string) any {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice:
		u := make([]any, rv.Len())
		for i := range u {
			u[i] = remap(rv.Index(i).Interface(), fn)
		}
		return u

	case reflect.Map:
		u := make(map[string]any, rv.Len())
		for it := rv.MapRange(); it.Next(); {
			u[fn(it.Key().String())] = remap(it.Value().Interface(), fn)
		}
		return u

	default:
		return v
	}
}

var reKebab = regexp.MustCompile(`-[a-z]`)
var reCamel = regexp.MustCompile(`[a-z][A-Z]+`)

func kebab2camel(s string) string {
	return reKebab.ReplaceAllStringFunc(s, func(s string) string {
		return strings.ToUpper(s[1:])
	})
}

func camel2kebab(s string) string {
	return strings.ToLower(reCamel.ReplaceAllStringFunc(s, func(s string) string {
		return s[:1] + "-" + s[1:]
	}))
}

// type orderedMap []kvp

// type kvp struct {
// 	key   string
// 	value any
// }

// func (m *orderedMap) UnmarshalJSON(b []byte) error {
// 	json.Token
// }
