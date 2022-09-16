package walletd

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/bip44"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	accounts "gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate"
	"gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate/usbwallet"
)

type LedgerApi struct {
	hub *usbwallet.Hub
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

func (l *LedgerApi) QueryLedgerWalletsInfo() ([]api.LedgerWalletInfo, error) {
	wallets := l.hub.Wallets()
	var ledgerInfos []api.LedgerWalletInfo
	for _, wallet := range wallets {
		info, err := l.queryLedgerInfo(wallet)
		if err != nil {
			return nil, err
		}
		ledgerInfos = append(ledgerInfos, *info)
	}
	return ledgerInfos, nil
}

func (l *LedgerApi) queryLedgerInfo(wallet accounts.Wallet) (*api.LedgerWalletInfo, error) {
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

func (l *LedgerApi) Wallets() []accounts.Wallet {
	return l.hub.Wallets()
}

func (l *LedgerApi) GenerateKey(wallet accounts.Wallet, label string) (*api.KeyData, error) {
	walletID := wallet.URL()

	derivation, err := bip44.NewDerivationPath(protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	address, err := getKeyCountAndIncrement(protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	derivation = bip44.Derivation{derivation.Purpose(), derivation.CoinType(), derivation.Account(), derivation.Chain(), address}
	account, err := wallet.Derive(derivation, true)

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
