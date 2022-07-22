package main

import (
	"bytes"
	"fmt"
	"os"
	"reflect"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
)

var flags struct {
	files typegen.FileReader

	Package     string
	SubPackage  string
	Language    string
	Out         string
	FilePerType bool
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-enum [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVarP(&flags.Language, "language", "l", "Go", "Output language or template file")
	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVar(&flags.SubPackage, "subpackage", "", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "enums_gen.go", "Output file")
	cmd.Flags().BoolVar(&flags.FilePerType, "file-per-type", false, "Generate a separate file for each type")
	flags.files.SetFlags(cmd.Flags(), "enums")

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

func run(_ *cobra.Command, args []string) {
	switch flags.Language {
	case "java", "Java":
		flags.FilePerType = true
	}

	types, err := flags.files.ReadMap(args, reflect.TypeOf((map[string]typegen.Enum)(nil)))
	check(err)
	ttypes := convert(types.(map[string]typegen.Enum), flags.Package, flags.SubPackage)

	if !flags.FilePerType {
		w := new(bytes.Buffer)
		check(Templates.Execute(w, flags.Language, ttypes))
		check(typegen.WriteFile(flags.Out, w))
	}

	fileTmpl, err := Templates.Parse(flags.Out, "filename", nil)
	checkf(err, "--out")

	w := new(bytes.Buffer)
	for _, typ := range ttypes.Types {
		w.Reset()
		err := fileTmpl.Execute(w, typ)
		check(err)
		filename := w.String()

		w.Reset()
		check(Templates.Execute(w, flags.Language, SingleTypeFile{flags.Package, typ}))
		check(typegen.WriteFile(filename, w))
	}
}
