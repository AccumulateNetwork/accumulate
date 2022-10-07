package cmd

import (
	"github.com/spf13/cobra"
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
	Run:   runValCmdFunc(addFollower),
	Args:  cobra.RangeArgs(3, 5),
}

var followerRemoveCmd = &cobra.Command{
	Use:   "remove [partition ID] [key name[@key book or page]] [key name or path]",
	Short: "Remove a follower",
	Run:   runValCmdFunc(removeFollower),
	Args:  cobra.RangeArgs(3, 5),
}

var followerUpdateKeyCmd = &cobra.Command{
	Use:   "update-key [partition ID] [key name[@key book or page]] [old key name or path] [new key name or path]",
	Short: "Update a follower's key",
	Run:   runValCmdFunc(updateFollowerKey),
	Args:  cobra.RangeArgs(4, 6),
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
