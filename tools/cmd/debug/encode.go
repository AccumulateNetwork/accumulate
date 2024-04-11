// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var encodeCmd = &cobra.Command{
	Use:     "encode",
	Aliases: []string{"decode"},
	Short:   "Convert something from JSON to binary",
}

var encodeTxnCmd = &cobra.Command{
	Use:     "transaction <transaction (json)>",
	Short:   "Convert a transaction from JSON to binary",
	Aliases: []string{"txn"},
	Args:    cobra.RangeArgs(0, 1),
	Run:     encodeWithValue(new(protocol.Transaction)),
}

var encodeTxnHeaderCmd = &cobra.Command{
	Use:   "header <transaction header (json)>",
	Short: "Convert a transaction header from JSON to binary",
	Args:  cobra.RangeArgs(0, 1),
	Run:   encodeWithValue(new(protocol.TransactionHeader)),
}

var encodeTxnBodyCmd = &cobra.Command{
	Use:   "body <transaction body (json)>",
	Short: "Convert a transaction body from JSON to binary",
	Args:  cobra.RangeArgs(0, 1),
	Run:   encodeWithFunc(protocol.UnmarshalTransactionBodyJSON, protocol.UnmarshalTransactionBody),
}

var encodeSigCmd = &cobra.Command{
	Use:     "signature <signature (json)>",
	Short:   "Convert a signature from JSON to binary",
	Aliases: []string{"sig"},
	Args:    cobra.RangeArgs(0, 1),
	Run:     encodeWithFunc(protocol.UnmarshalSignatureJSON, protocol.UnmarshalSignature),
}

var encodeEnvCmd = &cobra.Command{
	Use:     "envelope <envelope (json)>",
	Short:   "Convert an envelope from JSON to binary",
	Aliases: []string{"env"},
	Args:    cobra.RangeArgs(0, 1),
	Run:     encodeWithValue(new(messaging.Envelope)),
}

var signCmd = &cobra.Command{
	Use:   "sign <signature (json)> <key>",
	Short: "Sign a signature (yes, that's confusing)",
	Run:   sign,
	Args:  cobra.ExactArgs(2),
}

var encodeFlag = struct {
	Hash bool
}{}

var encodeSigFlag = struct {
	SignWith []byte
}{}

func init() {
	cmd.AddCommand(encodeCmd, signCmd)
	encodeCmd.AddCommand(encodeTxnCmd, encodeSigCmd, encodeEnvCmd)
	encodeTxnCmd.AddCommand(encodeTxnHeaderCmd, encodeTxnBodyCmd)

	encodeCmd.PersistentFlags().BoolVarP(&encodeFlag.Hash, "hash", "H", false, "Output the hash instead of the encoded object")
	encodeSigCmd.Flags().BytesHexVarP(&encodeSigFlag.SignWith, "sign-with", "s", nil, "Hex-encoded private key to sign the transaction with")
}

func doSha256(b ...[]byte) []byte {
	digest := sha256.New()
	for _, b := range b {
		_, _ = digest.Write(b)
	}
	return digest.Sum(nil)
}

func argOrStdin(args []string, isHex bool) []byte {
	var b []byte
	var err error
	if len(args) > 0 {
		b = []byte(args[0])
	} else {
		b, err = io.ReadAll(os.Stdin)
		check(err)
	}
	if !isHex {
		return b
	}

	s := strings.TrimSpace(string(b))
	b, err = hex.DecodeString(s)
	check(err)
	return b
}

func isEncode() bool {
	switch os.Args[1] {
	case "encode":
		return true
	case "decode":
		return false
	default:
		panic(fmt.Errorf(`os.Args[1] == "%s"`, os.Args[1]))
	}
}

func encodeWithValue[T encoding.BinaryValue](v T) func(_ *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		if isEncode() {
			check(json.Unmarshal(argOrStdin(args, false), v))

			data, err := v.MarshalBinary()
			check(err)
			if encodeFlag.Hash {
				if u, ok := any(v).(interface{ GetHash() []byte }); ok {
					// Use custom hashing for transactions and certain transaction
					// bodies
					data = u.GetHash()
				} else {
					data = doSha256(data)
				}
			}

			fmt.Printf("%x\n", data)

		} else {
			check(v.UnmarshalBinary(argOrStdin(args, true)))
			data, err := json.MarshalIndent(v, "", "  ")
			check(err)
			fmt.Printf("%s\n", data)
		}
	}
}

func encodeWithFunc[T encoding.BinaryValue](fromJson, fromBinary func([]byte) (T, error)) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		if isEncode() {
			v, err := fromJson(argOrStdin(args, false))
			check(err)

			data, err := v.MarshalBinary()
			check(err)
			if encodeFlag.Hash {
				data = doSha256(data)
			}

			fmt.Printf("%x\n", data)

		} else {
			v, err := fromBinary(argOrStdin(args, true))
			check(err)
			data, err := json.MarshalIndent(v, "", "  ")
			check(err)
			fmt.Printf("%s\n", data)
		}
	}
}

func sign(_ *cobra.Command, args []string) {
	sig, err := protocol.UnmarshalSignatureJSON([]byte(args[0]))
	check(err)

	key, err := hex.DecodeString(args[1])
	check(err)

	signer := new(signing.Builder)
	signer.SetPrivateKey(key)
	_, err = signer.Import(sig)
	check(err)

	h := sig.GetTransactionHash()
	sig, err = signer.Sign(h[:])
	check(err)

	b, err := json.Marshal(sig)
	check(err)
	println(string(b))
}
