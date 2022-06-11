package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
	"gopkg.in/yaml.v3"
)

var flags struct {
	Package  string
	Out      string
	Language string
	Exclude  []string
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-record [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVarP(&flags.Language, "language", "l", "Go", "Output language or template file")
	cmd.Flags().StringVar(&flags.Package, "package", "", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "records_gen.go", "Output file")
	cmd.Flags().StringSliceVarP(&flags.Exclude, "exclude", "x", nil, "Exclude specific records")

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

func run(_ *cobra.Command, args []string) {
	data, err := ioutil.ReadFile(args[0])
	check(err)
	var raw interface{}
	check(yaml.Unmarshal(data, &raw))
	data, err = json.Marshal(raw)
	check(err)
	var records []*typegen.ContainerRecord
	check(json.Unmarshal(data, &records))
	for _, r := range records {
		containerize(r.Parts, r)
	}

	w := new(bytes.Buffer)
	check(Templates.Execute(w, flags.Language, struct {
		Package string
		Records []*typegen.ContainerRecord
	}{flags.Package, records}))
	check(typegen.WriteFile(flags.Out, w))
}

func containerize(r []typegen.Record, c *typegen.ContainerRecord) {
	for _, r := range r {
		switch r := r.(type) {
		case *typegen.ContainerRecord:
			r.Container = c
			containerize(r.Parts, r)
		case *typegen.ChainRecord:
			r.Container = c
		case *typegen.StateRecord:
			r.Container = c
		case *typegen.IndexRecord:
			r.Container = c
		case *typegen.OtherRecord:
			r.Container = c
		default:
			panic(fmt.Errorf("unknown record type %T", r))
		}
	}
}
