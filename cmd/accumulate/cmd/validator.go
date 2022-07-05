package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/build"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	operatorCmd.AddCommand(
		operatorAddCmd,
		operatorRemoveCmd,
		operatorUpdateKeyCmd)
	validatorCmd.AddCommand(
		validatorAddCmd,
		validatorRemoveCmd,
		validatorUpdateKeyCmd)
	operatorAddCmd.Flags().IntVar(&KeyHeight, "key height", 0, "Specify the key height")
	operatorUpdateKeyCmd.Flags().IntVar(&KeyHeight, "key height", 0, "Specify the key height")
	operatorRemoveCmd.Flags().IntVar(&KeyHeight, "key height", 0, "Specify the key height")
	validatorAddCmd.Flags().IntVar(&KeyHeight, "key height", 0, "Specify the key height")
	validatorUpdateKeyCmd.Flags().IntVar(&KeyHeight, "key height", 0, "Specify the key height")
	validatorRemoveCmd.Flags().IntVar(&KeyHeight, "key height", 0, "Specify the key height")

}

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Manage operators",
}

var operatorAddCmd = &cobra.Command{
	Use:   "add [partition ID] [<keyname>@<keypage>] [key name or path]",
	Short: "Add a operator",
	Run:   runValCmdFunc(addOperator),
	Args:  cobra.RangeArgs(3, 5),
}

var operatorRemoveCmd = &cobra.Command{
	Use:   "remove [partition ID] [<keyname>@<keypage>]  [key name or path]",
	Short: "Remove a operator",
	Run:   runValCmdFunc(removeOperator),
	Args:  cobra.RangeArgs(3, 5),
}

var operatorUpdateKeyCmd = &cobra.Command{
	Use:   "update-key [partition ID] [<keyname>@<keypage>] [old key name or path] [new key name or path]",
	Short: "Update a operator's key",
	Run:   runValCmdFunc(updateOperatorKey),
	Args:  cobra.RangeArgs(4, 6),
}

var validatorCmd = &cobra.Command{
	Use:   "validator",
	Short: "Manage validators",
}

var validatorAddCmd = &cobra.Command{
	Use:   "add [partition ID] [<keyname>@<keypage>] [key name or path]",
	Short: "Add a validator",
	Run:   runValCmdFunc(addValidator),
	Args:  cobra.RangeArgs(3, 5),
}

var validatorRemoveCmd = &cobra.Command{
	Use:   "remove [partition ID] [<keyname>@<keypage>] [key name or path]",
	Short: "Remove a validator",
	Run:   runValCmdFunc(removeValidator),
	Args:  cobra.RangeArgs(3, 5),
}

var validatorUpdateKeyCmd = &cobra.Command{
	Use:   "update-key [partition ID] [<keyname>@<keypage>] [old key name or path] [new key name or path]",
	Short: "Update a validator's key",
	Run:   runValCmdFunc(updateValidatorKey),
	Args:  cobra.RangeArgs(4, 6),
}

func runValCmdFunc(fn func(values *core.GlobalValues, pageCount int, signer []*signing.Builder, partition string, args []string) (*protocol.Envelope, error)) func(cmd *cobra.Command, args []string) {
	return runCmdFunc(func(args []string) (string, error) {
		describe, err := Client.Describe(context.Background())
		if err != nil {
			return PrintJsonRpcError(err)
		}
		if describe.Values.Globals == nil {
			return "", errors.New("cannot determine the network's global values")
		}

		req := new(api.GeneralQuery)
		req.Url = protocol.DnUrl().JoinPath(protocol.Operators, "1")
		resp := new(api.ChainQueryResponse)
		page := new(protocol.KeyPage)
		resp.Data = page
		err = Client.RequestAPIv2(context.Background(), "query", req, resp)
		if err != nil {
			return PrintJsonRpcError(err)
		}

		partition := args[0]
		if partition == "dn" {
			partition = protocol.Directory
		}
		args[0] = protocol.PartitionUrl(partition).JoinPath(protocol.Operators, "1").String()
		args, principal, signers, err := parseArgsAndPrepareSigner(args)
		if err != nil {
			return "", err
		}

		env, err := fn(&describe.Values, len(page.Keys), signers, partition, args)
		if err != nil {
			return "", err
		}

		return dispatchTxAndPrintResponse(env, principal, signers)
	})
}

func addOperator(values *core.GlobalValues, pageCount int, signers []*signing.Builder, _ string, args []string) (*protocol.Envelope, error) {
	newKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	return build.AddToOperatorPage(values, pageCount, newKey.PublicKeyHash(), signers...)
}

func removeOperator(values *core.GlobalValues, pageCount int, signers []*signing.Builder, _ string, args []string) (*protocol.Envelope, error) {
	oldKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	return build.RemoveFromOperatorPage(values, pageCount, oldKey.PublicKeyHash(), signers...)
}

func updateOperatorKey(_ *core.GlobalValues, _ int, signers []*signing.Builder, _ string, args []string) (*protocol.Envelope, error) {
	oldKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	newKey, err := resolvePublicKey(args[1])
	if err != nil {
		return nil, err
	}

	return build.UpdateKeyOnOperatorPage(oldKey.PublicKeyHash(), newKey.PublicKeyHash(), signers...)
}

func addValidator(values *core.GlobalValues, pageCount int, signers []*signing.Builder, partition string, args []string) (*protocol.Envelope, error) {
	newKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	return build.AddValidator(values, pageCount, newKey.PublicKey, partition, signers...)
}

func removeValidator(values *core.GlobalValues, pageCount int, signers []*signing.Builder, partition string, args []string) (*protocol.Envelope, error) {
	oldKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	return build.RemoveValidator(values, pageCount, oldKey.PublicKey, partition, signers...)
}

func updateValidatorKey(values *core.GlobalValues, _ int, signers []*signing.Builder, partition string, args []string) (*protocol.Envelope, error) {
	oldKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	newKey, err := resolvePublicKey(args[1])
	if err != nil {
		return nil, err
	}

	return build.UpdateValidatorKey(values, oldKey.PublicKey, newKey.PublicKey, partition, signers...)
}
