package state

import (
	"crypto/sha256"
	"testing"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/stretchr/testify/require"
)

func TestStateDB_GetChainRange(t *testing.T) {
	s := new(StateDB)
	require.NoError(t, s.Open("", true, false, nil))

	// Set the mark frequency to 4
	s.merkleMgr.MarkPower = 2
	s.merkleMgr.MarkFreq = 1 << s.merkleMgr.MarkPower
	s.merkleMgr.MarkMask = s.merkleMgr.MarkFreq - 1

	id := []byte(t.Name())
	require.NoError(t, s.merkleMgr.SetKey(id))

	const N = 10
	for i := byte(0); i < N; i++ {
		h := sha256.Sum256([]byte{i})
		s.merkleMgr.AddHash(h[:])
	}

	const start, end = 1, 6
	hashes, count, err := s.GetChainRange(id, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(N), count)
	require.Equal(t, end-start, len(hashes))
}

func TestDBTransaction_writeSynthTxnSigs(t *testing.T) {
	type fields struct {
		state        *StateDB
		updates      map[types.Bytes32]*blockUpdates
		writes       map[storage.Key][]byte
		addSynthSigs []*SyntheticSignature
		delSynthSigs [][32]byte
		transactions transactionLists
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &DBTransaction{
				state:        tt.fields.state,
				updates:      tt.fields.updates,
				writes:       tt.fields.writes,
				addSynthSigs: tt.fields.addSynthSigs,
				delSynthSigs: tt.fields.delSynthSigs,
				transactions: tt.fields.transactions,
			}
			if err := tx.writeSynthTxnSigs(); (err != nil) != tt.wantErr {
				t.Errorf("DBTransaction.writeSynthTxnSigs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
