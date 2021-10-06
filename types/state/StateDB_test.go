package state_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/AccumulateNetwork/accumulated/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/stretchr/testify/require"
)

func bytes32(b []byte) *types.Bytes32 {
	// TODO go1.17: return (*types.Bytes32)((*[32]byte)(b))
	c := (*[32]byte)(unsafe.Pointer(&b[0]))
	return (*types.Bytes32)(c)
}

func unhex(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	require.NoError(t, err, "Failed to decode hex")
	return b
}

func writeStates(t *testing.T, sdb *state.StateDB, height int64) []byte {
	hash, _, err := sdb.WriteStates(height)
	require.NoError(t, err, "Failed to write states")
	return hash
}

func addStateEntry(t *testing.T, sdb *state.StateDB, chainId, txHash, entry string) {
	err := sdb.AddStateEntry(bytes32(unhex(t, chainId)), bytes32(unhex(t, txHash)), unhex(t, entry))
	require.NoError(t, err, "Failed to add state entry")
}

func TestStateDBConsistency(t *testing.T) {
	dir := t.TempDir()
	db := new(badger.DB)
	err := db.InitDB(filepath.Join(dir, "valacc.db"))
	require.NoError(t, err)
	defer db.Close()

	bvcId := sha256.Sum256([]byte("FooBar"))
	sdb := new(state.StateDB)
	sdb.Load(db, bvcId[:], true)

	writeStates(t, sdb, 1)
	writeStates(t, sdb, 2)
	addStateEntry(t, sdb, "E026AEB836A146A6A040943A7857F2131F007E8ECF728210843C6F80D2DDFA7F", "1C8C0365EB98F2D9F02F9D73DE5C200B491A860A897A04D2271BDC829D05A41B", "066A61636D652D383336316265633166643935386262316536363036383733633038616163663866353733366631656233663163353831")
	addStateEntry(t, sdb, "33027FCB49C15E8AD42ED257643837E90321C9BB1C680F00E48E761E5E37C918", "1C8C0365EB98F2D9F02F9D73DE5C200B491A860A897A04D2271BDC829D05A41B", "057A61636D652D3833363162656331666439353862623165363630363837336330386161636638663537333666316562336631633538312F64632F41434D450E64632F41434D450000000000000000000000000000000000000000000000000000048C27395000")
	addStateEntry(t, sdb, "33027FCB49C15E8AD42ED257643837E90321C9BB1C680F00E48E761E5E37C918", "314E35700F46CF7342C83A3B604FCFFEEAC26AA88649BBC3E1710CF150FE92AD", "057A61636D652D3833363162656331666439353862623165363630363837336330386161636638663537333666316562336631633538312F64632F41434D450E64632F41434D450000000000000000000000000000000000000000000000000000048C27394C18")
	h := writeStates(t, sdb, 3)
	require.Equal(t, "5B99F18B6AD8F8AB07996F86000B58ABA92FD6D9D9D729B729C8B8E51AB440D9", fmt.Sprintf("%X", h), "Commit hash mismatch @ height=3")
	writeStates(t, sdb, 4)
	addStateEntry(t, sdb, "94B40EA4E519CE3BA077B2A1E104D726F5CB08E2A2D046F70935BB36E3C800C0", "8F8AAE0CCA2464B83CAC19F5149C784B9552A4413EC186A3F9C596B1E07B7C86", "066A61636D652D356636386233363533613737393466373431393761626632306131376133313138326137323234613834333136303139")
	addStateEntry(t, sdb, "9FB6144EFE59399555E569D18B50629867EA01DD6418DEF6C8391B90C63DAA78", "8F8AAE0CCA2464B83CAC19F5149C784B9552A4413EC186A3F9C596B1E07B7C86", "057A61636D652D3566363862333635336137373934663734313937616266323061313761333131383261373232346138343331363031392F64632F41434D450E64632F41434D4500000000000000000000000000000000000000000000000000000000000003E8")
	h = writeStates(t, sdb, 5)
	require.Equal(t, "718FBEA4699DA2466C53985436F00CBCD5DF2116365F9C8C4FA953306DBB3DB4", fmt.Sprintf("%X", h), "Commit hash mismatch @ height=5")
	writeStates(t, sdb, 6)

	// Reopen the database
	sdb = new(state.StateDB)
	sdb.Load(db, bvcId[:], true)

	// Block 6 does not make changes so is not saved
	require.Equal(t, int64(5), sdb.BlockIndex(), "Block index does not match after load from disk")
	require.Equal(t, "718FBEA4699DA2466C53985436F00CBCD5DF2116365F9C8C4FA953306DBB3DB4", fmt.Sprintf("%X", sdb.RootHash()), "Hash does not match after load from disk")
}
