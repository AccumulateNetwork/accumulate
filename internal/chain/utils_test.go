package chain

// Whitebox testing utilities

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func LoadStateManagerForTest(t *testing.T, db *database.Database, envelope *protocol.Envelope) (*StateManager, *Delivery) {
	delivery, err := NormalizeEnvelope(envelope)
	require.NoError(t, err)
	require.Len(t, delivery, 1)
	batch := db.Begin(false)
	defer batch.Discard()
	_, err = delivery[0].LoadTransaction(batch)
	require.NoError(t, err)

	return NewStateManagerForTest(t, db, delivery[0].Transaction), delivery[0]
}

func NewStateManagerForTest(t *testing.T, db *database.Database, transaction *protocol.Transaction) *StateManager {
	txid := types.Bytes(transaction.GetHash()).AsBytes32()
	m := new(StateManager)
	m.OriginUrl = transaction.Header.Principal
	m.stateCache = *newStateCache(&config.Network{LocalSubnetID: t.Name()}, transaction.Body.Type(), txid, db.Begin(true))

	require.NoError(t, m.LoadUrlAs(m.OriginUrl, &m.Origin))
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
	case protocol.TransactionTypeAddValidator:
		return AddValidator{}
	case protocol.TransactionTypeRemoveValidator:
		return RemoveValidator{}
	case protocol.TransactionTypeUpdateValidatorKey:
		return UpdateValidatorKey{}
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
	case protocol.TransactionTypePartitionAnchor:
		return PartitionAnchor{}
	default:
		t.Fatalf("Invalid transaction type %v", typ)
		panic("unreachable")
	}
}
