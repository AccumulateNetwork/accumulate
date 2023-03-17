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
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
)

func (m *DB) MarshalJSON() ([]byte, error) {
	vv := make(map[string]string, len(m.entries.values))
	for k, v := range m.entries.values {
		s := hex.EncodeToString(k.key[:])
		if k.prefix != "" {
			s = k.prefix + "." + s
		}
		vv[s] = hex.EncodeToString(v) //nolint:rangevarref
	}
	return json.Marshal(vv)
}

func (m *DB) UnmarshalJSON(b []byte) error {
	var vv map[string]string
	err := json.Unmarshal(b, &vv)
	if err != nil {
		return err
	}
	for s, v := range vv {
		var k storeKey
		i := strings.IndexByte(s, '.')
		if i >= 0 {
			k.prefix, s = s[:i], s[i+1:]
		}
		kk, err := hex.DecodeString(s)
		if err != nil {
			return err
		}
		if len(kk) != 32 {
			return fmt.Errorf("invalid key length: want 32, got %d", len(kk))
		}
		k.key = *(*storage.Key)(kk)
		vv, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		m.entries.values[k] = vv
	}
	return nil
}
