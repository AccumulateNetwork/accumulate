// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
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

	Package                string
	SubPackage             string
	Out                    string
	Language               string
	LanguageUnion          string
	Reference              []string
	FilePerType            bool
	ExpandEmbedded         bool
	LongUnionDiscriminator bool
	UnionSkipNew           bool
	UnionSkipType          bool
	ElidePackageType       bool
	GoInclude              []string
	Header                 string
	GenericSkipAs          bool
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-types [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVarP(&flags.Language, "language", "l", "Go", "Output language or template file")
	cmd.Flags().StringVarP(&flags.LanguageUnion, "language-union", "u", "go-union", "Output language or template file for a union")
	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVar(&flags.SubPackage, "subpackage", "", "Subpackage name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "types_gen.go", "Output file")
	cmd.Flags().StringSliceVar(&flags.Reference, "reference", nil, "Extra type definition files to use as a reference")
	cmd.Flags().BoolVar(&flags.FilePerType, "file-per-type", false, "Generate a separate file for each type")
	cmd.Flags().BoolVar(&flags.LongUnionDiscriminator, "long-union-discriminator", false, "Use the full name of the union type for the discriminator method")
	cmd.Flags().BoolVar(&flags.UnionSkipNew, "union-skip-new", false, "Don't generate a new func for unions")
	cmd.Flags().BoolVar(&flags.UnionSkipType, "union-skip-type", false, "Don't generate a type method for union members")
	cmd.Flags().BoolVar(&flags.ElidePackageType, "elide-package-type", false, "If there is a union type that has the same name as the package, elide it")
	cmd.Flags().BoolVar(&flags.GenericSkipAs, "skip-generic-as", false, "Do not create {Type}As methods for generic types")
	cmd.Flags().StringSliceVar(&flags.GoInclude, "go-include", nil, "Additional Go packages to include")
	cmd.Flags().StringVar(&flags.Header, "header", "", "Add a header to each file")
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

var moduleInfo = func() struct{ Dir, Path string } {
	buf := new(bytes.Buffer)
	cmd := exec.Command("go", "list", "-m", "-f={{.Dir}}\u2028{{.Path}}")
	cmd.Stdout = buf
	check(cmd.Run())

	cwd, err := os.Getwd()
	checkf(err, "get working directory")

	for _, s := range strings.Split(buf.String(), "\n") {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			continue
		}
		parts := strings.Split(s, "\u2028")
		r, err := filepath.Rel(parts[0], cwd)
		checkf(err, "check module path")
		if r == "." || !strings.HasPrefix(r, ".") {
			return struct{ Dir, Path string }{parts[0], parts[1]}
		}
	}

	fatalf("cannot find module")
	panic("not reached")
}()

func getPackagePath(dir string) string {
	rel, err := filepath.Rel(moduleInfo.Dir, dir)
	check(err)

	rel = strings.ReplaceAll(rel, "\\", "/")
	return rel
}

func run(_ *cobra.Command, args []string) {
	switch flags.Language {
	case "java", "Java", "c-source", "c-header":
		flags.FilePerType = true
		flags.ExpandEmbedded = true
		flags.LanguageUnion = flags.Language + "-union"
	}

	types := read(args, true)
	refTypes := read(nil, false)
	ttypes, err := convert(types, refTypes, flags.Package, flags.SubPackage)
	check(err)

	var missing []string
	for _, typ := range ttypes.Types {
		for _, field := range typ.Fields {
			if field.IsEmbedded && field.TypeRef == nil {
				missing = append(missing, fmt.Sprintf("%s (%s.%s)", field.Type.String(), typ.Name, field.Name))
			}
		}
	}
	if len(missing) > 0 {
		fatalf("missing type reference for %s", strings.Join(missing, ", "))
	}

	if !flags.FilePerType {
		w := new(bytes.Buffer)
		w.WriteString(flags.Header)
		check(Templates.Execute(w, flags.Language, ttypes))
		check(typegen.WriteFile(flags.Out, w))
	} else {
		fileTmpl, err := Templates.Parse(flags.Out, "filename", nil)
		checkf(err, "--out")

		w := new(bytes.Buffer)
		for _, typ := range ttypes.Types {
			w.Reset()
			w.WriteString(flags.Header)
			err := fileTmpl.Execute(w, typ)
			check(err)
			filename := SafeClassName(w.String())
			fmt.Printf("types filename: %s\n", filename)

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
			w.WriteString(flags.Header)
			typ2 := *typ
			typ2.Type = typ.Type + "_union"
			typ2.Name = typ.Name + "_union"
			err := fileTmpl.Execute(w, typ2)
			check(err)
			filename := SafeClassName(w.String())
			w.Reset()

			fmt.Printf("union filename: %s\n", filename)
			err = Templates.Execute(w, flags.LanguageUnion, &SingleUnionFile{flags.Package, typ})
			if errors.Is(err, typegen.ErrSkip) {
				continue
			}
			check(err)
			check(typegen.WriteFile(filename, w))
		}
	}
}

func read(files []string, main bool) typegen.Types {
	fileLookup := map[*typegen.Type]string{}
	record := func(file string, typ *typegen.Type) {
		fileLookup[typ] = file
	}

	var all map[string]*typegen.Type
	var err error
	if main {
		all, err = typegen.ReadMap(&flags.files, files, record)
	} else {
		all, err = typegen.ReadRaw(flags.Reference, record)
	}
	check(err)

	pkgLookup := map[string]string{}
	if main {
		pkgLookup["."] = PackagePath
	} else {
		wd, err := os.Getwd()
		check(err)

		for _, file := range fileLookup {
			dir := filepath.Dir(file)
			if pkgLookup[dir] != "" {
				continue
			}

			pkgLookup[dir] = getPackagePath(filepath.Join(wd, dir))
		}
	}

	var v typegen.Types
	check(v.Unmap(all, fileLookup, pkgLookup))
	v.Sort()
	return v
}
