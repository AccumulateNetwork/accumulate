// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log/slog"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <envelope (json)>",
	Short: "Verify an envelope and its signatures",
	Args:  cobra.ExactArgs(1),
	Run:   verifyEnvelope,
}

func init() {
	cmd.AddCommand(verifyCmd)
}

func verifyEnvelope(_ *cobra.Command, args []string) {
	env := new(messaging.Envelope)
	check(env.UnmarshalJSON([]byte(args[0])))

	messages, err := env.Normalize()
	check(err)

	for i, msg := range messages {
		sigMsg, ok := msg.(*messaging.SignatureMessage)
		if !ok {
			continue
		}
		sig, ok := sigMsg.Signature.(protocol.UserSignature)
		if !ok {
			slog.Info("Skipping non-user signature", "type", sigMsg.Signature.Type())
			continue
		}
		h := sig.GetTransactionHash()
		if h == [32]byte{} {
			slog.Info("Skipping signature: no transaction hash", "type", sigMsg.Signature.Type())
			continue
		}
		if !sig.Verify(nil, protocol.SignableHash(h)) {
			slog.Error("Signature is invalid", "message", i)
		}
	}
}
