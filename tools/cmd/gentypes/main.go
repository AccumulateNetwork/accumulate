package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AccumulateNetwork/accumulate/tools/internal/typegen"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var flags struct {
	Package string
	Out     string
	IsState bool
}

func main() {
	cmd := cobra.Command{
		Use:  "gentypes [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "types_gen.go", "Output file")

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
	check(Go.Execute(w, ttypes))
	check(typegen.GoFmt(flags.Out, w))
}
