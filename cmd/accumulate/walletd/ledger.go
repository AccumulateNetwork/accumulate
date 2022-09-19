package walletd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/bip44"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	accounts "gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate"
	"gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate/usbwallet"
)

type LedgerApi struct {
	hub *usbwallet.Hub
}

type LedgerSigner struct {
	ledgerApi *LedgerApi
	key       *Key
	wallet    accounts.Wallet
}

func NewLedgerApi() (*LedgerApi, error) {
	hub, err := usbwallet.NewLedgerHub()
	if err != nil {
		return nil, err
	}

	return &LedgerApi{
		hub: hub,
	}, nil
}

func NewLedgerSigner(key *Key) (*LedgerSigner, error) {
	ledgerApi, err := NewLedgerApi()
	if err != nil {
		return nil, err
	}

	selWallet, err := ledgerApi.SelectWallet(key.KeyInfo.WalletID)
	if err != nil {
		return nil, err
	}

	signer := &LedgerSigner{
		ledgerApi: nil,
		key:       key,
		wallet:    selWallet,
	}
	return signer, nil
}

func (m *JrpcMethods) GetLedgerInfo(_ context.Context, params json.RawMessage) interface{} {
	resp := &api.LedgerWalletInfoResponse{}
	hub, err := NewLedgerApi()
	if err != nil {
		return validatorError(err)
	}
	resp.LedgerWalletsInfo, err = hub.QueryLedgerWalletsInfo()
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (la *LedgerApi) QueryLedgerWalletsInfo() ([]api.LedgerWalletInfo, error) {
	wallets := la.hub.Wallets()
	var ledgerInfos []api.LedgerWalletInfo
	for _, wallet := range wallets {
		info, err := la.queryLedgerInfo(wallet)
		if err != nil {
			return nil, err
		}
		ledgerInfos = append(ledgerInfos, *info)
	}
	return ledgerInfos, nil
}

func (la *LedgerApi) queryLedgerInfo(wallet accounts.Wallet) (*api.LedgerWalletInfo, error) {
	err := wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		//	wallet.Close() This freezes up
	}()

	info := wallet.Info()
	url := wallet.URL()
	return &api.LedgerWalletInfo{
		Url: url.String(),
		Version: api.Version{
			Label: fmt.Sprintf("%d.%d.%d", info.AppVersion.Major, info.AppVersion.Minor, info.AppVersion.Patch),
			Major: uint64(info.AppVersion.Major),
			Minor: uint64(info.AppVersion.Minor),
			Patch: uint64(info.AppVersion.Patch),
		},
		VendorID:     uint64(info.DeviceInfo.VendorID),
		Manufacturer: info.DeviceInfo.Manufacturer,
		ProductID:    uint64(info.DeviceInfo.ProductID),
		Product:      info.DeviceInfo.Product,
	}, nil
}

func (la *LedgerApi) Wallets() []accounts.Wallet {
	return la.hub.Wallets()
}

func (la *LedgerApi) GenerateKey(wallet accounts.Wallet, label string) (*api.KeyData, error) {
	walletID := wallet.URL()
	err := wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		wallet.Close() //This freezes up
	}()

	derivation, err := bip44.NewDerivationPath(protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	address, err := getKeyCountAndIncrement(protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	derivation = bip44.Derivation{derivation.Purpose(), derivation.CoinType(), derivation.Account(), derivation.Chain(), address}
	account, err := wallet.Derive(derivation, true, true)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("derive function failed on ledger wallet: %v", err))
	}

	derivationPath, err := derivation.ToPath()
	key := &Key{
		PublicKey:  account.PubKey,
		PrivateKey: nil,
		KeyInfo: KeyInfo{
			Type:       account.SignatureType,
			Derivation: derivationPath,
			WalletID:   walletID.String(),
		},
	}
	if err != nil {
		return nil, err
	}
	key.Save(label, account.LiteAccount.String())

	return &api.KeyData{
		Name:       label,
		PublicKey:  key.PublicKey,
		Derivation: derivationPath,
		KeyType:    account.SignatureType,
		WalletID:   walletID.String(),
	}, nil
}

func (la *LedgerApi) SelectWallet(walletID string) (accounts.Wallet, error) {
	wallets := la.Wallets()
	walletCnt := len(wallets)
	switch {
	case walletCnt == 1:
		return wallets[0], nil
	case walletCnt == 0:
		return nil, errors.New("no wallets found, please check if your wallet and the Accumulate app on it are online")
	case walletCnt > 1 && len(walletID) == 0:
		return nil, errors.New(
			fmt.Sprintf("there is more than wallets available (%d), please use the --wallet-id flag to select the correct wallet", walletCnt))
	}

	var selWallet accounts.Wallet
	for i, wallet := range wallets {
		if strings.HasPrefix(walletID, "ledger://") {
			wid := wallet.URL()
			if wid.String() == walletID {
				selWallet = wallet
				break
			}
		} else {
			if walletIdx, err := strconv.Atoi(walletID); err == nil {
				if walletIdx == i+1 {
					selWallet = wallet
					break
				}
			}
		}
	}
	if selWallet == nil {
		return nil, errors.New(
			fmt.Sprintf("no wallet with ID %s could be found, please use accumulate ledger info to identify the connected wallets", walletID))
	}
	return selWallet, nil
}

func (la *LedgerApi) Sign(wallet accounts.Wallet, txn *protocol.Transaction, sig *protocol.ED25519Signature) (*protocol.Signature, error) {
	err := wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		//	wallet.Close() This freezes up
	}()

	for _, account := range wallet.Keys() {
		if bytes.Equal(account.PubKey, sig.PublicKey) {

			tx, err := wallet.SignTx(&account, txn, sig)
			return &tx, err
		}
	}
	return nil, errors.New("the request key is was found in the wallet")
}

func (ls *LedgerSigner) SetPublicKey(sig protocol.Signature) error {
	switch sig := sig.(type) {
	case *protocol.ED25519Signature:
		sig.PublicKey = ls.key.PublicKey

	case *protocol.RCD1Signature:
		sig.PublicKey = ls.key.PublicKey
	default:
		return fmt.Errorf("cannot set the public key on a %T, not supported", sig)
	}
	return nil
}

func (ls *LedgerSigner) SignTransaction(sig protocol.Signature, txn *protocol.Transaction) error {
	switch sig := sig.(type) {
	case *protocol.ED25519Signature:
		_, err := ls.ledgerApi.Sign(ls.wallet, txn, sig)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("cannot set the public key on a %T, not supported", sig)
	}
	return nil
}

func (ls *LedgerSigner) Sign(protocol.Signature, []byte, []byte) error {
	return fmt.Errorf("ledgers only support SignTransaction")
}
