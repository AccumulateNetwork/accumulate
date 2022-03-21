package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var managerCmd = &cobra.Command{
	Use:   "manager",
	Short: "add and remove manager from chain",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		switch args[0] {
		case "set":
			out, err = SetManager(args[1], args[2:])
		case "remove":
			out, err = RemoveManager(args[1], args[2:])
		default:
			fmt.Println("Usage:")
			PrintManagerSet()
		}
		printOutput(cmd, out, err)
	},
}

func PrintManagerSet() {
	fmt.Println("  accumulate manager set [origin url] [signing key name] [key index (optional)] [key height (optional)] [manager keybook url]			Set manager keybook to the chain")
	fmt.Println("  accumulate manager remove [origin url] [signing key name] [key index (optional)] [key height (optional)]			Removes manager keybook from the chain")

}

// SetManager sets manager to the chain
func SetManager(origin string, args []string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	managerKeyBookUrl, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}

	req := protocol.UpdateManager{}
	req.ManagerKeyBook = managerKeyBookUrl
	res, err := dispatchTxRequest("update-manager", &req, nil, u, signer)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}

// RemoveManager removes manager to the chain
func RemoveManager(origin string, args []string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	_, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	req := protocol.RemoveManager{}
	res, err := dispatchTxRequest("remove-manager", &req, nil, u, signer)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}
