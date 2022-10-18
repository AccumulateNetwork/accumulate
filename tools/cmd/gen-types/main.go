// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

var flags struct {
	files typegen.FileReader

	Package        string
	SubPackage     string
	Out            string
	Language       string
	Reference      []string
	FilePerType    bool
	ExpandEmbedded bool
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-types [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVarP(&flags.Language, "language", "l", "Go", "Output language or template file")
	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVar(&flags.SubPackage, "subpackage", "", "Subpackage name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "types_gen.go", "Output file")
	cmd.Flags().StringSliceVar(&flags.Reference, "reference", nil, "Extra type definition files to use as a reference")
	cmd.Flags().BoolVar(&flags.FilePerType, "file-per-type", false, "Generate a separate file for each type")
	flags.files.SetFlags(cmd.Flags(), "types")

	_ = cmd.Execute()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

var moduleInfo = func() struct{ Dir string } {
	buf := new(bytes.Buffer)
	cmd := exec.Command("go", "list", "-m", "-json")
	cmd.Stdout = buf
	check(cmd.Run())

	info := new(struct{ Dir string })
	check(json.Unmarshal(buf.Bytes(), info))
	return *info
}()

func getWdPackagePath() string {
	wd, err := os.Getwd()
	check(err)
	return getPackagePath(wd)
}

func getPackagePath(dir string) string {
	rel, err := filepath.Rel(moduleInfo.Dir, dir)
	check(err)

	rel = strings.ReplaceAll(rel, "\\", "/")
	fmt.Printf("package %s\n", rel)
	return rel
}

func run(_ *cobra.Command, args []string) {
	switch flags.Language {
	case "java", "Java":
		flags.FilePerType = true
		flags.ExpandEmbedded = true
	}

	types := read(args, true)
	refTypes := read(nil, false)
	ttypes, err := convert(types, refTypes, flags.Package, flags.SubPackage, getWdPackagePath())
	check(err)

	if !flags.FilePerType {
		w := new(bytes.Buffer)
		check(Templates.Execute(w, flags.Language, ttypes))
		check(typegen.WriteFile(flags.Out, w))
	} else {
		fileTmpl, err := Templates.Parse(flags.Out, "filename", nil)
		checkf(err, "--out")

		w := new(bytes.Buffer)
		for _, typ := range ttypes.Types {
			w.Reset()
			err := fileTmpl.Execute(w, typ)
			check(err)
			filename := SafeClassName(w.String())

			w.Reset()
			err = Templates.Execute(w, flags.Language, &SingleTypeFile{flags.Package, typ})
			if errors.Is(err, typegen.ErrSkip) {
				continue
			}
			check(err)
			check(typegen.WriteFile(filename, w))
		}
		for _, typ := range ttypes.Unions {
			w.Reset()
			err := fileTmpl.Execute(w, typ)
			check(err)
			filename := w.String()

			w.Reset()
			err = Templates.Execute(w, flags.Language, &SingleUnionFile{flags.Package, typ})
			if errors.Is(err, typegen.ErrSkip) {
				continue
			}
			check(err)
			check(typegen.WriteFile(filename, w))
		}
	}
}

func read(files []string, main bool) typegen.Types {
	flup := map[*typegen.Type]string{}
	record := func(file string, typ *typegen.Type) {
		flup[typ] = file
	}

	var all map[string]*typegen.Type
	var err error
	if main {
		all, err = typegen.ReadMap(&flags.files, files, record)
	} else {
		all, err = typegen.ReadRaw(flags.Reference, record)
	}
	check(err)

	var v typegen.Types
	check(v.Unmap(all, flup))
	v.Sort()
	return v
}
