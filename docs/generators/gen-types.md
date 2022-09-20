# `tools/cmd/gen-types`

> `gen-types [flags] [definition files]`

- `--language <name>` (default `Go`) - Generator language or specifies an
  external template file. Supported languages are Go and C.
- `--package <name>` (required) - Package name of the generated file.
- `--out <file>` (default `enums_gen.go`) - Output file.
- `--reference <files>` - Additional definition files for reference. Required
  for embedded fields.
- `--include <types>` - Only include specific types.
- `--exclude <types>` - Exclude types.
- `--rename <types>` - Rename types, e.g. `Foo:Bar`.

`gen-types` generates data types and binary un/marshallers.

The basic format of a data type definition is:

```yaml
MyType:
  fields:
  - name: FieldOne
    type: uint
```

A definition file can have multiple types, each of which can have multiple
fields. There are a few flags for data types that modify the generator's
behavior:

- `description` is added to the doc comment.
- `union` is discussed below.
- `non-binary` disables generation of binary a un/marshaller.
- `incomparable` disables generation of `Equal` and `Copy`.