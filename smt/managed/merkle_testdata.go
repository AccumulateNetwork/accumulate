package managed

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
