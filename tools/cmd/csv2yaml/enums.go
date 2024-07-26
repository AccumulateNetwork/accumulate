package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v2"
)

// Enumeration represents a single enumeration entry
type Enumeration struct {
	Value       string   `yaml:"value"`
	Label       string   `yaml:"label,omitempty"`
	Description string   `yaml:"description"`
	Aliases     []string `yaml:"aliases,omitempty"`
}

// EnumEntry represents a single enumeration entry with its name
type EnumEntry struct {
	Name        string `yaml:"-"`
	Enumeration `yaml:",inline"`
}

// EnumTable represents a collection of enumerations
type EnumTable struct {
	Entries []EnumEntry `yaml:"-"`
}

// EnumTables represents a collection of enum tables
type EnumTables struct {
	Tables map[string]EnumTable
}

func processCSVEnums(csvFilePath, outFilePath, packageName string) error {
	csvFile, err := os.Open(csvFilePath)
	if err != nil {
		return err
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	reader.FieldsPerRecord = -1 // variable number of fields per record

	enumTables := EnumTables{Tables: make(map[string]EnumTable)}
	var currentEnumTableName string

	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for i := 0; i < len(records); i++ {
		row := records[i]
		if len(row) == 0 || row[0] == "" || strings.HasPrefix(row[0], "#") {
			continue
		}
		if row[1] == "" && row[2] == "" && row[3] == "" {
			currentEnumTableName = row[0]
			enumTables.Tables[currentEnumTableName] = EnumTable{}
			i++ // skip the next header row
		} else if len(row) > 3 && strings.TrimSpace(row[1]) != "" {
			enumeration := Enumeration{
				Value:       row[1], // Keep the value as a string to preserve hex values
				Description: row[3],
			}
			if len(row) > 4 && row[4] != "" {
				enumeration.Aliases = strings.Split(row[4], ",")
				for i := range enumeration.Aliases {
					enumeration.Aliases[i] = strings.TrimSpace(enumeration.Aliases[i])
				}
			}
			enumTable := enumTables.Tables[currentEnumTableName]
			entry := EnumEntry{
				Name:        capitalizeFirst(row[0]),
				Enumeration: enumeration,
			}
			// Only set the Label if it doesn't match the Name
			if entry.Name != row[2] {
				entry.Enumeration.Label = row[2]
			}
			enumTable.Entries = append(enumTable.Entries, entry)
			enumTables.Tables[currentEnumTableName] = enumTable
		}
	}

	yamlData, err := marshalEnumTables(enumTables)
	if err != nil {
		return err
	}

	yamlFilePath := strings.TrimSuffix(csvFilePath, filepath.Ext(csvFilePath)) + ".yml"
	if outFilePath == "" {
		outFilePath = yamlFilePath
	}

	if err := os.WriteFile(outFilePath, yamlData, 0644); err != nil {
		return err
	}

	fmt.Printf("YAML output saved to %s\n", outFilePath)
	return nil
}

func marshalEnumTables(enumTables EnumTables) ([]byte, error) {
	var builder strings.Builder

	for tableName, table := range enumTables.Tables {
		builder.WriteString(fmt.Sprintf("%s:\n", tableName))
		for _, entry := range table.Entries {
			builder.WriteString(fmt.Sprintf("  %s:\n", entry.Name))
			encodedEntry, err := yaml.Marshal(entry.Enumeration)
			if err != nil {
				return nil, err
			}
			lines := strings.Split(string(encodedEntry), "\n")
			for _, line := range lines {
				if line != "" {
					builder.WriteString(fmt.Sprintf("    %s\n", line))
				}
			}
		}
		builder.WriteString("\n") // Add a space after each table
	}

	return []byte(builder.String()), nil
}

func capitalizeFirst(s string) string {
	for i, v := range s {
		return string(unicode.ToUpper(v)) + s[i+1:]
	}
	return ""
}
