// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	Logger   string
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-model [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVarP(&flags.Language, "language", "l", "Go", "Output language or template file")
	cmd.Flags().StringVar(&flags.Package, "package", "", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "model_gen.go", "Output file")
	cmd.Flags().StringSliceVarP(&flags.Exclude, "exclude", "x", nil, "Exclude specific records")
	cmd.Flags().StringVar(&flags.Logger, "logger", "logging.OptionalLogger", "The logger that is used")

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
	data, err := os.ReadFile(args[0])
	check(err)
	var raw interface{}
	check(yaml.Unmarshal(data, &raw))
	data, err = json.Marshal(raw)
	check(err)
	var records []*typegen.EntityRecord
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	check(dec.Decode(&records))
	for _, r := range records {
		containerize(r.Attributes, r)
	}

	w := new(bytes.Buffer)
	check(Templates.Execute(w, flags.Language, struct {
		Package string
		Records []*typegen.EntityRecord
	}{flags.Package, records}))
	check(typegen.WriteFile(flags.Out, w))
}

func containerize(r []typegen.Record, c *typegen.EntityRecord) {
	for _, r := range r {
		switch r := r.(type) {
		case *typegen.EntityRecord:
			r.Parent = c
			containerize(r.Attributes, r)
		case *typegen.ChainRecord:
			r.Parent = c
		case *typegen.StateRecord:
			r.Parent = c
		case *typegen.IndexRecord:
			r.Parent = c
		case *typegen.OtherRecord:
			r.Parent = c
		default:
			panic(fmt.Errorf("unknown record type %T", r))
		}
	}
}
