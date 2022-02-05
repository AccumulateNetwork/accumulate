package cmd

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func LookupByLabel(label string) ([]byte, error) {
	label, _ = LabelForLiteTokenAccount(label)

	pubKey, err := Db.Get(BucketLabel, []byte(label))
	if err != nil {
		return nil, fmt.Errorf("valid key not found for %s", label)
	}
	return LookupByPubKey(pubKey)
}

// LabelForLiteTokenAccount returns the identity of the token account if label
// is a valid token account URL. Otherwise, LabelForLiteTokenAccount returns the
// original value.
func LabelForLiteTokenAccount(label string) (string, bool) {
	u, err := url.Parse(label)
	if err != nil {
		return label, false
	}

	key, _, err := protocol.ParseLiteTokenAddress(u)
	if key == nil || err != nil {
		return label, false
	}

	return u.Hostname(), true
}

func LookupByPubKey(pubKey []byte) ([]byte, error) {
	return Db.Get(BucketKeys, pubKey)
}

func FindLabelFromPubKey(pubKey []byte) (lab string, err error) {
	b, err := Db.GetBucket(BucketLabel)
	if err != nil {
		return lab, err
	}

	for _, v := range b.KeyValueList {
		if bytes.Equal(v.Value, pubKey) {
			lab = string(v.Key)
			break
		}
	}

	if lab == "" {
		err = fmt.Errorf("key name not found for %x", pubKey)
	}
	return lab, err
}

func lookupSeed() (seed []byte, err error) {
	seed, err = Db.Get(BucketMnemonic, []byte("seed"))
	if err != nil {
		return nil, fmt.Errorf("mnemonic seed doesn't exist")
	}

	return seed, nil
}
