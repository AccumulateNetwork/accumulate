package cmd

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"reflect"
)

func PrintJsonRpcError(err error) (string, error) {
	var e jsonrpc2.Error
	switch err := err.(type) {
	case jsonrpc2.Error:
		e = err
	default:
		return "", fmt.Errorf("error with request, %v", err)
	}

	if WantJsonOutput {
		out, err := json.Marshal(e)
		if err != nil {
			return "", err
		}
		return "", &JsonRpcError{Err: e, Msg: string(out)}
	} else {
		var out string
		out += fmt.Sprintf("\n\tMessage\t\t:\t%v\n", e.Message)
		out += fmt.Sprintf("\tError Code\t:\t%v\n", e.Code)
		out += fmt.Sprintf("\tDetail\t\t:\t%s\n", e.Data)
		return "", &JsonRpcError{Err: e, Msg: out}
	}
}

func printOutput(cmd *cobra.Command, out string, err error) {
	if err != nil {
		if WantJsonOutput {
			cmd.PrintErrf("{\"error\":%v}", err)
		} else {
			cmd.PrintErrf("Error: %v\n", err)
		}
		DidError = err
	} else {
		cmd.Println(out)
	}
}

//nolint:gosimple
func printGeneralTransactionParameters(res *api2.TransactionQueryResponse) string {
	out := fmt.Sprintf("---\n")
	out += fmt.Sprintf("  - Transaction           : %x\n", res.TransactionHash)
	out += fmt.Sprintf("  - Signer Url            : %s\n", res.Origin)
	out += fmt.Sprintf("  - Signatures            :\n")
	for _, book := range res.SignatureBooks {
		for _, page := range book.Pages {
			out += fmt.Sprintf("  - Signatures            :\n")
			out += fmt.Sprintf("    - Signer              : %s (%v)\n", page.Signer.Url, page.Signer.Type)
			for _, sig := range page.Signatures {
				if sig.Type().IsSystem() {
					out += fmt.Sprintf("      -                   : %v\n", sig.Type())
				} else {
					out += fmt.Sprintf("      -                   : %x (sig) / %x (key)\n", sig.GetSignature(), sig.GetPublicKey())
				}
			}
		}
	}
	out += fmt.Sprintf("===\n")
	return out
}

