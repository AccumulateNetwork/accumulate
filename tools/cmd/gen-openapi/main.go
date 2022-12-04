package main

import (
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/jsonx"
	"golang.org/x/tools/go/packages"
)

func main() {
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

var cmd = &cobra.Command{
	Use:  "gen-openapi [types...]",
	Args: cobra.ExactArgs(1),
	Run:  run,
}

var flag = struct {
	JsonRpc bool
}{}

func init() {
	cmd.Flags().BoolVar(&flag.JsonRpc, "json-rpc", false, "Generate for JSON-RPC")
}

func run(_ *cobra.Command, args []string) {
	pkgs, err := packages.Load(&packages.Config{
		Mode:       packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo,
		BuildFlags: []string{"-tags=spec"},
	})
	check(err)
	if len(pkgs) != 1 {
		fatalf("expected one package, got %d", len(pkgs))
	}

	pkg := pkgs[0]
	for _, e := range pkg.Errors {
		fmt.Fprintf(os.Stderr, "Error: %v\n", e)
	}
	if len(pkg.Errors) > 0 {
		os.Exit(1)
	}

	obj := pkg.Types.Scope().Lookup(args[0])
	if obj == nil {
		fatalf("%s not found", args[0])
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		fatalf("%s is not a named type", args[0])
	}
	bld := Builder{}
	schema := bld.jsonRpcSchema(named)

	schema.Definitions = make(map[string]*jsonx.Schema, len(bld))
	for typ, s := range bld {
		name := fmt.Sprintf("%s-%s", typ.Obj().Pkg().Name(), typ.Obj().Name())
		if other, ok := schema.Definitions[name]; ok {
			fatalf("duplicate type names: %s and %s", other.Title, s.Title)
		}
		schema.Definitions[name] = s
	}

	b, err := json.Marshal(schema)
	check(err)
	fmt.Println(string(b))
}

type Builder map[*types.Named]*jsonx.Schema

var reCamel = regexp.MustCompile("[a-z]?[A-Z]+")

func (b Builder) jsonRpcSchema(typ *types.Named) *jsonx.Schema {
	in := new(jsonx.Schema)

	for i, n := 0, typ.NumMethods(); i < n; i++ {
		meth := typ.Method(i)
		sig := meth.Type().(*types.Signature)
		if sig.Params().Len() != 1 {
			fatalf("%s.%s has %d parameters, expected one", typ.Obj().Name(), meth.Name(), sig.Params().Len())
		}
		if sig.Results().Len() != 1 {
			fatalf("%s.%s has %d results, expected one", typ.Obj().Name(), meth.Name(), sig.Results().Len())
		}

		name := reCamel.ReplaceAllStringFunc(meth.Name(), func(s string) string {
			if !unicode.IsLower(rune(s[0])) {
				return strings.ToLower(s)
			}
			return fmt.Sprintf("%c-%s", s[0], strings.ToLower(s[1:]))
		})

		in.OneOf = append(in.OneOf, &jsonx.Schema{
			Type:          jsonx.Schema_Type{jsonx.SimpleTypes_Object},
			Discriminator: "method",
			Properties: map[string]*jsonx.Schema{
				"jsonrpc": {Type: jsonx.Schema_Type{jsonx.SimpleTypes_String}},
				"id":      {Type: jsonx.Schema_Type{jsonx.SimpleTypes_Number}},
				"method":  {Type: jsonx.Schema_Type{jsonx.SimpleTypes_String}, Enum: []string{name}},
				"params":  b.schemaForType(sig.Params().At(0).Type()),
			},
		})
	}

	return in
}

func (b Builder) schemaForType(typ types.Type) (schema *jsonx.Schema) {
	schema = new(jsonx.Schema)
	switch typ.String() {
	case "time.Duration":
		schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_Integer}
		return schema
	case "math/big.Int", "*math/big.Int",
		"time.Time", "*time.Time",
		"[]byte", "[32]byte",
		"gitlab.com/accumulatenetwork/accumulate/pkg/url.URL",
		"*gitlab.com/accumulatenetwork/accumulate/pkg/url.URL",
		"gitlab.com/accumulatenetwork/accumulate/pkg/url.TxID",
		"*gitlab.com/accumulatenetwork/accumulate/pkg/url.TxID":
		schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_String}
		return schema
	case "interface{}", "encoding/json.RawMessage":
		schema.Type = jsonx.Schema_Type{
			jsonx.SimpleTypes_Array,
			jsonx.SimpleTypes_Boolean,
			jsonx.SimpleTypes_Integer,
			jsonx.SimpleTypes_Null,
			jsonx.SimpleTypes_Number,
			jsonx.SimpleTypes_Object,
			jsonx.SimpleTypes_String,
		}
		return schema
	}

	named, ok := typ.(*types.Named)
	if ok {
		// Un-instantiate
		named = named.Origin()

		ref := &jsonx.Schema{Ref: "#/definitions/" + fmt.Sprintf("%s-%s", named.Obj().Pkg().Name(), named.Obj().Name())}
		_, ok := b[named]
		if ok {
			return ref
		}

		b[named] = schema
		schema.Title = named.String()
		defer func() { schema = ref }()
	}

	if param, ok := typ.(*types.TypeParam); ok {
		return b.schemaForType(param.Constraint())
	}

	switch utyp := typ.Underlying().(type) {
	case *types.Basic:
		switch utyp.Kind() {
		case types.UntypedBool, types.Bool:
			schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_Boolean}
		case types.UntypedInt,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
			schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_Integer}
		case types.UntypedFloat, types.Float32, types.Float64:
			schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_Number}
		case types.UntypedString, types.String:
			schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_String}
		case types.UntypedNil:
			schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_Null}
		default:
			fatalf("unsupported basic type %v", utyp.Kind())
		}

	case *types.Pointer:
		return b.schemaForType(utyp.Elem())

	case *types.Slice:
		schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_Array}
		schema.Items = b.schemaForType(utyp.Elem())

	case *types.Struct:
		schema.Type = jsonx.Schema_Type{jsonx.SimpleTypes_Object}
		for i, n := 0, utyp.NumFields(); i < n; i++ {
			b.schemaForField(schema, utyp, i)
		}

	case *types.Interface:
		if named == nil {
			fatalf("unnamed interfaces are not supported: %v", typ)
		}

		schema.Discriminator = "type"

		pkg := named.Obj().Pkg().Scope()
		for _, name := range pkg.Names() {
			// Exclude synthetic transactions
			if named.Obj().Name() == "TransactionBody" && strings.HasPrefix(name, "Synthetic") {
				continue
			}

			typ, ok := pkg.Lookup(name).Type().(*types.Named)
			if !ok {
				continue
			}
			if _, ok := typ.Underlying().(*types.Interface); ok {
				continue
			}
			if !types.Implements(typ, utyp) && !types.Implements(types.NewPointer(typ), utyp) {
				continue
			}
			schema.OneOf = append(schema.OneOf, b.schemaForType(typ))
		}

	default:
		println(typ.String())
	}
	return schema
}

func (b Builder) schemaForField(schema *jsonx.Schema, typ *types.Struct, i int) {
	field := typ.Field(i)
	if !field.Exported() {
		return
	}

	if schema.Properties == nil {
		schema.Properties = map[string]*jsonx.Schema{}
	}

	if field.Anonymous() {
		s := b.schemaForType(field.Type())
		if named, ok := field.Type().(*types.Named); ok {
			s = b[named]
		}
		for name, prop := range s.Properties {
			schema.Properties[name] = prop
		}
		return
	}

	tag := strings.Split(reflect.StructTag(typ.Tag(i)).Get("json"), ",")
	if tag[0] == "-" {
		return
	}

	name := tag[0]
	if name == "" {
		name = field.Name()
	}

	schema.Properties[name] = b.schemaForType(field.Type())
}
