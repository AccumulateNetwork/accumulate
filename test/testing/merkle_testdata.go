// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"encoding/hex"

	"gopkg.in/yaml.v3"
)

type YamlHexString []byte

func (s *YamlHexString) UnmarshalYAML(value *yaml.Node) error {
	var str string
	err := value.Decode(&str)
	if err != nil {
		return err
	}

	*s, err = hex.DecodeString(str)
	if err != nil {
		return err
	}

	return nil
}

type MerkleTestCase struct {
	Root    YamlHexString
	Entries []YamlHexString
	Cascade []YamlHexString
}
