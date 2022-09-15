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
	followerCmd.AddCommand(
		followerAddCmd,
		followerRemoveCmd,
		followerUpdateKeyCmd)

}

var followerCmd = &cobra.Command{
	Use:   "follower",
	Short: "Manage followers",
}
var followerAddCmd = &cobra.Command{
	Use:   "add [partition ID] [key name[@key book or page]] [key name or path]",
	Short: "Add a follower",
	Run:   runFolCmdFunc(addFollower),
	Args:  cobra.RangeArgs(3, 5),
}

var followerRemoveCmd = &cobra.Command{
	Use:   "remove [partition ID] [key name[@key book or page]] [key name or path]",
	Short: "Remove a follower",
	Run:   runFolCmdFunc(removeFollower),
	Args:  cobra.RangeArgs(3, 5),
}

var followerUpdateKeyCmd = &cobra.Command{
	Use:   "update-key [partition ID] [key name[@key book or page]] [old key name or path] [new key name or path]",
	Short: "Update a follower's key",
	Run:   runFolCmdFunc(updateFollowerKey),
	Args:  cobra.RangeArgs(4, 6),
}

func runFolCmdFunc(fn func(values *core.GlobalValues, pageCount int, signer []*signing.Builder, partition string, args []string) (*protocol.Envelope, error)) func(cmd *cobra.Command, args []string) {
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
		args[0] = protocol.DnUrl().JoinPath(protocol.Operators, "1").String()
		args, principal, signers, err := parseArgsAndPrepareSigner(args)
		if err != nil {
			return "", err
		}

		env, err := fn(&describe.Values, len(page.Keys), signers, partition, args)
		if err != nil {
			return "", err
		}

		return dispatchTxAndPrintResponse(env, principal, nil)
	})
}

func addFollower(values *core.GlobalValues, pageCount int, signers []*signing.Builder, _ string, args []string) (*protocol.Envelope, error) {
	newKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	return build.AddValidator(values, pageCount, newKey.PublicKey, args[0], true, signers...)
}

func removeFollower(values *core.GlobalValues, pageCount int, signers []*signing.Builder, _ string, args []string) (*protocol.Envelope, error) {
	oldKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	return build.RemoveValidator(values, pageCount, oldKey.PublicKey, signers...)
}

func updateFollowerKey(values *core.GlobalValues, _ int, signers []*signing.Builder, _ string, args []string) (*protocol.Envelope, error) {
	oldKey, err := resolvePublicKey(args[0])
	if err != nil {
		return nil, err
	}

	newKey, err := resolvePublicKey(args[1])
	if err != nil {
		return nil, err
	}

	return build.UpdateValidatorKey(values, oldKey.PublicKey, newKey.PublicKey, signers...)
}
