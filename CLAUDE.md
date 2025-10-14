# Claude Development Notes

This file contains important notes for Claude when working on the Accumulate codebase.

## Generated Files - DO NOT EDIT DIRECTLY

**IMPORTANT**: Files ending in `_gen.go` are automatically generated and should **NEVER** be edited directly.

### Generated File Patterns:
- `*_gen.go` - All files ending in `_gen.go`
- `types_gen.go` - Protocol type definitions
- `enums_gen.go` - Enumeration definitions  
- `unions_gen.go` - Union type definitions
- `schema_gen.go` - Schema definitions

### How to Make Changes:

1. **For Protocol Types** (`protocol/types_gen.go`):
   - Edit the corresponding YAML files in `protocol/`:
     - `general.yml` - General types like KeySpec, ADI, etc.
     - `accounts.yml` - Account type definitions
     - `operations.yml` - Operation definitions
     - `signatures.yml` - Signature type definitions
     - Other `.yml` files in the protocol directory
   - Run `go generate ./protocol` to regenerate types

2. **For Other Generated Files**:
   - Find the corresponding `.yml` file in the same directory
   - Edit the YAML file to make your changes
   - Run `go generate` in the appropriate directory

### Example Workflow:
```bash
# To add new fields to KeySpec:
1. Edit protocol/general.yml
2. Run: go generate ./protocol
3. Verify the changes in protocol/types_gen.go
4. Test your changes
5. Run: gosimports -w .
6. Commit both the .yml and _gen.go files
```

### Import Cycle Issues:
If you encounter import cycle errors in generated files:
- Remove problematic imports that cause cycles
- Focus on removing self-imports or internal package imports
- Keep essential imports for basic functionality

### Mining Fields Implementation:
The mining fields (`MiningDifficulty` and `MiningExpiry`) have been added to the KeySpec type in `protocol/general.yml` and are reflected in the generated `protocol/types_gen.go`.

## Pre-Commit Requirements

**CRITICAL**: Always run `gosimports -w .` before committing any changes. The CI pipeline requires proper import formatting and will fail without this step.

```bash
# Before every commit:
gosimports -w .
git add .
git commit -m "your commit message"
```

---

**Remember**: Always edit the source YAML files, never the generated `_gen.go` files directly!