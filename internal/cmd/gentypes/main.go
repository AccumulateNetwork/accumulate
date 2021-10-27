package main

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Record struct {
	name      string
	Kind      string
	NonBinary bool `yaml:"non-binary"`
	Fields    []*Field
}

type Field struct {
	Name      string
	Type      string
	MarshalAs string `yaml:"marshal-as"`
	Slice     *Field
	Pointer   bool
	Optional  bool
	IsUrl     bool `yaml:"is-url"`
}

var flags struct {
	Package string
	Out     string
}

func main() {
	cmd := cobra.Command{
		Use:  "gentypes [file]",
		Args: cobra.ExactArgs(1),
		Run:  run,
	}

	cmd.Flags().StringVar(&flags.Package, "package", "protocol", "Package name")
	cmd.Flags().StringVarP(&flags.Out, "out", "o", "types_gen.go", "Output file")

	_ = cmd.Execute()
}

func check(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func readTypes(file string) []*Record {
	f, err := os.Open(file)
	check(err)
	defer f.Close()

	var v map[string]*Record
	err = yaml.NewDecoder(f).Decode(&v)
	check(err)

	r := make([]*Record, 0, len(v))
	for name, rec := range v {
		rec.name = name
		r = append(r, rec)
	}

	sort.Slice(r, func(i, j int) bool {
		return r[i].name < r[j].name
	})

	return r
}

func resolveType(field *Field, forNew bool) string {
	switch field.Type {
	case "bytes":
		return "[]byte"
	case "bigint":
		return "big.Int"
	case "uvarint":
		return "uint64"
	case "chain":
		return "[32]byte"
	case "chainSet":
		return "[][32]byte"
	case "duration":
		return "time.Duration"
	case "any":
		return "interface{}"
	case "slice":
		return "[]" + resolveType(field.Slice, false)
	}

	typ := field.Type
	if field.Pointer && !forNew {
		typ = "*" + typ
	}
	return typ
}

func jsonType(field *Field) string {
	switch field.Type {
	case "bytes", "chain":
		return "string"
	case "chainSet":
		return "[]string"
	case "duration":
		return "interface{}"
	case "slice":
		jt := jsonType(field.Slice)
		if jt != "" {
			return "[]" + jt
		}
	}
	return ""
}

func fieldError(op, name string, args ...string) string {
	args = append(args, "err")
	return fmt.Sprintf("fmt.Errorf(\"error %s %s: %%w\", %s)", op, name, strings.Join(args, ","))
}

func binarySize(w *bytes.Buffer, field *Field, varName string) {
	var expr string
	switch field.Type {
	case "bytes", "string", "chainSet", "uvarint", "duration":
		expr = field.Type + "BinarySize(%s)"
	case "bigint", "chain":
		expr = field.Type + "BinarySize(&%s)"
	case "slice":
		expr = "uvarintBinarySize(uint64(len(%s)))"
	default:
		if field.MarshalAs != "self" {
			panic(fmt.Errorf("cannot determine how to marshal %s", resolveType(field, false)))
		}
		expr = "%s.BinarySize()"
	}

	expr = fmt.Sprintf(expr, varName)
	fmt.Fprintf(w, "\tn += %s\n\n", expr)

	if field.Type != "slice" {
		fmt.Fprintf(w, "\n")
		return
	}

	fmt.Fprintf(w, "\tfor _, v := range %s {\n", varName)
	binarySize(w, field.Slice, "v")
	fmt.Fprintf(w, "\t}\n\n")
}

func binaryMarshalValue(w *bytes.Buffer, field *Field, varName, errName string, errArgs ...string) {
	var expr string
	var canErr bool
	switch field.Type {
	case "bytes", "string", "chainSet", "uvarint", "duration":
		expr, canErr = field.Type+"MarshalBinary(%s)", false
	case "bigint", "chain":
		expr, canErr = field.Type+"MarshalBinary(&%s)", false
	case "slice":
		expr, canErr = "uvarintMarshalBinary(uint64(len(%s)))", false
	default:
		if field.MarshalAs != "self" {
			panic(fmt.Errorf("cannot determine how to marshal %s", resolveType(field, false)))
		}
		expr, canErr = "%s.MarshalBinary()", true
	}

	expr = fmt.Sprintf(expr, varName)
	if canErr {
		err := fieldError("encoding", errName, errArgs...)
		fmt.Fprintf(w, "\tif b, err := %s; err != nil { return nil, %s } else { buffer.Write(b) }\n", expr, err)
	} else {
		fmt.Fprintf(w, "\tbuffer.Write(%s)\n", expr)
	}

	if field.Type != "slice" {
		fmt.Fprintf(w, "\n")
		return
	}

	fmt.Fprintf(w, "\tfor i, v := range %s {\n", varName)
	fmt.Fprintf(w, "\t\t_ = i\n")
	binaryMarshalValue(w, field.Slice, "v", errName+"[%d]", "i")
	fmt.Fprintf(w, "\t}\n\n")
}

func binaryUnmarshalValue(w *bytes.Buffer, field *Field, varName, errName string, errArgs ...string) {
	var expr, size, sliceName string
	var inPlace bool
	switch field.Type {
	case "bytes", "string", "chainSet", "uvarint", "duration":
		expr, size, inPlace = field.Type+"UnmarshalBinary(data)", field.Type+"BinarySize(%s)", false
	case "bigint", "chain":
		expr, size, inPlace = field.Type+"UnmarshalBinary(data)", field.Type+"BinarySize(&%s)", false
	case "slice":
		sliceName, varName = varName, "len"+field.Name
		fmt.Fprintf(w, "var %s uint64\n", varName)
		expr, size, inPlace = "uvarintUnmarshalBinary(data)", "uvarintBinarySize(%s)", false
	default:
		if field.MarshalAs != "self" {
			panic(fmt.Errorf("cannot determine how to marshal %s", resolveType(field, false)))
		}
		expr, size, inPlace = "%s.UnmarshalBinary(data)", "%s.BinarySize()", true
	}

	size = fmt.Sprintf(size, varName)
	err := fieldError("decoding", errName, errArgs...)
	if inPlace {
		expr = fmt.Sprintf(expr, varName)
		fmt.Fprintf(w, "\tif err := %s; err != nil { return %s }\n", expr, err)
	} else if field.Type == "bigint" {
		fmt.Fprintf(w, "\tif x, err := %s; err != nil { return %s } else { %s.Set(x) }\n", expr, err, varName)
	} else {
		fmt.Fprintf(w, "\tif x, err := %s; err != nil { return %s } else { %s = x }\n", expr, err, varName)
	}
	fmt.Fprintf(w, "\tdata = data[%s:]\n\n", size)

	if field.Type != "slice" {
		return
	}

	fmt.Fprintf(w, "\t%s = make(%s, %s)\n", sliceName, resolveType(field, false), varName)
	fmt.Fprintf(w, "\tfor i := range %s {\n", sliceName)
	if field.Slice.Pointer {
		fmt.Fprintf(w, "\t\tx := new(%s)\n", resolveType(field.Slice, true))
		binaryUnmarshalValue(w, field.Slice, "x", errName+"[%d]", "i")
		fmt.Fprintf(w, "\t\t%s[i] = x", sliceName)
	} else {
		binaryUnmarshalValue(w, field.Slice, sliceName+"[i]", errName+"[%d]", "i")
	}
	fmt.Fprintf(w, "\t}\n\n")
}

func jsonVar(w *bytes.Buffer, typ *Record, varName string) {
	fmt.Fprintf(w, "\tvar %s struct{\n", varName)
	if typ.Kind == "chain" {
		fmt.Fprintf(w, "\t\tstate.ChainHeader\n")
	}
	for _, f := range typ.Fields {
		lcName := strings.ToLower(f.Name[:1]) + f.Name[1:]
		typ := jsonType(f)
		if typ == "" {
			typ = resolveType(f, false)
		}
		fmt.Fprintf(w, "\t\t%s %s `json:\"%s\"`\n", f.Name, typ, lcName)
	}
	fmt.Fprintf(w, "\t}\n")
}

func valueToJson(w *bytes.Buffer, field *Field, tgtName, srcName string) {
	switch field.Type {
	case "bytes", "chain", "chainSet", "duration":
		fmt.Fprintf(w, "\t%s = %sToJSON(%s)\n", tgtName, field.Type, srcName)
		return

	case "slice":
		if jsonType(field.Slice) == "" {
			break
		}

		fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, jsonType(field.Slice), srcName)
		fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		valueToJson(w, field.Slice, tgtName+"[i]", "x")
		fmt.Fprintf(w, "\t}\n")
		return
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s\n", tgtName, srcName)
}

func valueFromJson(w *bytes.Buffer, field *Field, tgtName, srcName, errName string, errArgs ...string) {
	err := fieldError("decoding", errName, errArgs...)
	switch field.Type {
	case "bytes", "chain", "chainSet", "duration":
		fmt.Fprintf(w, "\tif x, err := %sFromJSON(%s); err != nil {\n\t\treturn %s\n\t} else {\n\t\t%s = x\n\t}\n", field.Type, srcName, err, tgtName)
		return

	case "slice":
		if jsonType(field.Slice) == "" {
			break
		}

		fmt.Fprintf(w, "\t%s = make([]%s, len(%s))\n", tgtName, resolveType(field.Slice, false), srcName)
		fmt.Fprintf(w, "\tfor i, x := range %s {\n", srcName)
		valueFromJson(w, field.Slice, tgtName+"[i]", "x", errName+"[%d]", "i")
		fmt.Fprintf(w, "\t}\n")
		return
	}

	// default:
	fmt.Fprintf(w, "\t%s = %s\n", tgtName, srcName)
}

func needsCustomJSON(typ *Record) bool {
	for _, f := range typ.Fields {
		if jsonType(f) != "" {
			return true
		}
	}
	return false
}

func run(_ *cobra.Command, args []string) {
	w := new(bytes.Buffer)
	fmt.Fprintf(w, "package %s\n\n", flags.Package)
	fmt.Fprintf(w, "// GENERATED BY go run ./internal/cmd/genmarshal. DO NOT EDIT.\n\n")
	fmt.Fprintf(w, `import (
		"bytes"
		"encoding/json"
		"fmt"
		"math/big"
		"time"

		"github.com/AccumulateNetwork/accumulated/types"
		"github.com/AccumulateNetwork/accumulated/types/state"
	)`+"\n\n")

	types := readTypes(args[0])

	for _, typ := range types {
		fmt.Fprintf(w, "type %s struct {\n", typ.name)
		if typ.Kind == "chain" {
			fmt.Fprintf(w, "\nstate.ChainHeader\n")
		}
		for _, field := range typ.Fields {
			lcName := strings.ToLower(field.Name[:1]) + field.Name[1:]
			fmt.Fprintf(w, "\t%s %s `", field.Name, resolveType(field, false))
			fmt.Fprintf(w, `json:"%[1]s" form:"%[1]s" query:"%[1]s"`, lcName)

			var validate []string
			if !field.Optional {
				validate = append(validate, "required")
			}
			if field.IsUrl {
				validate = append(validate, "acc-url")
			}
			if len(validate) > 0 {
				fmt.Fprintf(w, ` validate:"%s"`, strings.Join(validate, ","))
			}

			fmt.Fprint(w, "`\n")
		}
		fmt.Fprintf(w, "}\n\n")
	}

	for _, typ := range types {
		if typ.Kind != "chain" {
			continue
		}
		fmt.Fprintf(w, `func New%s() *%[1]s {
			v := new(%[1]s)
			v.Type = types.ChainType%[1]s
			return v
		}`+"\n\n", typ.name)
	}

	for _, typ := range types {
		if typ.Kind != "tx" {
			continue
		}
		fmt.Fprintf(w, "func (*%s) GetType() types.TxType { return types.TxType%[1]s }\n\n", typ.name)
	}

	for _, typ := range types {
		if typ.NonBinary {
			continue
		}

		fmt.Fprintf(w, "func (v *%s) BinarySize() int {\n", typ.name)
		fmt.Fprintf(w, "\tvar n int\n\n")

		switch typ.Kind {
		case "tx":
			fmt.Fprintf(w, "\nn += uvarintBinarySize(uint64(types.TxType%s))\n\n", typ.name)
		case "chain":
			fmt.Fprintf(w, "\t// Enforce sanity\n\tv.Type = types.ChainType%s\n", typ.name)
			fmt.Fprintf(w, "\nn += v.ChainHeader.GetHeaderSize()\n\n")
		}

		for _, field := range typ.Fields {
			binarySize(w, field, "v."+field.Name)
		}

		fmt.Fprintf(w, "\n\treturn n\n}\n\n")
	}

	for _, typ := range types {
		if typ.NonBinary {
			continue
		}

		fmt.Fprintf(w, "func (v *%s) MarshalBinary() ([]byte, error) {\n", typ.name)
		fmt.Fprintf(w, "\tvar buffer bytes.Buffer\n\n")

		switch typ.Kind {
		case "tx":
			fmt.Fprintf(w, "\tbuffer.Write(uvarintMarshalBinary(uint64(types.TxType%s)))\n\n", typ.name)
		case "chain":
			fmt.Fprintf(w, "\t// Enforce sanity\n\tv.Type = types.ChainType%s\n\n", typ.name)
			err := fieldError("encoding", "header")
			fmt.Fprintf(w, "\tif b, err := v.ChainHeader.MarshalBinary(); err != nil { return nil, %s } else { buffer.Write(b) }\n", err)
		}

		for _, field := range typ.Fields {
			binaryMarshalValue(w, field, "v."+field.Name, field.Name)
		}

		fmt.Fprintf(w, "\n\treturn buffer.Bytes(), nil\n}\n\n")
	}

	for _, typ := range types {
		if typ.NonBinary {
			continue
		}

		fmt.Fprintf(w, "func (v *%s) UnmarshalBinary(data []byte) error {\n", typ.name)

		switch typ.Kind {
		case "tx":
			err := fieldError("decoding", "TX type")
			fmt.Fprintf(w, "\ttyp := types.TxType%s\n", typ.name)
			fmt.Fprintf(w, "\tif v, err := uvarintUnmarshalBinary(data); err != nil { return %s } else if v != uint64(typ) { return fmt.Errorf(\"invalid TX type: want %%v, got %%v\", typ, types.TxType(v)) }\n", err)
			fmt.Fprintf(w, "\tdata = data[uvarintBinarySize(uint64(typ)):]\n\n")

		case "chain":
			err := fieldError("decoding", "header")
			fmt.Fprintf(w, "\ttyp := types.ChainType%s\n", typ.name)
			fmt.Fprintf(w, "\tif err := v.ChainHeader.UnmarshalBinary(data); err != nil { return %s } else if v.Type != typ { return fmt.Errorf(\"invalid chain type: want %%v, got %%v\", typ, v.Type) }\n", err)
			fmt.Fprintf(w, "\tdata = data[v.GetHeaderSize():]\n\n")
		}

		for _, field := range typ.Fields {
			binaryUnmarshalValue(w, field, "v."+field.Name, field.Name)
		}

		fmt.Fprintf(w, "\n\treturn nil\n}\n\n")
	}

	for _, typ := range types {
		if !needsCustomJSON(typ) {
			continue
		}

		fmt.Fprintf(w, "func (v *%s) MarshalJSON() ([]byte, error) {\n", typ.name)
		jsonVar(w, typ, "u")

		if typ.Kind == "chain" {
			fmt.Fprintf(w, "\tu.ChainHeader = v.ChainHeader\n")
		}
		for _, f := range typ.Fields {
			valueToJson(w, f, "u."+f.Name, "v."+f.Name)
		}

		fmt.Fprintf(w, "\treturn json.Marshal(u)\t")
		fmt.Fprintf(w, "}\n\n")
	}

	for _, typ := range types {
		if !needsCustomJSON(typ) {
			continue
		}

		fmt.Fprintf(w, "func (v *%s) UnmarshalJSON(data []byte) error {\t", typ.name)
		jsonVar(w, typ, "u")
		fmt.Fprintf(w, "\tif err := json.Unmarshal(data, &u); err != nil {\n\t\treturn err\n\t}\n")

		if typ.Kind == "chain" {
			fmt.Fprintf(w, "\tv.ChainHeader = u.ChainHeader\n")
		}
		for _, f := range typ.Fields {
			valueFromJson(w, f, "v."+f.Name, "u."+f.Name, f.Name)
		}

		fmt.Fprintf(w, "\treturn nil\t")
		fmt.Fprintf(w, "}\n\n")
	}

	f, err := os.Create(flags.Out)
	check(err)
	defer f.Close()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, flags.Out, w, parser.ParseComments)
	if err != nil {
		// If parsing fails, write out the unformatted code. Without this,
		// debugging the generator is a pain.
		_, _ = w.WriteTo(f)
	}
	check(err)

	err = format.Node(f, fset, file)
	check(err)
}
