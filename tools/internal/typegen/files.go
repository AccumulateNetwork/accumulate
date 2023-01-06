// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

func GetModifiedDate(file string) (time.Time, error) {
	b, err := exec.Command("git", "log", "-1", "--pretty=format:%ci", file).Output()
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(b)))
}

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

func ReadRaw[V any](files []string, recordFile func(string, V)) (map[string]V, error) {
	all := map[string]V{}
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("opening %q: %v", file, err)
		}
		defer f.Close()

		var values map[string]V
		dec := yaml.NewDecoder(f)
		dec.KnownFields(true)
		err = dec.Decode(&values)
		if err != nil {
			return nil, fmt.Errorf("decoding %q: %v", file, err)
		}

		for k, v := range values {
			if _, ok := all[k]; ok {
				return nil, fmt.Errorf("duplicate entries for %s", k)
			}
			all[k] = v
			if recordFile != nil {
				recordFile(file, v)
			}
		}
	}
	return all, nil
}

func ReadMap[V any](f *FileReader, files []string, recordFile func(string, V)) (map[string]V, error) {
	all, err := ReadRaw(files, recordFile)
	if err != nil {
		return nil, err
	}

	all, err = mapInclude(f, all)
	if err != nil {
		return nil, err
	}

	err = mapExclude(f, all)
	if err != nil {
		return nil, err
	}

	err = mapRename(f, all)
	if err != nil {
		return nil, err
	}

	return all, nil
}

func mapInclude[V any](f *FileReader, all map[string]V) (map[string]V, error) {
	if f.Include == nil {
		return all, nil
	}

	included := map[string]V{}
	for _, k := range f.Include {
		if k = strings.TrimSpace(k); k == "" {
			continue
		}
		v, ok := all[k]
		if !ok {
			return all, fmt.Errorf("%s is not an entry", k)
		}
		included[k] = v
	}

	return included, nil
}

func mapExclude[V any](f *FileReader, all map[string]V) error {
	for _, k := range f.Exclude {
		if k = strings.TrimSpace(k); k == "" {
			continue
		}
		_, ok := all[k]
		if !ok {
			return fmt.Errorf("%s is not an entry", k)
		}
		delete(all, k)
	}
	return nil
}

func mapRename[V any](f *FileReader, all map[string]V) error {
	for _, spec := range f.Rename {
		bits := strings.Split(spec, ":")
		if len(bits) != 2 {
			return fmt.Errorf("invalid rename: want 'X:Y', got '%s'", spec)
		}

		from, to := bits[0], bits[1]
		v, ok := all[from]
		if !ok {
			return fmt.Errorf("%s is not an entry", from)
		}
		delete(all, from)
		all[to] = v
	}
	return nil
}
