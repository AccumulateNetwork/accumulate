package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/AccumulateNetwork/accumulate/tools/internal/typegen"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var flags struct {
	Package  string
	Language string
	Out      string
	IsState  bool
}

func main() {
	cmd := cobra.Command{
		Use:  "gentypes [file]",
		Args: cobra.MinimumNArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVar(&flags.Language, "language", "go", "go or c")
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
	buf := new(bytes.Buffer)
	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		check(err)
		buf.Write(data)
		buf.WriteRune('\n')
	}

	var types map[string]*typegen.DataType

	dec := yaml.NewDecoder(buf)
	dec.KnownFields(true)
	check(dec.Decode(&types))

	return typegen.DataTypesFrom(types)
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

func cleanupWhitespace(s string) string {
	sa := strings.FieldsFunc(s, func(c rune) bool { return '\n' == c })
	var cw bytes.Buffer
	for _, s := range sa {
		s = strings.TrimRightFunc(s, func(c rune) bool { return unicode.IsSpace(c) })
		if len(s) > 0 {
			if strings.HasPrefix(s, "///") {
				cw.WriteByte('\n')
			}
			cw.WriteString(s)
			cw.WriteByte('\n')
		}
	}
	return cw.String()
}

func run(_ *cobra.Command, args []string) {
	types := readTypes(args)
	ttypes := convert(types, flags.Package, getPackagePath())

	switch flags.Language {
	case "go":
		w := new(bytes.Buffer)
		check(Go.Execute(w, ttypes))
		check(typegen.GoFmt(flags.Out, w))
	case "c":
		cw := new(bytes.Buffer)
		check(CH.Execute(cw, ttypes))
		cc := new(bytes.Buffer)
		check(C.Execute(cc, ttypes))

		ext := filepath.Ext(flags.Out)
		basepath := flags.Out
		if ext == ".h" || ext == ".c" {
			basepath = strings.TrimSuffix(basepath, ext)
		}
		header := basepath + ".h"
		source := basepath + ".c"

		sh := cleanupWhitespace(cw.String())
		sc := cleanupWhitespace(cc.String())

		f, err := os.Create(header)
		check(err)
		f.WriteString(sh)
		f.Close()

		f, err = os.Create(source)
		check(err)
		fmt.Fprintf(f, "#include \"%s\"\n", header)
		f.WriteString(sc)
		f.Close()

	default:
		fmt.Printf("Unsupported language %s", flags.Language)
	}

}
