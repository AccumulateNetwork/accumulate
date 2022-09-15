package walletd

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	accounts "gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate"
	"gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate/usbwallet"
)

type LedgerHub struct {
	*usbwallet.Hub
}

func NewLedgerHub() (*LedgerHub, error) {
	hub, err := usbwallet.NewLedgerHub()
	if err != nil {
		return nil, err
	}

	return &LedgerHub{
		Hub: hub,
	}, nil
}

func (m *JrpcMethods) GetLedgerInfo(_ context.Context, params json.RawMessage) interface{} {
	resp := &api.LedgerWalletInfoResponse{}
	hub, err := NewLedgerHub()
	if err != nil {
		return validatorError(err)
	}
	resp.LedgerWalletsInfo, err = hub.QueryLedgerWalletsInfo()
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (h *LedgerHub) QueryLedgerWalletsInfo() ([]api.LedgerWalletInfo, error) {
	wallets := h.Wallets()
	var ledgerInfos []api.LedgerWalletInfo
	for _, wallet := range wallets {
		info, err := h.queryLedgerInfo(wallet)
		if err != nil {
			return nil, err
		}
		ledgerInfos = append(ledgerInfos, *info)
	}
	return ledgerInfos, nil
}

func (h *LedgerHub) queryLedgerInfo(wallet accounts.Wallet) (*api.LedgerWalletInfo, error) {
	err := wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		//	wallet.Close() This freezes up
	}()

	info := wallet.Info()
	return &api.LedgerWalletInfo{
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
