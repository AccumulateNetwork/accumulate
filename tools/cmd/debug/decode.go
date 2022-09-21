package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var decodeCmd = &cobra.Command{
	Use:   "decode",
	Short: "Decode Accumulate records",
}

var decodeTxnCmd = &cobra.Command{
	Use:     "transaction <transaction>",
	Aliases: []string{"txn"},
	Args:    cobra.ExactArgs(1),
	Run:     decodeWithValue(new(protocol.Transaction)),
}

var decodeTxnHeaderCmd = &cobra.Command{
	Use:  "header <transaction header>",
	Args: cobra.ExactArgs(1),
	Run:  decodeWithValue(new(protocol.TransactionHeader)),
}

var decodeTxnBodyCmd = &cobra.Command{
	Use:  "body <transaction body>",
	Args: cobra.ExactArgs(1),
	Run:  decodeWithFunc(protocol.UnmarshalTransactionBody),
}

var decodeSigCmd = &cobra.Command{
	Use:     "signature <signature>",
	Aliases: []string{"sig"},
	Run:     decodeWithFunc(protocol.UnmarshalKeySignature),
}

func init() {
	cmd.AddCommand(decodeCmd, signCmd)
	decodeCmd.AddCommand(decodeTxnCmd, decodeSigCmd)
	decodeTxnCmd.AddCommand(decodeTxnHeaderCmd, decodeTxnBodyCmd)
}

func decodeWithValue[T encoding.BinaryValue](v T) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		b, err := hex.DecodeString(args[0])
		check(err)
		check(v.UnmarshalBinary(b))
		b, err = json.Marshal(v)
		check(err)
		fmt.Printf("%s\n", b)
	}
}

func decodeWithFunc[T encoding.BinaryValue](fn func([]byte) (T, error)) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		b, err := hex.DecodeString(args[0])
		check(err)
		v, err := fn(b)
		check(err)
		b, err = json.Marshal(v)
		check(err)
		fmt.Printf("%s\n", b)
	}
}
