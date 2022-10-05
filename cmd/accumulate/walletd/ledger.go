package walletd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/FactomProject/go-bip32"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/bip44"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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

func NewLedgerApi(debugLogging bool) (*LedgerApi, error) {
	hub, err := usbwallet.NewLedgerHub(debugLogging)
	if err != nil {
		return nil, err
	}

	return &LedgerApi{
		hub: hub,
	}, nil
}

func NewLedgerSigner(key *Key) (*LedgerSigner, error) {
	if key.KeyInfo.WalletID == nil {
		return nil, fmt.Errorf("the given key is not on a Ledger device")
	}

	ledgerApi, err := NewLedgerApi(false) // FIXME
	if err != nil {
		return nil, err
	}

	walletID := *key.KeyInfo.WalletID
	selWallet, err := ledgerApi.SelectWallet(walletID.String())
	if err != nil {
		return nil, err
	}

	signer := &LedgerSigner{
		ledgerApi: ledgerApi,
		key:       key,
		wallet:    selWallet,
	}
	return signer, nil
}

func (m *JrpcMethods) LedgerQueryWallets(_ context.Context, params json.RawMessage) interface{} {
	resp := &api.LedgerWalletResponse{}
	hub, err := NewLedgerApi(false)
	if err != nil {
		return validatorError(err)
	}
	resp.LedgerWalletsInfo, err = hub.QueryWallets()
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (la *LedgerApi) QueryWallets() ([]api.LedgerWalletInfo, error) {
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
	walletOpen := false
	status := "ok"
	err := wallet.Open("")
	if err == nil {
		walletOpen = true
	} else {
		status = err.Error()
	}

	defer func() {
		if walletOpen {
			wallet.Close()
		}
	}()

	info := wallet.Info()
	ledgerInfo := &api.LedgerWalletInfo{
		WalletID: info.WalletID,
		Version: api.LedgerVersion{
			Label: fmt.Sprintf("%d.%d.%d", info.AppVersion.Major, info.AppVersion.Minor, info.AppVersion.Patch),
			Major: uint64(info.AppVersion.Major),
			Minor: uint64(info.AppVersion.Minor),
			Patch: uint64(info.AppVersion.Patch),
		},
		VendorID:     uint64(info.DeviceInfo.VendorID),
		Manufacturer: info.DeviceInfo.Manufacturer,
		ProductID:    uint64(info.DeviceInfo.ProductID),
		Product:      info.DeviceInfo.Product,
		Status:       status,
	}
	return ledgerInfo, nil
}

func (la *LedgerApi) Wallets() []accounts.Wallet {
	return la.hub.Wallets()
}

func (m *JrpcMethods) LedgerGenerateKey(_ context.Context, params json.RawMessage) interface{} {
	req := &api.GenerateLedgerKeyRequest{}
	err := json.Unmarshal(params, req)
	if err != nil {
		return validatorError(err)
	}

	ledgerApi, err := NewLedgerApi(false)
	if err != nil {
		return validatorError(err)
	}

	selWallet, err := ledgerApi.SelectWallet(req.WalletID)
	if err != nil {
		return validatorError(err)
	}

	keyData, err := ledgerApi.GenerateKey(selWallet, req.KeyLabel)
	if err != nil {
		return validatorError(err)
	}
	return keyData

}

func (la *LedgerApi) GenerateKey(wallet accounts.Wallet, label string) (*api.KeyData, error) {
	// make sure the key name doesn't already exist
	k := new(Key)
	err := k.LoadByLabel(label)
	if err == nil {
		return nil, fmt.Errorf("key already exists for key name %s", label)
	}

	// Open wallet, derive new key & save it
	err = wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		wallet.Close()
	}()

	derivation, err := bip44.NewDerivationPath(protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	address, err := getKeyCountAndIncrement(protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	chain := derivation.Chain()
	if chain&bip32.FirstHardenedChild == 0 {
		chain ^= bip32.FirstHardenedChild
		address ^= bip32.FirstHardenedChild
	}
	derivation = bip44.Derivation{derivation.Purpose(), derivation.CoinType(), derivation.Account(), chain, address}
	account, err := wallet.Derive(derivation, false, true, label)
	if err != nil {
		return nil, fmt.Errorf("derive function failed on ledger wallet: %v", err)
	}
	derivationPath, err := derivation.ToPath()
	walletID := wallet.WalletID()
	if wallet == nil {
		return nil, fmt.Errorf("could not derive wallet ID: %v", err)
	}

	key := &Key{
		Key: api.Key{PublicKey: account.PubKey,
			PrivateKey: nil,
			KeyInfo: api.KeyInfo{
				Type:       account.SignatureType,
				Derivation: derivationPath,
				WalletID:   walletID,
			}},
	}

	if err != nil {
		return nil, err
	}

	err = key.Save(label, account.LiteAccount.Authority)
	if err != nil {
		return nil, err
	}

	return &api.KeyData{
		Name:      label,
		PublicKey: key.PublicKey,
		KeyInfo: api.KeyInfo{
			Type:       account.SignatureType,
			Derivation: derivationPath,
			WalletID:   walletID,
		},
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
		return nil, fmt.Errorf("there is more than wallets available (%d), please use the --wallet-id flag to select the correct wallet", walletCnt)
	}

	var selWallet accounts.Wallet
	widUrl, err := url.Parse(walletID)
	if err != nil {
		for _, wallet := range wallets {
			wid := wallet.WalletID()
			if wid.Equal(widUrl) {
				selWallet = wallet
				break
			}
		}
	} else if walletIdx, err := strconv.Atoi(walletID); err == nil {
		if len(wallets) < walletIdx {
			return nil, fmt.Errorf("wallet with index number %d is not available", walletIdx)
		}
		selWallet = wallets[walletIdx-1]
	}

	if selWallet == nil {
		return nil, fmt.Errorf("No wallet with ID %s could be found on an active Ledger device.\nPlease use the \"accumulate ledger info\" command to identify "+
			"the connected wallets, make sure the Accumulate app is ready and the device is not locked.", walletID)
	}
	return selWallet, nil
}

func (la *LedgerApi) Sign(wallet accounts.Wallet, derivationPath string, txn *protocol.Transaction, sig *protocol.ED25519Signature) (*protocol.Signature, error) {
	err := wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		wallet.Close()
	}()

	hd := bip44.Derivation{}
	err = hd.FromPath(derivationPath)
	if err != nil {
		return nil, err
	}
	account, err := wallet.Derive(hd, true, false)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(account.PubKey, sig.PublicKey) {
		tx, err := wallet.SignTx(&account, txn, sig)
		return &tx, err
	}

	return nil, fmt.Errorf("the request key is was not found in the wallet (the wallet ID is %s)", wallet.WalletID())
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
		_, err := ls.ledgerApi.Sign(ls.wallet, ls.key.KeyInfo.Derivation, txn, sig)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("cannot sign using a %T, not supported", sig)
	}
	return nil
}

func (ls *LedgerSigner) Sign(protocol.Signature, []byte, []byte) error {
	return fmt.Errorf("only SignTransaction is supported on ledger devices")
}
