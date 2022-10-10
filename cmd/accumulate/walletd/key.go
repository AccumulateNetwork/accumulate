package walletd

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func LookupByLiteTokenUrl(lite string) (*Key, error) {
	liteKey, isLite := LabelForLiteTokenAccount(lite)
	if !isLite {
		return nil, fmt.Errorf("invalid lite account %s", liteKey)
	}

	label, err := GetWallet().Get(BucketLite, []byte(liteKey))
	if err != nil {
		return nil, fmt.Errorf("lite account not found %s", lite)
	}

	return LookupByLabel(string(label))
}

func LookupByLiteIdentityUrl(lite string) (*Key, error) {
	liteKey, isLite := LabelForLiteIdentity(lite)
	if !isLite {
		return nil, fmt.Errorf("invalid lite identity %s", liteKey)
	}

	label, err := GetWallet().Get(BucketLite, []byte(liteKey))
	if err != nil {
		return nil, fmt.Errorf("lite identity account not found %s", lite)
	}

	return LookupByLabel(string(label))
}

func LookupByLabel(label string) (*Key, error) {
	k := new(Key)
	return k, k.LoadByLabel(label)
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

// LabelForLiteIdentity returns the label of the LiteIdentity if label
// is a valid LiteIdentity account URL. Otherwise, LabelForLiteIdentity returns the
// original value.
func LabelForLiteIdentity(label string) (string, bool) {
	u, err := url.Parse(label)
	if err != nil {
		return label, false
	}

	key, err := protocol.ParseLiteIdentity(u)
	if key == nil || err != nil {
		return label, false
	}

	return u.Hostname(), true
}

func LookupByPubKey(pubKey []byte) (*Key, error) {
	k := new(Key)
	return k, k.LoadByPublicKey(pubKey)
}

func GetKeyList() (kla []api.KeyData, err error) {
	b, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		return nil, err
	}

	for _, v := range b.KeyValueList {
		k := Key{}
		err := k.LoadByLabel(string(v.Key))
		if err != nil {
			return nil, err
		}
		kla = append(kla, api.KeyData{PublicKey: k.PublicKey, Name: string(v.Key), KeyInfo: k.KeyInfo})
	}
	return kla, nil
}

func ListKeyPublic() (out string, err error) {
	out = "Public Key\t\t\t\t\t\t\t\tKey name\n"
	b, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		return "", err
	}

	for _, v := range b.KeyValueList {
		out += fmt.Sprintf("%x\t%s\n", v.Value, v.Key)
	}
	return out, nil
}

func FindLabelFromPublicKeyHash(pubKeyHash []byte) (lab string, err error) {
	b, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		return lab, err
	}

	for _, v := range b.KeyValueList {
		keyHash := sha256.Sum256(v.Value)
		if bytes.Equal(keyHash[:], pubKeyHash) {
			lab = string(v.Key)
			break
		}
	}

	if lab == "" {
		err = fmt.Errorf("key name not found for key hash %x", pubKeyHash)
	}
	return lab, err
}

func FindLabelFromPubKey(pubKey []byte) (lab string, err error) {
	b, err := GetWallet().GetBucket(BucketLabel)
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
