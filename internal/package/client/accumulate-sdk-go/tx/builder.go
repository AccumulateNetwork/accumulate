package txn

import (
	"fmt"

	sdk "github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go"
	"github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go/tx/signer"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type wrapper struct {
	txn *Txn
}

func newBuilder() *wrapper {
	return &wrapper{
		txn: &Txn{
			TxBody: &Envelope{},
	
		},
	}

}

func (w *wrapper) ValidateBasic() error {
	return w.txn.ValidateBasic()
}


func (w *wrapper) GetSigners() []sdk.AccAddress {
	return w.txn.GetSigners()
}

func (w *wrapper) GetPublicKeys() []*transactions.ED25519Sig {

	return w.txn.TxBody.Signatures
}

func (w *wrapper) GetSignatures() []*transactions.ED25519Sig {
	return w.txn.TxBody.Signatures
}

func (w *wrapper) GetSignatureV1() ([]signing.SignatureV1, error) {
	sigs := w.txn.TxBody.Signatures
//	pubkeys := w.GetPublicKeys()

	res := make([]signing.SignatureV1, len(sigs))

	return res, nil

}


func (w *wrapper) SetSignature(signatures ...signing.SignatureV1) error {
	n := len(signatures)
	signerInfo := make([]signing.SignatureV1, n)
	rawSig := make([]*transactions.ED25519Sig, n)

	for i, sig := range signatures {
		signerInfo[i] = signing.SignatureV1{
			Data: sig.Data,

		}
		fmt.Println(sig)
	}
//	w.setSignatureInfos(signerInfo)
	w.setSignatures(rawSig)

	return nil
}

/*
func (w *wrapper) setSignatureInfos(infos []*transactions.SignatureInfo) {
	w.txn.SigInfo = infos
}
*/

func (w *wrapper) setSignatures(sigs []*transactions.ED25519Sig) {
	w.txn.TxBody.Signatures = sigs
}

func (w *wrapper) SetMsgs(msgs ...sdk.Msg) error {
	if len(msgs) == 0 {
		return fmt.Errorf("no messages")
	}
//	w.txn.TxBody.Transaction.Header := msgs[0]
	return nil
}

func (w *wrapper) GetTxn() sdk.Txn {
	return w.txn
}

func (w *wrapper) GetMsgs() []sdk.Msg {
	return w.txn.GetMsgs()
}