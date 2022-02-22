package typegen

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

type FileReader struct {
	Include []string
	Exclude []string
	Rename  []string
}

func (f *FileReader) SetFlags(flags *pflag.FlagSet, label string) {
	flags.StringSliceVarP(&f.Include, "include", "i", nil, "Include only specific "+label)
	flags.StringSliceVarP(&f.Exclude, "exclude", "x", nil, "Exclude specific "+label)
	flags.StringSliceVar(&f.Rename, "rename", nil, "Rename "+label+", e.g. 'Foo:Bar'")
}

func (f *FileReader) Read(files []string, typ reflect.Type) (interface{}, error) {
	if typ.Kind() != reflect.Map {
		panic("typ must be a map type")
	}
	if typ.Key().Kind() != reflect.String {
		panic("typ key must be string")
	}

	all := reflect.MakeMap(typ)
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("opening %q: %v", file, err)
		}
		defer f.Close()

		values := reflect.New(typ)
		dec := yaml.NewDecoder(f)
		dec.KnownFields(true)
		err = dec.Decode(values.Interface())
		if err != nil {
			return nil, fmt.Errorf("decoding %q: %v", file, err)
		}

		for it := values.Elem().MapRange(); it.Next(); {
			if all.MapIndex(it.Key()) != (reflect.Value{}) {
				return nil, fmt.Errorf("duplicate entries for %q", it.Key())
			}
			all.SetMapIndex(it.Key(), it.Value())
		}
	}

	all, err := f.include(all)
	if err != nil {
		return nil, err
	}

	err = f.exclude(all)
	if err != nil {
		return nil, err
	}

	err = f.rename(all)
	if err != nil {
		return nil, err
	}

	return all.Interface(), nil
}

func (f *FileReader) include(all reflect.Value) (reflect.Value, error) {
	if f.Include == nil {
		return all, nil
	}

	included := reflect.MakeMap(all.Type())
	for _, name := range f.Include {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		typ := all.MapIndex(reflect.ValueOf(name))
		if typ == (reflect.Value{}) {
			return all, fmt.Errorf("%q is not a type", name)
		}
		included.SetMapIndex(reflect.ValueOf(name), typ)
	}

	return included, nil
}

func (f *FileReader) exclude(all reflect.Value) error {
	for _, name := range f.Exclude {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		typ := all.MapIndex(reflect.ValueOf(name))
		if typ == (reflect.Value{}) {
			return fmt.Errorf("%q is not a type", name)
		}
		all.SetMapIndex(reflect.ValueOf(name), reflect.Value{})
	}
	return nil
}

func (f *FileReader) rename(all reflect.Value) error {
	for _, spec := range f.Rename {
		bits := strings.Split(spec, ":")
		if len(bits) != 2 {
			return fmt.Errorf("invalid rename: want 'X:Y', got '%s'", spec)
		}

		from, to := bits[0], bits[1]
		typ := all.MapIndex(reflect.ValueOf(from))
		if typ == (reflect.Value{}) {
			return fmt.Errorf("%q is not a type", from)
		}

		all.SetMapIndex(reflect.ValueOf(from), reflect.Value{})
		all.SetMapIndex(reflect.ValueOf(to), typ)
	}
	return nil
}
