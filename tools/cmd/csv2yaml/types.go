// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/typegen"
	"gopkg.in/yaml.v2"
)

// The YAML this tool writes is read back by gen-types as typegen.Type and
// typegen.Field, so every decision about a field is made with typegen's own
// vocabulary: typegen.TypeCodeByName resolves the type name exactly as
// gen-types will, and typegen.MarshalAs names the marshalling mode. A type
// this tool cannot account for is one gen-types would not have accepted, and
// it is reported here, against the CSV row, instead of surfacing later as a
// generator failure.
//
// The emitted shape stays compact rather than being typegen.Field marshalled
// directly: typegen's structs carry no omitempty, so marshalling them writes
// every zero-valued field and renders marshal-as as an integer. Field below is
// the wire form; typegen remains the authority on what goes into it.

// Field is the compact YAML form of a typegen.Field.
type Field struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
	MarshalAs   string `yaml:"marshal-as,omitempty"`
	Repeatable  bool   `yaml:"repeatable,omitempty"`
	Optional    bool   `yaml:"optional,omitempty"`
}

// Table represents a collection of fields
type Table struct {
	Fields []Field `yaml:"fields"`
}

func processCSVTypes(csvFilePath, outFilePath, packageName string) error {
	csvFile, err := os.Open(csvFilePath)
	if err != nil {
		return err
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	reader.FieldsPerRecord = -1 // variable number of fields per record

	tables := make(map[string]Table)
	var currentTableName string

	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i := 0; i < len(records); i++ {
		row := records[i]
		if len(row) == 0 || row[0] == "" || strings.HasPrefix(row[0], "#") {
			continue
		}
		if row[1] == "" && row[2] == "" {
			currentTableName = row[0]
			tables[currentTableName] = Table{}
			i++ // skip the next header row
			continue
		}
		if len(row) <= 1 || strings.TrimSpace(row[1]) == "" {
			continue
		}
		if currentTableName == "" {
			return fmt.Errorf("%s: field %q appears before any table name", csvFilePath, row[0])
		}

		field, err := parseField(row)
		if err != nil {
			return fmt.Errorf("%s: table %s: %w", csvFilePath, currentTableName, err)
		}

		table := tables[currentTableName]
		table.Fields = append(table.Fields, encodeField(field))
		tables[currentTableName] = table
	}

	yamlData, err := yaml.Marshal(tables)
	if err != nil {
		return err
	}

	yamlDataWithSpaces := addSpacesBetweenDefinitions(string(yamlData))

	if outFilePath == "" {
		outFilePath = strings.TrimSuffix(csvFilePath, filepath.Ext(csvFilePath)) + ".yml"
	}

	if err := os.WriteFile(outFilePath, []byte(yamlDataWithSpaces), 0644); err != nil {
		return err
	}

	fmt.Printf("YAML output saved to %s\n", outFilePath)
	return nil
}

// parseField builds a typegen.Field from one CSV row: name, type, description,
// optional.
//
// The type name resolves through typegen.TypeCodeByName — the same lookup
// gen-types performs when it reads the YAML back. A name it knows is a
// primitive and marshals as its own kind; a name it does not know is a
// reference to another type and is marked as such.
//
// The previous implementation hardcoded "string", "url" and "int" as the only
// non-reference types, so every other primitive — bytes, uint, bool, hash,
// bigint, duration, and the rest — was written out as `marshal-as: reference`,
// making gen-types generate code for a referenced type that does not exist.
func parseField(row []string) (*typegen.Field, error) {
	name := strings.TrimSpace(row[0])
	rawType := strings.TrimSpace(row[1])

	field := new(typegen.Field)
	field.Name = name
	if len(row) > 2 {
		field.Description = strings.TrimSpace(row[2])
	}
	if len(row) > 3 {
		field.Optional = parseOptional(row[3])
	}

	// A trailing [] means a repeated field. typegen rejects bracket notation
	// inside a type name, so it becomes the Repeatable flag instead.
	typeName := strings.TrimSuffix(rawType, "[]")
	field.Repeatable = strings.HasSuffix(rawType, "[]")
	if typeName == "" {
		return nil, fmt.Errorf("field %q has no type", name)
	}

	if code, ok := typegen.TypeCodeByName(typeName); ok {
		field.Type.SetKnown(code)
	} else {
		field.Type.SetNamed(typeName)
		field.MarshalAs = typegen.MarshalAsReference
	}

	return field, nil
}

// encodeField converts a typegen.Field into the compact YAML form. Both the
// type name and the marshal-as name come from typegen, so they cannot drift
// from what gen-types parses.
func encodeField(f *typegen.Field) Field {
	out := Field{
		Name:        f.Name,
		Type:        f.Type.String(),
		Description: f.Description,
		Repeatable:  f.Repeatable,
		Optional:    f.Optional,
	}
	if f.MarshalAs != typegen.MarshalAsBasic {
		out.MarshalAs = f.MarshalAs.String()
	}
	return out
}

func parseOptional(value string) bool {
	return strings.TrimSpace(value) == "true"
}

func addSpacesBetweenDefinitions(yamlData string) string {
	lines := strings.Split(yamlData, "\n")
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(result) > 0 && !strings.HasPrefix(line, "  ") {
			result = append(result, "")
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}
