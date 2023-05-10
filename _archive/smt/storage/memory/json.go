// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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
		vv[k.Prefix+hex.EncodeToString(k.Key[:])] = hex.EncodeToString(v) //nolint:rangevarref
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
		i := len(k) - 64
		if i < 0 {
			return fmt.Errorf("invalid key length: want ≥ 64, got %d", len(k))
		}
		kk, err := hex.DecodeString(k[i:])
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
		sk := storage.Key(*(*[32]byte)(kk))
		m.entries[PrefixedKey{k[:i], sk}] = vv
	}
	return nil
}
