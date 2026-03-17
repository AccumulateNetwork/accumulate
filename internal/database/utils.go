// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func GetSignaturesForSigner(transaction *Transaction, signer protocol.Signer) ([]protocol.Signature, error) {
	// Load the signature set
	sigset, err := transaction.ReadSignaturesForSigner(signer)
	if err != nil {
		return nil, fmt.Errorf("load signatures set %v: %w", signer.GetUrl(), err)
	}

	entries := sigset.Entries()
	signatures := make([]protocol.Signature, 0, len(entries))
	for _, e := range entries {
		var msg messaging.MessageWithSignature
		err = transaction.parent.Message(e.SignatureHash).Main().GetAs(&msg)
		if err != nil {
			return nil, fmt.Errorf("load signature entry %X: %w", e.SignatureHash, err)
		}

		signatures = append(signatures, msg.GetSignature())
	}
	return signatures, nil
}

func newBlockEntryLog(_ record.Record, logger *slog.Logger, store record.Store, key *record.Key, _ string) *indexing.Log[*BlockLedger] {
	return indexing.NewLog[*BlockLedger](logger, store, key, 4<<10)
}
