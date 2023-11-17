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
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gopkg.in/yaml.v3"
)

func (c *Config) LoadFrom(file string) error {
	var decode func([]byte, any) error
	switch s := filepath.Ext(file); s {
	case ".toml", ".tml", ".ini":
		decode = toml.Unmarshal
	case ".yaml", ".yml":
		decode = yaml.Unmarshal
	case ".json":
		decode = json.Unmarshal
	default:
		return errors.BadRequest.WithFormat("unknown file type %s", s)
	}

	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	var v any
	err = decode(b, &v)
	if err != nil {
		return err
	}

	v = remap(v, kebab2camel)
	b, err = json.Marshal(v)
	if err != nil {
		return err
	}

	c.file = file
	return json.Unmarshal(b, c)
}

func (c *Config) Save() error {
	if c.file == "" {
		return errors.BadRequest.With("not loaded from a file")
	}
	return c.SaveTo(c.file)
}

func (c *Config) SaveTo(file string) error {
	var encode func(any) ([]byte, error)
	switch s := filepath.Ext(file); s {
	case ".toml", ".tml", ".ini":
		encode = func(a any) ([]byte, error) {
			b := new(bytes.Buffer)
			e := toml.NewEncoder(b)
			err := e.Encode(a)
			return b.Bytes(), err
		}
	case ".yaml", ".yml":
		encode = yaml.Marshal
	case ".json":
		encode = json.Marshal
	default:
		return errors.BadRequest.WithFormat("unknown file type %s", s)
	}

	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	var v any
	err = json.Unmarshal(b, &v)
	if err != nil {
		return err
	}

	v = remap(v, camel2kebab)
	b, err = encode(v)
	if err != nil {
		return err
	}

	return os.WriteFile(file, b, 0700)
}

func remap(v any, fn func(string) string) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}

	n := make(map[string]any, len(m))
	for k, v := range m {
		n[fn(k)] = remap(v, fn)
	}
	return n
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
