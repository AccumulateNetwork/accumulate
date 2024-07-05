// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/joho/godotenv"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gopkg.in/yaml.v3"
)

func (c *Config) FilePath() string     { return c.file }
func (c *Config) SetFilePath(p string) { c.file = p }

func (c *Config) LoadFrom(file string) error {
	return c.LoadFromFS(os.DirFS("."), file)
}

func (c *Config) LoadFromFS(fs fs.FS, file string) error {
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

	f, err := fs.Open(file)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	c.file = file
	c.fs = fs
	return c.Load(b, format)
}

func (c *Config) Load(b []byte, format func([]byte, any) error) error {
	var v any
	err := format(b, &v)
	if err != nil {
		return err
	}

	v = remap(v, kebab2camel, nil)
	b, err = json.Marshal(v)
	if err != nil {
		return err
	}

	err = json.Unmarshal(b, c)
	if err != nil {
		return err
	}

	return c.applyDotEnv()
}

func (c *Config) applyDotEnv() error {
	if !setDefaultPtr(&c.DotEnv, false) {
		return nil
	}

	file := ".env"
	if c.file != "" {
		dir := filepath.Dir(c.file)
		file = filepath.Join(dir, file)
	}

	var expand func(name string) string
	var errs []error

	f, err := c.fs.Open(file)
	switch {
	case err == nil:
		defer func() { _ = f.Close() }()

		// Parse
		env, err := godotenv.Parse(f)
		if err != nil {
			return err
		}

		// And expand
		expand = func(name string) string {
			value, ok := env[name]
			if ok {
				return value
			}
			errs = append(errs, fmt.Errorf("%q is not defined", name))
			return fmt.Sprintf("#!MISSING(%q)", name)
		}

	case errors.Is(err, fs.ErrNotExist):
		// Only return an error if there is at least one ${ENV}
		expand = func(name string) string {
			if len(errs) == 0 {
				errs = append(errs, err)
			}
			return fmt.Sprintf("#!MISSING(%q)", name)
		}

	default:
		return err
	}

	expandEnv(reflect.ValueOf(c), expand)
	return errors.Join(errs...)
}

func (c *Config) Save() error {
	if c.file == "" {
		return errors.BadRequest.With("not loaded from a file")
	}
	if c.fs != os.DirFS(".") {
		return errors.BadRequest.With("loaded from an immutable filesystem")
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

	v = remap(v, camel2kebab, float2int)
	return format(v)
}

func remap(v any, mapKey func(string) string, mapValue func(reflect.Value) any) any {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice:
		u := make([]any, rv.Len())
		for i := range u {
			u[i] = remap(rv.Index(i).Interface(), mapKey, mapValue)
		}
		return u

	case reflect.Map:
		u := make(map[string]any, rv.Len())
		for it := rv.MapRange(); it.Next(); {
			u[mapKey(it.Key().String())] = remap(it.Value().Interface(), mapKey, mapValue)
		}
		return u

	default:
		if mapValue != nil {
			return mapValue(rv)
		}
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

func float2int(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		// If the float has no fractional part, convert it to an int
		v := v.Float()
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	default:
		return v.Interface()
	}
}

func expandEnv(v reflect.Value, expand func(string) string) {
	switch v.Kind() {
	case reflect.String:
		s := v.String()
		s = os.Expand(s, expand)
		v.SetString(s)

	case reflect.Pointer, reflect.Interface:
		expandEnv(v.Elem(), expand)

	case reflect.Slice, reflect.Array:
		for i, n := 0, v.Len(); i < n; i++ {
			expandEnv(v.Index(i), expand)
		}

	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			expandEnv(it.Key(), expand)
			expandEnv(it.Value(), expand)
		}

	case reflect.Struct:
		typ := v.Type()
		for i, n := 0, typ.NumField(); i < n; i++ {
			if typ.Field(i).IsExported() {
				expandEnv(v.Field(i), expand)
			}
		}
	}
}
