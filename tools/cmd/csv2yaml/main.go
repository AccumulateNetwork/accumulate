package main

import (
	"log"

	"github.com/spf13/cobra"
)

var (
	outFilePath string
	packageName string
)

func main() {
	var rootCmd = &cobra.Command{Use: "csv2yaml"}

	var typesCmd = &cobra.Command{
		Use:   "types [csvfile]",
		Short: "Convert CSV to YAML for types",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			csvFile := args[0]
			if err := processCSVTypes(csvFile, outFilePath, packageName); err != nil {
				log.Fatal(err)
			}
		},
	}

	var enumsCmd = &cobra.Command{
		Use:   "enums [csvfile]",
		Short: "Convert CSV to YAML for enums",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			csvFile := args[0]
			if err := processCSVEnums(csvFile, outFilePath, packageName); err != nil {
				log.Fatal(err)
			}
		},
	}

	// Add flags to both subcommands
	typesCmd.Flags().StringVarP(&outFilePath, "out", "o", "", "Output file path")
	typesCmd.Flags().StringVarP(&packageName, "package", "p", "main", "Package name for the generated Go file")

	enumsCmd.Flags().StringVarP(&outFilePath, "out", "o", "", "Output file path")
	enumsCmd.Flags().StringVarP(&packageName, "package", "p", "main", "Package name for the generated Go file")

	rootCmd.AddCommand(typesCmd, enumsCmd)
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
