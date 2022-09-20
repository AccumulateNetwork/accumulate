# `tools/cmd/gen-enum`

> `gen-enum [flags] [definition files]`

- `--language <name>` (default `Go`) - Generator language or specifies an
  external template file. Supported languages are Go and C.
- `--package <name>` (required) - Package name of the generated file.
- `--out <file>` (default `enums_gen.go`) - Output file.
- `--include <types>` - Only include specific types.
- `--exclude <types>` - Exclude types.
- `--rename <types>` - Rename types, e.g. `Foo:Bar`.

`gen-enum` is used to generate enumeration types. However Go does not have true
enumeration types so what the generator actually generates is named integer
types and typed constants plus utility methods.

The format of an enumeration definition file is:

```yaml
MyType:
  ValueOne:
    value: 1
    description: is the description of the value # optional
    aliases: [otherName] # optional
```

A definition file can define any number of types, and each type can define any
number of values. Each value must have a unique name and value. The example
above would generate `const MyTypeValueOne MyType = 1`. The Go template does not
generate the type delcaration, e.g. `type MyType int`.

- `value` is the underlying value.
- `description` (optional) is appended to the value's doc comment.
- `aliases` (optional) is a list of alternative names that will be recognized by
  the string-to-enum function.

The Go template generates following functions and methods:

- `MyTypeByName(name string) (MyType, bool)` returns the value for the given
  name.
- `(MyType).String() string` returns the name of the value.
- A JSON marshaller and unmarshaller that marshal the value as a string.
- A getter and setter used by binary marshalling to marshal the value as an integer.
