package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	outFilePath string
	packageName string
	language    string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gen-typedef [ymlfile]",
		Short: "Generate type definitions from a YAML file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ymlFile := args[0]
			if err := processYAMLFile(ymlFile, outFilePath, packageName, language); err != nil {
				log.Fatal(err)
			}
		},
	}

	// Add flags to the command
	rootCmd.Flags().StringVarP(&outFilePath, "out", "o", "", "Output file path")
	rootCmd.Flags().StringVarP(&packageName, "package", "p", "main", "Package name for the generated Go file")
	rootCmd.Flags().StringVarP(&language, "language", "l", "go", "Language for the generated code (go, C, java, javascript)")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func processYAMLFile(ymlFilePath, outFilePath, packageName, language string) error {
	ymlFile, err := os.ReadFile(ymlFilePath)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	if err := yaml.Unmarshal(ymlFile, &data); err != nil {
		return err
	}

	if language == "go" {
		goData, err := generateGoCode(data, packageName)
		if err != nil {
			return err
		}

		if outFilePath == "" {
			outFilePath = strings.TrimSuffix(ymlFilePath, filepath.Ext(ymlFilePath)) + "_gen_typedef.go"
		}

		if err := os.WriteFile(outFilePath, goData, 0644); err != nil {
			return err
		}

		fmt.Printf("Go code output saved to %s\n", outFilePath)
	}

	return nil
}

func generateGoCode(data map[string]interface{}, packageName string) ([]byte, error) {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("package %s\n\n", packageName))

	for tableName := range data {
		typeName := capitalizeFirst(tableName)
		builder.WriteString(fmt.Sprintf("type %s uint64\n\n", typeName))
	}

	return []byte(builder.String()), nil
}

func capitalizeFirst(s string) string {
	for i, v := range s {
		return string(unicode.ToUpper(v)) + s[i+1:]
	}
	return ""
}
