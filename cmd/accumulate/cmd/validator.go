package cmd

import (
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	validatorCmd.AddCommand(
		validatorAddCmd,
		validatorRemoveCmd,
		validatorUpdateKeyCmd)
}

var validatorCmd = &cobra.Command{
	Use:   "validator",
	Short: "Manage validators",
}

var validatorAddCmd = &cobra.Command{
	Use:   "add [subnet URL] [signing key name] [key index (optional)] [key height (optional)] [key name or path]",
	Short: "Add a validator",
	Run:   runCmdFunc(addValidator),
	Args:  cobra.RangeArgs(3, 5),
}

var validatorRemoveCmd = &cobra.Command{
	Use:   "remove [subnet URL] [signing key name] [key index (optional)] [key height (optional)] [key name or path]",
	Short: "Remove a validator",
	Run:   runCmdFunc(removeValidator),
	Args:  cobra.RangeArgs(3, 5),
}

var validatorUpdateKeyCmd = &cobra.Command{
	Use:   "update-key [subnet URL] [signing key name] [key index (optional)] [key height (optional)] [old key name or path] [new key name or path]",
	Short: "Update a validator's key",
	Run:   runCmdFunc(updateValidatorKey),
	Args:  cobra.RangeArgs(4, 6),
}

func addValidator(args []string) (string, error) {
	args, principal, signer, err := parseArgsAndPrepareSigner(args)
	if err != nil {
		return "", err
	}

	newKey, _, _, err := resolvePublicKey(args[0])
	if err != nil {
		return "", err
	}

	txn := new(protocol.AddValidator)
	txn.PubKey = newKey
	return dispatchTxAndPrintResponse("add-validator", txn, nil, principal, signer)
}

func removeValidator(args []string) (string, error) {
	args, principal, signer, err := parseArgsAndPrepareSigner(args)
	if err != nil {
		return "", err
	}

	oldKey, _, _, err := resolvePublicKey(args[0])
	if err != nil {
		return "", err
	}

	txn := new(protocol.RemoveValidator)
	txn.PubKey = oldKey
	return dispatchTxAndPrintResponse("remove-validator", txn, nil, principal, signer)
}

func updateValidatorKey(args []string) (string, error) {
	args, principal, signer, err := parseArgsAndPrepareSigner(args)
	if err != nil {
		return "", err
	}

	oldKey, _, _, err := resolvePublicKey(args[0])
	if err != nil {
		return "", err
	}

	newKey, _, _, err := resolvePublicKey(args[1])
	if err != nil {
		return "", err
	}

	txn := new(protocol.UpdateValidatorKey)
	txn.PubKey = oldKey
	txn.NewPubKey = newKey
	return dispatchTxAndPrintResponse("update-validator-key", txn, nil, principal, signer)
}
