package memory

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
)

func (m *DB) MarshalJSON() ([]byte, error) {
	vv := make(map[string]string, len(m.entries))
	for k, v := range m.entries {
		vv[hex.EncodeToString(k[:])] = hex.EncodeToString(v) //nolint:rangevarref
	}
	return json.Marshal(vv)
}

func (m *DB) UnmarshalJSON(b []byte) error {
	var vv map[string]string
	err := json.Unmarshal(b, &vv)
	if err != nil {
		return err
	}
	for k, v := range vv {
		kk, err := hex.DecodeString(k)
		if err != nil {
			return err
		}
		if len(kk) != 32 {
			return fmt.Errorf("invalid key length: want 32, got %d", len(kk))
		}
		vv, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		m.entries[storage.Key(*(*[32]byte)(kk))] = vv
	}
	return nil
}
