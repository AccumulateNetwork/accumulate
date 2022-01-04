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
		Args: cobra.ExactArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "types_gen.go", "Output file")
	cmd.Flags().BoolVar(&flags.IsState, "is-state", false, "Is this the state package?")

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

func readTypes(file string) typegen.Types {
	f, err := os.Open(file)
	check(err)
	defer f.Close()

	var types map[string]*typegen.Type

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	err = dec.Decode(&types)
	check(err)

	return typegen.TypesFrom(types)
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
	types := readTypes(args[0])
	ttypes := convert(types, flags.Package, getPackagePath())

	w := new(bytes.Buffer)
	check(Go.Execute(w, ttypes))
	check(typegen.GoFmt(flags.Out, w))
}
