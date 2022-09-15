package walletd

import (
	"context"
	"encoding/json"
	"gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
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
	resp.LedgerWalletsInfo, err = hub.GetLedgerWalletsInfo()
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (h *LedgerHub) GetLedgerWalletsInfo() ([]api.LedgerWalletInfo, error) {
	wallets := h.Wallets()
	var ledgerInfos []api.LedgerWalletInfo
	for _, wallet := range wallets {
		info, err := h.getLedgerInfo(wallet)
		if err != nil {
			return nil, err
		}
		ledgerInfos = append(ledgerInfos, *info)
	}
	return ledgerInfos, nil
}

func (h *LedgerHub) getLedgerInfo(wallet accounts.Wallet) (*api.LedgerWalletInfo, error) {
	err := wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		//	wallet.Close() This freezes up
	}()

	info := wallet.Info()
	//info.DeviceInfo.ProductID
	return &api.LedgerWalletInfo{
		Version: info.AppVersion,
	}, nil
}
