package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var flags struct {
	Package  string
	Out      string
	Language string
	Include  []string
	Exclude  []string
	Rename   []string
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-types [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVarP(&flags.Language, "language", "l", "Go", "Output language or template file")
	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "types_gen.go", "Output file")
	cmd.Flags().StringSliceVarP(&flags.Include, "include", "i", nil, "Include only specific types")
	cmd.Flags().StringSliceVarP(&flags.Exclude, "exclude", "x", nil, "Exclude specific types")
	cmd.Flags().StringSliceVar(&flags.Rename, "rename", nil, "Rename types, e.g. 'Object:AccObject'")

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

func readTypes(files []string) typegen.DataTypes {
	allTypes := map[string]*typegen.DataType{}
	for _, file := range files {
		f, err := os.Open(file)
		checkf(err, "opening %q", file)
		defer f.Close()

		var types map[string]*typegen.DataType
		dec := yaml.NewDecoder(f)
		dec.KnownFields(true)
		checkf(dec.Decode(&types), "decoding %q", file)

		for name, typ := range types {
			if allTypes[name] != nil {
				fatalf("duplicate entries for %q", name)
			}
			allTypes[name] = typ
		}
	}

	if flags.Include != nil {
		included := map[string]*typegen.DataType{}
		for _, name := range flags.Include {
			typ, ok := allTypes[name]
			if !ok {
				fatalf("%q is not a type", name)
			}
			included[name] = typ
		}
		allTypes = included
	}

	for _, name := range flags.Exclude {
		_, ok := allTypes[name]
		if !ok {
			fatalf("%q is not a type", name)
		}
		delete(allTypes, name)
	}

	for _, spec := range flags.Rename {
		bits := strings.Split(spec, ":")
		if len(bits) != 2 {
			fatalf("invalid rename: want 'X:Y', got '%s'", spec)
		}

		from, to := bits[0], bits[1]
		typ, ok := allTypes[from]
		if !ok {
			fatalf("%q is not a type", from)
		}
		delete(allTypes, from)
		allTypes[to] = typ
	}

	return typegen.DataTypesFrom(allTypes)
}

func getPackagePath() string {
	buf := new(bytes.Buffer)
	cmd := exec.Command("go", "list", "-m", "-json")
	cmd.Stdout = buf
	check(cmd.Run())

	info := new(struct{ Dir string })
	check(json.Unmarshal(buf.Bytes(), info))

	wd, err := os.Getwd()
	check(err)

	rel, err := filepath.Rel(info.Dir, wd)
	check(err)

	rel = strings.ReplaceAll(rel, "\\", "/")
	fmt.Printf("package %s\n", rel)
	return rel
}

func run(_ *cobra.Command, args []string) {
	types := readTypes(args)
	ttypes, err := convert(types, flags.Package, getPackagePath())
	check(err)

	w := new(bytes.Buffer)
	check(Templates.Execute(w, flags.Language, ttypes))
	check(typegen.WriteFile(flags.Language, flags.Out, w))
}