func PrintJson(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func PrintChainQueryResponseV2(res *QueryResponse) (string, error) {
	if WantJsonOutput || res.Type == "dataEntry" {
		return PrintJson(res)
	}

	out, err := outputForHumans(res)
	if err != nil {
		return "", err
	}

	for i, txid := range res.SyntheticTxids {
		out += fmt.Sprintf("  - Synthetic Transaction %d : %x\n", i, txid)
	}
	return out, nil
}

func PrintTransactionQueryResponseV2(res *api2.TransactionQueryResponse) (string, error) {
	if WantJsonOutput {
		return PrintJson(res)
	}

	out, err := outputForHumansTx(res)
	if err != nil {
		return "", err
	}

	for i, txid := range res.SyntheticTxids {
		out += fmt.Sprintf("  - Synthetic Transaction %d : %x\n", i, txid)
	}

	for _, receipt := range res.Receipts {
		// // TODO Figure out how to include the directory receipt and block
		// out += fmt.Sprintf("Receipt from %v#chain/%s in block %d\n", receipt.Account, receipt.Chain, receipt.DirectoryBlock)
		out += fmt.Sprintf("Receipt from %v#chain/%s\n", receipt.Account, receipt.Chain)
		if receipt.Error != "" {
			out += fmt.Sprintf("  Error!! %s\n", receipt.Error)
		}
		if !receipt.Receipt.Convert().Validate() {
			//nolint:gosimple
			out += fmt.Sprintf("  Invalid!!\n")
		}
	}

	return out, nil
}

func PrintMinorBlockQueryResponseV2(res *api2.MinorQueryResponse) (string, error) {
	if WantJsonOutput {
		return PrintJson(res)
	}

	out := fmt.Sprintf("--- block #%d, blocktime %v:\n", res.BlockIndex, res.BlockTime)
	out += fmt.Sprintf("    txid: %s\n", hex.EncodeToString(res.TransactionHash))
	tr, err := PrintTransactionQueryResponseV2(&res.TransactionQueryResponse)
	if err != nil {
		return "", err
	}
	out += tr
	return out, nil
}

func PrintMultiResponse(res *api2.MultiResponse) (string, error) {
	if WantJsonOutput || res.Type == "dataSet" {
		return PrintJson(res)
	}

	var out string
	switch res.Type {
	case "directory":
		out += fmt.Sprintf("\n\tADI Entries: start = %d, count = %d, total = %d\n", res.Start, res.Count, res.Total)

		if len(res.OtherItems) == 0 {
			for _, s := range res.Items {
				out += fmt.Sprintf("\t%v\n", s)
			}
			return out, nil
		}

		for _, s := range res.OtherItems {
			qr := new(api2.ChainQueryResponse)
			var data json.RawMessage
			qr.Data = &data
			err := Remarshal(s, qr)
			if err != nil {
				return "", err
			}

			account, err := protocol.UnmarshalAccountJSON(data)
			if err != nil {
				return "", err
			}

			chainDesc := account.GetType().String()
			if err == nil {
				if v, ok := ApiToString[account.GetType()]; ok {
					chainDesc = v
				}
			}
			out += fmt.Sprintf("\t%v (%s)\n", account.Header().Url, chainDesc)
		}
	case "pending":
		out += fmt.Sprintf("\n\tPending Tranactions -> Start: %d\t Count: %d\t Total: %d\n", res.Start, res.Count, res.Total)
		for i, item := range res.Items {
			out += fmt.Sprintf("\t%d\t%s", i, item)
		}
	case "txHistory":
		out += fmt.Sprintf("\n\tTransaction History Start: %d\t Count: %d\t Total: %d\n", res.Start, res.Count, res.Total)
		for i := range res.Items {
			// Convert the item to a transaction query response
			txr := new(api2.TransactionQueryResponse)
			err := Remarshal(res.Items[i], txr)
			if err != nil {
				return "", err
			}

			s, err := PrintTransactionQueryResponseV2(txr)
			if err != nil {
				return "", err
			}
			out += s
		}
	case "minorBlock":
		out += fmt.Sprint("\n\n==========================================================================\n")
		out += fmt.Sprintf("\tMinor block result Start: %d\t Count: %d\t Total: %d\n", res.Start, res.Count, res.Total)
		for i := range res.Items {
			out += fmt.Sprint("\n\n==========================================================================\n")
			// Convert the item to a transaction query response
			mtr := new(api2.MinorQueryResponse)
			err := Remarshal(res.Items[i], mtr)
			if err != nil {
				return "", err
			}

			s, err := PrintMinorBlockQueryResponseV2(mtr)
			if err != nil {
				return "", err
			}
			out += s
		}
	}

	return out, nil
}

//nolint:gosimple
func outputForHumans(res *QueryResponse) (string, error) {
	switch string(res.Type) {
	case protocol.AccountTypeLiteTokenAccount.String():
		ata := protocol.LiteTokenAccount{}
		err := Remarshal(res.Data, &ata)
		if err != nil {
			return "", err
		}

		amt, err := formatAmount(ata.TokenUrl.String(), &ata.Balance)
		if err != nil {
			amt = "unknown"
		}

		var out string
		out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
		out += fmt.Sprintf("\tToken Url\t:\t%v\n", ata.TokenUrl)
		out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)

		out += fmt.Sprintf("\tCredits\t\t:\t%d\n", protocol.CreditPrecision*ata.CreditBalance)
		out += fmt.Sprintf("\tLast Used On\t\t:\t%d\n", ata.LastUsedOn)
		return out, nil
	case protocol.AccountTypeTokenAccount.String():
		ata := protocol.TokenAccount{}
		err := Remarshal(res.Data, &ata)
		if err != nil {
			return "", err
		}

		amt, err := formatAmount(ata.TokenUrl.String(), &ata.Balance)
		if err != nil {
			amt = "unknown"
		}

		var out string
		out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
		out += fmt.Sprintf("\tToken Url\t:\t%s\n", ata.TokenUrl)
		out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)
		out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", ata.KeyBook)

		return out, nil
	case protocol.AccountTypeIdentity.String():
		adi := protocol.ADI{}
		err := Remarshal(res.Data, &adi)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tADI url\t\t:\t%v\n", adi.Url)
		out += fmt.Sprintf("\tKey Book url\t:\t%s\n", adi.KeyBook)

		return out, nil
	case protocol.AccountTypeKeyBook.String():
		book := protocol.KeyBook{}
		err := Remarshal(res.Data, &book)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tPage Count\n")
		out += fmt.Sprintf("\t%d\n", book.PageCount)
		return out, nil
	case protocol.AccountTypeKeyPage.String():
		ss := protocol.KeyPage{}
		err := Remarshal(res.Data, &ss)
		if err != nil {
			return "", err
		}

		out := fmt.Sprintf("\n\tCredit Balance\t:\t%.2f\n", float64(ss.CreditBalance)/protocol.CreditPrecision)
		out += fmt.Sprintf("\n\tIndex\tNonce\t\tPublic Key\t\t\t\t\t\t\t\tKey Name\n")
		for i, k := range ss.Keys {
			keyName := ""
			name, err := FindLabelFromPubKey(k.PublicKeyHash)
			if err == nil {
				keyName = name
			}
			out += fmt.Sprintf("\t%d\t%d\t\t%x\t%s\n", i, k.LastUsedOn, k.PublicKeyHash, keyName)
		}
		return out, nil
	case "token", protocol.AccountTypeTokenIssuer.String():
		ti := protocol.TokenIssuer{}
		err := Remarshal(res.Data, &ti)
		if err != nil {
			return "", err
		}

		out := fmt.Sprintf("\n\tToken URL\t:\t%s", ti.Url)
		out += fmt.Sprintf("\n\tSymbol\t\t:\t%s", ti.Symbol)
		out += fmt.Sprintf("\n\tPrecision\t:\t%d", ti.Precision)
		if ti.SupplyLimit != nil {
			out += fmt.Sprintf("\n\tSupply Limit\t\t:\t%s", amountToString(ti.Precision, ti.SupplyLimit))
		}
		out += fmt.Sprintf("\n\tTokens Issued\t:\t%s", amountToString(ti.Precision, &ti.Issued))
		out += fmt.Sprintf("\n\tProperties URL\t:\t%s", ti.Properties)
		out += "\n"
		return out, nil
	default:
		return printReflection("", "", reflect.ValueOf(res.Data)), nil
	}
}

