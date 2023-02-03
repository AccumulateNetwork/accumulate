// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

// Whitebox testing utilities

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func LoadStateManagerForTest(t *testing.T, db database.Beginner, envelope *messaging.Envelope) (*StateManager, *Delivery) {
	t.Helper()
	delivery, err := NormalizeEnvelope(envelope)
	require.NoError(t, err)
	require.Len(t, delivery, 1)
	batch := db.Begin(false)
	defer batch.Discard()
	_, err = delivery[0].LoadTransaction(batch)
	require.NoError(t, err)

	return NewStateManagerForTest(t, db, delivery[0].Transaction), delivery[0]
}

func NewStateManagerForTest(t *testing.T, db database.Beginner, transaction *protocol.Transaction) *StateManager {
	t.Helper()
	m := NewStateManagerForFuzz(t, db, transaction)
	require.NoError(t, m.LoadUrlAs(m.OriginUrl, &m.Origin))
	return m
}

func NewStateManagerForFuzz(t *testing.T, db database.Beginner, transaction *protocol.Transaction) *StateManager {
	t.Helper()
	txid := *(*[32]byte)(transaction.GetHash())
	m := new(StateManager)
	m.OriginUrl = transaction.Header.Principal
	m.stateCache = *newStateCache(&config.Describe{PartitionId: strings.ReplaceAll(strings.ReplaceAll(t.Name(), "/", "-"), "#", "-")}, nil, transaction.Body.Type(), txid, db.Begin(true))
	m.Globals = core.NewGlobals(nil)
	return m
}

func GetExecutor(t *testing.T, typ protocol.TransactionType) TransactionExecutor {
	switch typ {
	case protocol.TransactionTypeCreateIdentity:
		return CreateIdentity{}
	case protocol.TransactionTypeCreateTokenAccount:
		return CreateTokenAccount{}
	case protocol.TransactionTypeSendTokens:
		return SendTokens{}
	case protocol.TransactionTypeCreateDataAccount:
		return CreateDataAccount{}
	case protocol.TransactionTypeWriteData:
		return WriteData{}
	case protocol.TransactionTypeWriteDataTo:
		return WriteDataTo{}
	case protocol.TransactionTypeAcmeFaucet:
		return AcmeFaucet{}
	case protocol.TransactionTypeCreateToken:
		return CreateToken{}
	case protocol.TransactionTypeIssueTokens:
		return IssueTokens{}
	case protocol.TransactionTypeBurnTokens:
		return BurnTokens{}
	case protocol.TransactionTypeCreateKeyPage:
		return CreateKeyPage{}
	case protocol.TransactionTypeCreateKeyBook:
		return CreateKeyBook{}
	case protocol.TransactionTypeAddCredits:
		return AddCredits{}
	case protocol.TransactionTypeUpdateKeyPage:
		return UpdateKeyPage{}
	case protocol.TransactionTypeUpdateAccountAuth:
		return UpdateAccountAuth{}
	case protocol.TransactionTypeUpdateKey:
		return UpdateKey{}
	case protocol.TransactionTypeSyntheticCreateIdentity:
		return SyntheticCreateIdentity{}
	case protocol.TransactionTypeSyntheticWriteData:
		return SyntheticWriteData{}
	case protocol.TransactionTypeSyntheticDepositTokens:
		return SyntheticDepositTokens{}
	case protocol.TransactionTypeSyntheticDepositCredits:
		return SyntheticDepositCredits{}
	case protocol.TransactionTypeSyntheticBurnTokens:
		return SyntheticBurnTokens{}
	case protocol.TransactionTypeSyntheticForwardTransaction:
		return SyntheticForwardTransaction{}
	case protocol.TransactionTypeDirectoryAnchor:
		return DirectoryAnchor{}
	case protocol.TransactionTypeBlockValidatorAnchor:
		return PartitionAnchor{}
	default:
		t.Fatalf("Invalid transaction type %v", typ)
		panic("unreachable")
	}
}
