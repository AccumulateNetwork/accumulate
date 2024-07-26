package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// Field represents a single field in a table
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
		} else if len(row) > 1 && strings.TrimSpace(row[1]) != "" {
			fieldType := row[1]
			field := Field{
				Name:        row[0],
				Type:        strings.TrimSuffix(fieldType, "[]"),
				Description: row[2],
				Optional:    parseOptional(row[3]),
			}
			if field.Type != "string" && field.Type != "url" && field.Type != "int" {
				field.MarshalAs = "reference"
			}
			if strings.HasSuffix(fieldType, "[]") {
				field.Repeatable = true
			}
			table := tables[currentTableName]
			table.Fields = append(table.Fields, field)
			tables[currentTableName] = table
		}
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
