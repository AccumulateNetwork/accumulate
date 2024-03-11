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

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var encodeCmd = &cobra.Command{
	Use:   "encode",
	Short: "Encode Accumulate records",
}

var encodeTxnCmd = &cobra.Command{
	Use:     "transaction <transaction (json)>",
	Aliases: []string{"txn"},
	Args:    cobra.ExactArgs(1),
	Run:     encodeWithValue(new(protocol.Transaction)),
}

var encodeTxnHeaderCmd = &cobra.Command{
	Use:  "header <transaction header (json)>",
	Args: cobra.ExactArgs(1),
	Run:  encodeWithValue(new(protocol.TransactionHeader)),
}

var encodeTxnBodyCmd = &cobra.Command{
	Use:  "body <transaction body (json)>",
	Args: cobra.ExactArgs(1),
	Run:  encodeWithFunc(protocol.UnmarshalTransactionBodyJSON),
}

var encodeSigCmd = &cobra.Command{
	Use:     "signature <signature (json)>",
	Aliases: []string{"sig"},
	Run:     encodeWithFunc(protocol.UnmarshalSignatureJSON),
}

var signCmd = &cobra.Command{
	Use:  "sign <signature (json)> <key>",
	Run:  sign,
	Args: cobra.ExactArgs(2),
}

var encodeFlag = struct {
	Hash bool
}{}

var encodeSigFlag = struct {
	SignWith []byte
}{}

func init() {
	cmd.AddCommand(encodeCmd, signCmd)
	encodeCmd.AddCommand(encodeTxnCmd, encodeSigCmd)
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

func encodeWithValue[T encoding.BinaryValue](v T) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		check(json.Unmarshal([]byte(args[0]), v))

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
	}
}

func encodeWithFunc[T encoding.BinaryValue](fn func([]byte) (T, error)) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		v, err := fn([]byte(args[0]))
		check(err)

		data, err := v.MarshalBinary()
		check(err)
		if encodeFlag.Hash {
			data = doSha256(data)
		}

		fmt.Printf("%x\n", data)
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
