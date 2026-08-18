// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
	"gopkg.in/yaml.v3"
)

// TestTypesRoundTripThroughTypegen writes a types CSV, converts it, and reads
// the result back the way gen-types does — into typegen.Type. What this tool
// emits has to be what the generator accepts, so the assertion is made with
// the generator's own types rather than by matching strings.
func TestTypesRoundTripThroughTypegen(t *testing.T) {
	const csv = `MyTable,,
name,type,description,optional
a,string,a string,false
b,bytes,raw bytes,false
c,uint,a number,false
d,bool,a flag,false
e,SomeOtherType,a reference,true
f,hash[],repeated hashes,false
`

	dir := t.TempDir()
	in := filepath.Join(dir, "types.csv")
	out := filepath.Join(dir, "types.yml")
	require.NoError(t, os.WriteFile(in, []byte(csv), 0600))

	require.NoError(t, processCSVTypes(in, out, "main"))

	b, err := os.ReadFile(out)
	require.NoError(t, err)

	var types map[string]*typegen.Type
	require.NoError(t, yaml.Unmarshal(b, &types))

	table := types["MyTable"]
	require.NotNil(t, table, "MyTable missing from output")
	require.Len(t, table.Fields, 6)

	byName := map[string]*typegen.Field{}
	for _, f := range table.Fields {
		byName[f.Name] = f
	}

	// Every primitive resolves to a known type code and marshals as itself.
	// Before this tool used typegen's lookup it recognised only string, url
	// and int, so bytes, uint, bool and hash were each written out as
	// `marshal-as: reference` — a reference to a type that does not exist.
	for _, name := range []string{"a", "b", "c", "d", "f"} {
		f := byName[name]
		require.NotNilf(t, f, "field %s missing", name)
		require.Truef(t, f.Type.IsKnown(), "field %s: %s should be a known type code", name, f.Type)
		require.Equalf(t, typegen.MarshalAsBasic, f.MarshalAs, "field %s must not be marked as a reference", name)
	}

	// An unrecognised name is a reference to another type.
	e := byName["e"]
	require.NotNil(t, e)
	require.False(t, e.Type.IsKnown())
	require.Equal(t, "SomeOtherType", e.Type.String())
	require.Equal(t, typegen.MarshalAsReference, e.MarshalAs)
	require.True(t, e.Optional)

	// A trailing [] becomes Repeatable, not part of the type name — typegen
	// rejects bracket notation outright.
	f := byName["f"]
	require.True(t, f.Repeatable)
	require.Equal(t, "hash", f.Type.String())
}

// TestTypesRejectsFieldWithoutTable guards the case that used to panic on a
// missing table name by indexing an absent map entry.
func TestTypesRejectsFieldWithoutTable(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "types.csv")
	require.NoError(t, os.WriteFile(in, []byte("a,string,a string,false\n"), 0600))

	err := processCSVTypes(in, filepath.Join(dir, "types.yml"), "main")
	require.ErrorContains(t, err, "before any table name")
}

// TestEnumsOutputIsDeterministic covers the ordering fix: ranging over the
// table map emitted tables in Go's randomised order, so the same CSV produced
// different files run to run.
func TestEnumsOutputIsDeterministic(t *testing.T) {
	const csv = `Zebra,,,
name,value,label,description
one,1,One,the first
Alpha,,,
name,value,label,description
two,2,Two,the second
Mango,,,
name,value,label,description
three,3,Three,the third
`

	dir := t.TempDir()
	in := filepath.Join(dir, "enums.csv")
	require.NoError(t, os.WriteFile(in, []byte(csv), 0600))

	var first []byte
	for i := 0; i < 8; i++ {
		out := filepath.Join(dir, "enums.yml")
		require.NoError(t, processCSVEnums(in, out, "main"))
		b, err := os.ReadFile(out)
		require.NoError(t, err)
		if i == 0 {
			first = b
			continue
		}
		require.Equal(t, string(first), string(b), "output differs between runs")
	}

	require.Regexp(t, `(?s)Alpha:.*Mango:.*Zebra:`, string(first), "tables should be sorted")
}
