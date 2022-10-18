// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package walletd

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/db"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	url2 "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

func RestoreAccounts() (out string, err error) {
	walletVersion, err := GetWallet().GetRaw(db.BucketConfig, []byte("version"))
	if err == nil {
		var v db.Version
		v.FromBytes(walletVersion)
		//if there is no error getting version, check to see if it is the right version
		if db.WalletVersion.Compare(v) == 0 {
			//no need to update
			return "", nil
		}
		if db.WalletVersion.Compare(v) < 0 {
			return "", fmt.Errorf("cannot update wallet to an older version, wallet database version is %v, cli version is %v", v.String(), db.WalletVersion.String())
		}
	}

	anon, err := GetWallet().GetBucket(BucketAnon)
	if err == nil {
		for _, v := range anon.KeyValueList {
			u, err := url2.Parse(string(v.Key))
			if err != nil {
				out += fmt.Sprintf("%q is not a valid URL\n", v.Key)
			}
			if u != nil {
				key, _, err := protocol.ParseLiteTokenAddress(u)
				if err != nil {
					out += fmt.Sprintf("%q is not a valid lite account: %v\n", v.Key, err)
				} else if key == nil {
					out += fmt.Sprintf("%q is not a lite account\n", v.Key)
				}
			}

			label, _ := LabelForLiteTokenAccount(string(v.Key))
			v.Key = []byte(label)

			privKey := ed25519.PrivateKey(v.Value)
			pubKey := privKey.Public().(ed25519.PublicKey)
			out += fmt.Sprintf("Converting %s : %x\n", v.Key, pubKey)

			err = GetWallet().Put(BucketLabel, v.Key, pubKey)
			if err != nil {
				return "", err
			}
			err = GetWallet().Put(BucketKeys, pubKey, privKey)
			if err != nil {
				return "", err
			}
			err = GetWallet().DeleteBucket(BucketAnon)
			if err != nil {
				return "", err
			}
		}
	}

	//fix the labels... there can be only one key one label.
	//should not have multiple labels to the same public key
	labelz, err := GetWallet().GetBucket(BucketLabel)
	if err != nil {
		//nothing to do...
		return
	}
	for _, v := range labelz.KeyValueList {
		label, isLite := LabelForLiteTokenAccount(string(v.Key))
		if isLite {
			//if we get here, then that means we have a bogus label.
			bogusLiteLabel := string(v.Key)
			//so check to see if it is in our regular key bucket
			otherPubKey, err := GetWallet().Get(BucketLabel, []byte(label))
			if err != nil {
				//key isn't found, so let's add it
				out += fmt.Sprintf("Converting %s to %s : %x\n", v.Key, label, v.Value)
				//so it doesn't exist, map the good label to the public key
				err = GetWallet().Put(BucketLabel, []byte(label), v.Value)
				if err != nil {
					return "", err
				}

				//now delete the bogus label
				err = GetWallet().Delete(BucketLabel, []byte(bogusLiteLabel))
				if err != nil {
					return "", err
				}
			} else {
				//ok so it does exist, now need to know if public key is the same, it is
				//an error if they don't match so warn user
				if !bytes.Equal(v.Value, otherPubKey) {
					out += fmt.Sprintf("public key stored for %v, doesn't match what is expected for a lite account: %s (%x != %x)\n",
						bogusLiteLabel, label, v.Value, otherPubKey)
				} else {
					//key isn't found, so let's add it
					out += fmt.Sprintf("Removing duplicate %s / %s : %x\n", v.Key, label, v.Value)
					//now delete the bogus label
					err = GetWallet().Delete(BucketLabel, []byte(bogusLiteLabel))
					if err != nil {
						return "", err
					}
				}
			}
		}
	}

	//build the map of lite accounts to key labels
	labelz, err = GetWallet().GetBucket(BucketLabel)
	if err != nil {
		//nothing to do...
		return
	}
	for _, v := range labelz.KeyValueList {
		k := new(Key)

		var err error
		k.PrivateKey, err = GetWallet().Get(BucketKeys, v.Value)
		if err != nil {
			return "", fmt.Errorf("private key not found for %x", v.Value)
		}

		k.PublicKey = v.Value

		//check to see if the key type has been assigned, if not set it to the ed25519Legacy...
		sigTypeData, err := GetWallet().Get(BucketSigTypeDeprecated, v.Value)
		if err != nil {
			//if it doesn't exist then 1) we are already using BucketKeyInfo, so we're golden, or 2)
			//we are an old wallet, so default it to ed25519 signature types

			//first, test for 1)
			kid, err := GetWallet().Get(BucketKeyInfo, v.Value)
			if err != nil {
				//ok, so it is 2), wo we need to make the key info bucket
				k.KeyInfo.Type = protocol.SignatureTypeED25519
				k.KeyInfo.Derivation = "external"

				//add the default key type
				out += fmt.Sprintf("assigning default key type %s for key name %v\n", k.KeyInfo.Type, string(v.Key))
				kiData, err := k.KeyInfo.MarshalBinary()
				if err != nil {
					return "", err
				}

				err = GetWallet().Put(BucketKeyInfo, v.Value, kiData)
				if err != nil {
					return "", err
				}
			} else {
				//we have key info, so, make sure it isn't a legacyEd25519 key and
				// assign it to key's key info.  If it is a legacyEd25519 then
				// update it to the regular accumulate ed25519 type
				err = k.KeyInfo.UnmarshalBinary(kid)
				if err != nil {
					return "", err
				}
				//if we hae the old key type, make it the new type.
				if k.KeyInfo.Type == protocol.SignatureTypeLegacyED25519 {
					k.KeyInfo.Type = protocol.SignatureTypeED25519

					kiData, err := k.KeyInfo.MarshalBinary()
					if err != nil {
						return "", err
					}

					err = GetWallet().Put(BucketKeyInfo, v.Value, kiData)
					if err != nil {
						return "", err
					}
				}
			}
		} else {
			//we have some old data in the bucket, so move it to the new bucket.
			common.BytesUint64(sigTypeData)
			kt, err := encoding.UvarintUnmarshalBinary(sigTypeData)
			if err != nil {
				return "", err
			}

			k.KeyInfo.Type.SetEnumValue(kt)

			if k.KeyInfo.Type == protocol.SignatureTypeLegacyED25519 {
				k.KeyInfo.Type = protocol.SignatureTypeED25519
			}
			k.KeyInfo.Derivation = "external"

			//add the default key type
			out += fmt.Sprintf("assigning default key type %s for key name %v\n", k.KeyInfo.Type, string(v.Key))

			kiData, err := k.KeyInfo.MarshalBinary()
			if err != nil {
				return "", err
			}

			err = GetWallet().Put(BucketKeyInfo, v.Value, kiData)
			if err != nil {
				return "", err
			}
		}

		liteAccount, err := protocol.LiteTokenAddressFromHash(k.PublicKeyHash(), protocol.ACME)
		if err != nil {
			return "", err
		}

		liteLabel, _ := LabelForLiteTokenAccount(liteAccount.String())
		_, err = GetWallet().Get(BucketLite, []byte(liteLabel))
		if err == nil {
			continue
		}

		out += fmt.Sprintf("lite identity %v mapped to key name %v\n", liteLabel, string(v.Key))

		err = GetWallet().Put(BucketLite, []byte(liteLabel), v.Key)
		if err != nil {
			return "", err
		}
	}

	_, err = GetWallet().GetBucket(BucketSigTypeDeprecated)
	if err == nil {
		err = GetWallet().DeleteBucket(BucketSigTypeDeprecated)
		if err != nil {
			return "", fmt.Errorf("error removing %s, %v", BucketSigTypeDeprecated, err)
		}
	}

	//update wallet version
	err = GetWallet().PutRaw(db.BucketConfig, []byte("version"), db.WalletVersion.Bytes())
	if err != nil {
		return "", err
	}

	return out, nil
}