func outputForHumansTx(res *api2.TransactionQueryResponse) (string, error) {
	typStr := res.Data.(map[string]interface{})["type"].(string)
	typ, ok := protocol.TransactionTypeByName(typStr)
	if !ok {
		return "", fmt.Errorf("Unknown transaction type %s", typStr)
	}

	if typ == protocol.TransactionTypeSendTokens {
		txn := new(api2.TokenSend)
		err := Remarshal(res.Data, txn)
		if err != nil {
			return "", err
		}

		tx := txn
		var out string
		for i := range tx.To {
			amt, err := formatAmount("acc://ACME", &tx.To[i].Amount)
			if err != nil {
				amt = "unknown"
			}
			out += fmt.Sprintf("Send %s from %s to %s\n", amt, res.Origin, tx.To[i].Url)
			out += fmt.Sprintf("  - Synthetic Transaction : %x\n", tx.To[i].Txid)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil
	}

	txn, err := protocol.NewTransactionBody(typ)
	if err != nil {
		return "", err
	}

	err = Remarshal(res.Data, txn)
	if err != nil {
		return "", err
	}

	out := fmt.Sprintf("    type: %s\n", typStr)

	switch txn := txn.(type) {
	case *protocol.SyntheticDepositTokens:
		deposit := txn
		out += "\n"
		amt, err := formatAmount(deposit.Token.String(), &deposit.Amount)
		if err != nil {
			amt = "unknown"
		}
		out += fmt.Sprintf("Receive %s to %s (cause: %X)\n", amt, res.Origin, deposit.Cause)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.SyntheticDepositCredits:
		deposit := txn
		out += "\n"
		out += fmt.Sprintf("Source \t:%s\n", deposit.Source)
		out += fmt.Sprintf("Credits \t:%d\n", deposit.Amount)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.SyntheticCreateChain:
		scc := txn
		for _, cp := range scc.Chains {
			c, err := protocol.UnmarshalAccount(cp.Data)
			if err != nil {
				return "", err
			}
			// unmarshal
			verb := "Created"
			if cp.IsUpdate {
				verb = "Updated"
			}
			out += fmt.Sprintf("%s %v (%v)\n", verb, c.Header().Url, c.GetType())
		}
		return out, nil
	case *protocol.CreateIdentity:
		id := txn
		out += "\n"
		out += fmt.Sprintf("ADI URL \t\t:\t%s\n", id.Url)
		out += fmt.Sprintf("Key Book URL\t\t:\t%s\n", id.KeyBookUrl)

		keyName, err := FindLabelFromPubKey(id.KeyHash)
		if err != nil {
			out += fmt.Sprintf("Public Key \t:\t%x\n", id.KeyHash)
		} else {
			out += fmt.Sprintf("Public Key (name) \t:\t%x (%s)\n", id.KeyHash, keyName)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.CreateDataAccount:
		dataAcc := txn
		out += "\n"
		out += fmt.Sprintf("ADI URL \t\t:\t%s\n", dataAcc.Url)
		out += fmt.Sprintf("Key Book URL\t\t:\t%s\n", dataAcc.KeyBookUrl)
		out += fmt.Sprintf("Manager Book URL\t\t:\t%s\n", dataAcc.ManagerKeyBookUrl)
		out += fmt.Sprintf("Is scratch account\t\t:\t%t\n", dataAcc.Scratch)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.CreateKeyBook:
		keyBook := txn
		out += "\n"
		out += fmt.Sprintf("ADI URL \t\t:\t%s\n", keyBook.Url)
		out += fmt.Sprintf("Key Book URL\t\t:\t%s\n", keyBook.PublicKeyHash)
		out += fmt.Sprintf("Manager Book URL\t\t:\t%s\n", keyBook.Manager)

		keyName, err := FindLabelFromPubKey(keyBook.PublicKeyHash)
		if err != nil {
			out += fmt.Sprintf("Public Key \t:\t%x\n", keyBook.PublicKeyHash)
		} else {
			out += fmt.Sprintf("Public Key (name) \t:\t%x (%s)\n", keyBook.PublicKeyHash, keyName)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.CreateKeyPage:
		page := txn
		out += "\n"
		out += fmt.Sprintf("Manager URL\t\t:\t%s\n", page.Manager)

		for _, key := range page.Keys {
			keyName, err := FindLabelFromPubKey(key.KeyHash)
			if err != nil {
				out += fmt.Sprintf("Public Key \t:\t%x\n", key.KeyHash)
			} else {
				out += fmt.Sprintf("Public Key (name) \t:\t%x (%s)\n", key.KeyHash, keyName)
			}
			out += fmt.Sprintf("Key owner \t:\t%s\n", key.Owner)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.SyntheticAnchor:
		synthAnchor := txn
		out += "\n"
		out += fmt.Sprintf("Block \t\t\t:\t%d\n", synthAnchor.Block)
		out += fmt.Sprintf("ACME burnt \t\t:\t%d\n", synthAnchor.AcmeBurnt.Int64())
		out += fmt.Sprintf("ACME oracle price \t:\t%d\n", synthAnchor.AcmeOraclePrice)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.UpdateKeyPage:
		page := txn
		out += "\n"
		for _, op := range page.Operation {
			out += fmt.Sprintf("Type \t:\t%v\n", op.Type())
			// TODO switch operation types
		}
		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.SyntheticReceipt:
		synthRcpt := txn
		out += "\n"
		out += fmt.Sprintf("Source \t\t:%s\n", synthRcpt.Source)
		out += fmt.Sprintf("SynthTxHash \t:\t%s\n", hex.EncodeToString(synthRcpt.SynthTxHash[:]))
		strStatus := "" // TODO refine
		if synthRcpt.Status.Code > 0 {
			strStatus = "error: " + synthRcpt.Status.Message
		} else if synthRcpt.Status.Delivered {
			strStatus = "delivered"
		} else if synthRcpt.Status.Pending {
			strStatus = "pending"
		}
		out += fmt.Sprintf("Status \t\t:\t%s\n", strStatus)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.SyntheticBurnTokens:
		burn := txn
		out += "\n"
		amt, err := formatAmount(burn.Amount.String(), &burn.Amount)
		if err != nil {
			amt = "unknown"
		}

		out += fmt.Sprintf("Source \t\t\t:\t%s\n", burn.Source)
		out += fmt.Sprintf("Amount \t\t\t:\t%s\n", amt)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.SegWitDataEntry:
		seg := txn
		out += "\n"
		out += fmt.Sprintf("Source \t\t\t:\t%s\n", seg.Source)
		out += fmt.Sprintf("EntryUrl \t\t\t:\t%s\n", seg.EntryUrl)
		out += fmt.Sprintf("EntryHash \t\t\t:\t%s\n", hex.EncodeToString(seg.EntryHash[:]))

		out += printGeneralTransactionParameters(res)
		return out, nil

	default:
		return printReflection("", "", reflect.ValueOf(txn)), nil
	}
}

func printReflection(field, indent string, value reflect.Value) string {
	typ := value.Type()
	out := fmt.Sprintf("%s%s:", indent, field)
	if field == "" {
		out = ""
	}

	if typ.AssignableTo(reflect.TypeOf(new(url2.URL))) {
		v := value.Interface().(*url2.URL)
		if v != nil {
			return out + " " + v.String() + "\n"
		}
	}

	if typ.AssignableTo(reflect.TypeOf(url2.URL{})) {
		v := value.Interface().(url2.URL)
		return out + " " + v.String() + "\n"
	}

	switch value.Kind() {
	case reflect.Ptr, reflect.Interface:
		if value.IsNil() {
			return ""
		}
		return printReflection(field, indent, value.Elem())
	case reflect.Slice, reflect.Array:
		out += "\n"
		for i, n := 0, value.Len(); i < n; i++ {
			out += printReflection(fmt.Sprintf("%d (elem)", i), indent+"   ", value.Index(i))
		}
		return out
	case reflect.Map:
		out += "\n"
		for iter := value.MapRange(); iter.Next(); {
			out += printReflection(fmt.Sprintf("%s (key)", iter.Key()), indent+"   ", iter.Value())
		}
		return out
	case reflect.Struct:
		out += "\n"
		out += fmt.Sprintf("%s   (type): %s\n", indent, natural(typ.Name()))

		callee := value
		m, ok := typ.MethodByName("Type")
		if !ok {
			m, ok = reflect.PtrTo(typ).MethodByName("Type")
			callee = value.Addr()
		}
		if ok && m.Type.NumIn() == 1 && m.Type.NumOut() == 1 {
			v := m.Func.Call([]reflect.Value{callee})[0]
			if _, ok := v.Type().MethodByName("GetEnumValue"); ok {
				out += fmt.Sprintf("%s   %s: %s\n", indent, natural(v.Type().Name()), natural(fmt.Sprint(v)))
			}
		}

		for i, n := 0, value.NumField(); i < n; i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			out += printReflection(f.Name, indent+"   ", value.Field(i))
		}
		return out
	default:
		return out + " " + fmt.Sprint(value) + "\n"
	}
}

func outputTransactionResultForHumans(t protocol.TransactionResult) string {
	var out string

	switch c := t.(type) {
	case *protocol.AddCreditsResult:
		amt, err := formatAmount(protocol.ACME, &c.Amount)
		if err != nil {
			amt = "unknown"
		}
		out += fmt.Sprintf("Oracle\t$%.2f / ACME\n", float64(c.Oracle)/protocol.AcmeOraclePrecision)
		out += fmt.Sprintf("\t\t\t\t  Credits\t%.2f\n", float64(c.Credits)/protocol.CreditPrecision)
		out += fmt.Sprintf("\t\t\t\t  Amount\t%s\n", amt)
	case *protocol.WriteDataResult:
		out += fmt.Sprintf("Account URL\t%s\n", c.AccountUrl)
		out += fmt.Sprintf("\t\t\t\t  Account ID\t%x\n", c.AccountID)
		out += fmt.Sprintf("\t\t\t\t  Entry Hash\t%x\n", c.EntryHash)
	}
	return out
}

func (a *ActionLiteDataResponse) Print() (string, error) {
	var out string
	if WantJsonOutput {
		ok := a.Code == "0" || a.Code == ""
		if ok {
			a.Code = "ok"
		}
		b, err := json.Marshal(a)
		if err != nil {
			return "", err
		}
		out = string(b)
	} else {
		s, err := a.ActionDataResponse.Print()
		if err != nil {
			return "", err
		}
		out = fmt.Sprintf("\n\tAccount Url\t\t:%s\n", a.AccountUrl[:])
		out += fmt.Sprintf("\n\tAccount Id\t\t:%x\n", a.AccountId[:])
		out += s[1:]
	}
	return out, nil
}

func (a *ActionDataResponse) Print() (string, error) {
	var out string
	if WantJsonOutput {
		ok := a.Code == "0" || a.Code == ""
		if ok {
			a.Code = "ok"
		}
		b, err := json.Marshal(a)
		if err != nil {
			return "", err
		}
		out = string(b)
	} else {
		s, err := a.ActionResponse.Print()
		if err != nil {
			return "", err
		}
		out = fmt.Sprintf("\n\tEntry Hash\t\t:%x\n%s", a.EntryHash[:], s[1:])
	}
	return out, nil
}

func (a *ActionResponse) Print() (string, error) {
	ok := a.Code == "0" || a.Code == ""

	var out string
	if WantJsonOutput {
		if ok {
			a.Code = "ok"
		}
		b, err := json.Marshal(a)
		if err != nil {
			return "", err
		}
		out = string(b)
	} else {
		out += fmt.Sprintf("\n\tTransaction Hash\t: %x\n", a.TransactionHash)
		for i, hash := range a.SignatureHashes {
			out += fmt.Sprintf("\tSignature %d Hash\t: %x\n", i, hash)
		}
		out += fmt.Sprintf("\tSimple Hash\t\t: %x\n", a.SimpleHash)
		if !ok {
			out += fmt.Sprintf("\tError code\t\t: %s\n", a.Code)
		} else {
			//nolint:gosimple
			out += fmt.Sprintf("\tError code\t\t: ok\n")
		}
		if a.Error != "" {
			out += fmt.Sprintf("\tError\t\t\t: %s\n", a.Error)
		}
		if a.Log != "" {
			out += fmt.Sprintf("\tLog\t\t\t: %s\n", a.Log)
		}
		if a.Codespace != "" {
			out += fmt.Sprintf("\tCodespace\t\t: %s\n", a.Codespace)
		}
		if a.Result != nil {
			out += "\tResult\t\t\t: "
			d, err := json.Marshal(a.Result.Result)
			if err != nil {
				out += fmt.Sprintf("error remarshaling result %v\n", a.Result.Result)
			} else {
				v, err := protocol.UnmarshalTransactionResultJSON(d)
				if err != nil {
					out += fmt.Sprintf("error unmarshaling transaction result %v", err)
				} else {
					out += outputTransactionResultForHumans(v)
				}
			}
		}
	}

	if ok {
		return out, nil
	}
	return "", errors.New(out)
}
