package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
	"gopkg.in/yaml.v3"
)

var flags struct {
	Package string
	Out     string
	Include []string
	Exclude []string
}

func run(_ *cobra.Command, args []string) {
	api := readFile(args[0])
	tapi := convert(api)

	w := new(bytes.Buffer)
	check(Go.Execute(w, tapi))
	check(typegen.GoFmt(flags.Out, w))
}

func readFile(file string) typegen.API {
	f, err := os.Open(file)
	check(err)
	defer f.Close()

	var api typegen.API
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	err = dec.Decode(&api)
	check(err)

	if flags.Include != nil {
		included := typegen.API{}
		for _, name := range flags.Include {
			typ, ok := api[name]
			if !ok {
				fatalf("%q is not a type", name)
			}
			included[name] = typ
		}
		api = included
	}

	for _, name := range flags.Exclude {
		_, ok := api[name]
		if !ok {
			fatalf("%q is not a type", name)
		}
		delete(api, name)
	}

	return api
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-sdk [file]",
		Args: cobra.ExactArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "sdk_gen.go", "Output file")
	cmd.Flags().StringSliceVarP(&flags.Include, "include", "i", nil, "Include only specific types")
	cmd.Flags().StringSliceVarP(&flags.Exclude, "exclude", "x", nil, "Exclude specific types")

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
