package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
	"gopkg.in/yaml.v3"
)

var flags struct {
	Package    string
	SubPackage string
	Language   string
	Out        string
}

func run(_ *cobra.Command, args []string) {
	api := readFile(args[0])
	tapi := convert(api, flags.SubPackage)

	switch flags.Language {
	case "java", "Java":
		generateJava(tapi)
		break
	default:
		w := new(bytes.Buffer)
		check(Go.Execute(w, tapi))
		check(typegen.GoFmt(flags.Out, w))
	}
}

// FIXME is not finished but could make it work for what I need atm
func generateJava(tapi *TApi) {
	w := new(bytes.Buffer)
	dir, _ := filepath.Split(flags.Out)
	filename := strings.Replace(dir, "{{.SubPackage}}", flags.SubPackage, 1) + "/RPCMethod.java" // FIXME
	check(Templates.Execute(w, flags.Language, tapi))
	check(typegen.WriteFile(filename, w))
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

	return api
}

func main() {
	cmd := cobra.Command{
		Use:  "gen-api [file]",
		Args: cobra.ExactArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVarP(&flags.Language, "language", "l", "Go", "Output language or template file")
	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVar(&flags.SubPackage, "subpackage", "", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "api_gen.go", "Output file")

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
