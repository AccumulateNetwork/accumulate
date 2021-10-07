package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var adiCmd = &cobra.Command{
	Use:   "adi",
	Short: "Create and manage ADI",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					GetADI(args[1])
				} else {
					fmt.Println("Usage:")
					PrintADIGet()
				}
			case "public":
				if len(args) > 1 {
					PublicADI(args[1])
				} else {
					fmt.Println("Usage:")
					PrintADIPublic()
				}
			case "create":
				if len(args) > 1 {
					NewADI(args[1])
				} else {
					fmt.Println("Usage:")
					PrintADICreate()
				}
			case "import":
				if len(args) > 2 {
					ImportADI(args[1], args[2])
				} else {
					fmt.Println("Usage:")
					PrintADIImport()
				}
			default:
				fmt.Println("Usage:")
				PrintADI()
			}
		} else {
			fmt.Println("Usage:")
			PrintADI()
		}

	},
}

func init() {
	rootCmd.AddCommand(adiCmd)
}

func PrintADIGet() {
	fmt.Println("  accumulate adi get [URL]			Get existing ADI by URL")
}

func PrintADIPublic() {
	fmt.Println("  accumulate adi public [URL]			Print public keys hashes for chosen ADI")
}

func PrintADICreate() {
	fmt.Println("  accumulate adi create [URL] [SIGNER ADI]	Create new ADI")
}

func PrintADIImport() {
	fmt.Println("  accumulate adi import [URL] [PRIVATE KEY]	Import Existing ADI")
}

func PrintADI() {
	PrintADIGet()
	PrintADIPublic()
	PrintADICreate()
	PrintADIImport()
}

func GetADI(url string) {

	/*
		var res interface{}
		var str []byte

		params := acmeapi.APIRequestURL{}
		params.URL = types.String(url)

		if err := Client.Request(context.Background(), "adi", params, &res); err != nil {
			log.Fatal(err)
		}

		str, err := json.Marshal(res)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(str))
	*/

	fmt.Println("ADI functionality is not available on Testnet")

}

func PublicADI(url string) {

	fmt.Println("ADI functionality is not available on Testnet")

}

func NewADI(url string) {

	fmt.Println("ADI functionality is not available on Testnet")

}

func ImportADI(url string, pk string) {

	fmt.Println("ADI functionality is not available on Testnet")

}
