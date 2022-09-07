package main

import (
	"fmt"
	"io/ioutil"

	"github.com/FactomProject/factomd/common/adminBlock"
	"github.com/FactomProject/factomd/common/directoryBlock"
	"github.com/FactomProject/factomd/common/entryBlock"
	"github.com/FactomProject/factomd/common/entryCreditBlock"
	"github.com/FactomProject/factomd/common/factoid"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom"
)

var cmdList = &cobra.Command{
	Use:   "ls [object file*]",
	Short: "List a Factom object dump",
	Args:  cobra.MinimumNArgs(1),
	Run:   list,
}

func init() {
	cmd.AddCommand(cmdList)
}

func list(_ *cobra.Command, args []string) {
	for _, filename := range args {
		input, err := ioutil.ReadFile(filename)
		checkf(err, "read %s", filename)

		err = factom.ReadObjectFile(input, nil, func(_ *factom.Header, object interface{}) {
			switch object.(type) {
			case *directoryBlock.DirectoryBlock:
				fmt.Println("Directory block")
			case *adminBlock.AdminBlock:
				fmt.Println("Admin block")
			case *factoid.FBlock:
				fmt.Println("Factoid block")
			case *entryCreditBlock.ECBlock:
				fmt.Println("Entry credit block")
			case *entryBlock.EBlock:
				fmt.Println("Entry block")
			case *entryBlock.Entry:
				fmt.Println("Entry")
			case *factoid.Transaction:
				fmt.Println("Factoid transaction")
			}
		})
		checkf(err, "process object file")
	}
}
