package cmd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

func printError(cmd *cobra.Command, err error) {
	if WantJsonOutput {
		cmd.PrintErrf("{\"error\":%v}", err)
	} else {
		cmd.PrintErrf("Error: %v\n", err)
	}
	DidError = err
}

func printGeneralTransactionParameters(_ io.Writer, res *api2.TransactionQueryResponse) string {
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
					out += fmt.Sprintf("      -                   : %x (sig) / %x (key)\n", sig.GetSignature(), sig.GetPublicKeyHash())
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

func FPrintJson(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	fmt.Fprint(w, string(data))
	return nil
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
	buf := new(bytes.Buffer)
	err := FPrintTransactionQueryResponseV2(buf, res)
	return buf.String(), err
}

func FPrintTransactionQueryResponseV2(w io.Writer, res *api2.TransactionQueryResponse) error {
	if WantJsonOutput {
		return FPrintJson(w, res)
	}

	err := outputForHumansTx(w, res)
	if err != nil {
		return err
	}

	for i, txid := range res.SyntheticTxids {
		fmt.Fprintf(w, "  - Synthetic Transaction %d : %x\n", i, txid)
	}

	for _, receipt := range res.Receipts {
		// // TODO Figure out how to include the directory receipt and block
		// out += fmt.Sprintf("Receipt from %v#chain/%s in block %d\n", receipt.Account, receipt.Chain, receipt.DirectoryBlock)
		fmt.Fprintf(w, "Receipt from %v#chain/%s\n", receipt.Account, receipt.Chain)
		if receipt.Error != "" {
			fmt.Fprintf(w, "  Error!! %s\n", receipt.Error)
		}
		if !receipt.Receipt.Convert().Validate() {
			//nolint:gosimple
			fmt.Fprintf(w, "  Invalid!!\n")
		}
	}

	return nil
}

func PrintMinorBlockQueryResponseV2(w io.Writer, res *api2.MinorQueryResponse) error {
	if WantJsonOutput {
		return FPrintJson(w, res)
	}

	fmt.Fprintf(w, "--- block #%d, blocktime %v:\n", res.BlockIndex, res.BlockTime)
	if res.TxCount > 0 {
		fmt.Fprintf(w, "    total tx count\t: %d\n", res.TxCount)
	}

	if len(res.Transactions) > 0 {
		for _, tx := range res.Transactions {
			if tx.Data != nil {
				err := FPrintTransactionQueryResponseV2(w, tx)
				if err != nil {
					return err
				}
			}
		}
	} else {
		for i, txid := range res.TxIds {
			fmt.Fprintf(w, "    txid #%d\t\t: %s\n", i+1, hex.EncodeToString(txid))
		}
	}
	return nil
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

			chainDesc := account.Type().String()
			if err == nil {
				if v, ok := ApiToString[account.Type()]; ok {
					chainDesc = v
				}
			}
			out += fmt.Sprintf("\t%v (%s)\n", account.GetUrl(), chainDesc)
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
	}

	return out, nil
}

func FPrintMultiResponse(w io.Writer, res *api2.MultiResponse) error {
	if WantJsonOutput || res.Type == "dataSet" {
		return FPrintJson(w, res)
	}

	switch res.Type {
	case "minorBlock":
		fmt.Fprintf(w, "\n\tMinor block result Start: %d\t Count: %d\t Total: %d\n", res.Start, res.Count, res.Total)
		for i := range res.Items {
			fmt.Fprintln(w, "==========================================================================")

			// Convert the item to a transaction query response
			mtr := new(api2.MinorQueryResponse)
			err := Remarshal(res.Items[i], mtr)
			if err != nil {
				return err
			}

			err = PrintMinorBlockQueryResponseV2(w, mtr)
			if err != nil {
				return err
			}
			fmt.Fprintln(w)
		}
	}

	return nil
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
		out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", ata.KeyBook().String())

		return out, nil
	case protocol.AccountTypeIdentity.String():
		adi := protocol.ADI{}
		err := Remarshal(res.Data, &adi)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tADI url\t\t:\t%v\n", adi.Url)
		out += fmt.Sprintf("\tKey Book url\t:\t%s\n", adi.KeyBook().String())

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

func outputForHumansTx(w io.Writer, res *api2.TransactionQueryResponse) error {
	typStr := res.Data.(map[string]interface{})["type"].(string)
	typ, ok := protocol.TransactionTypeByName(typStr)
	if !ok {
		return fmt.Errorf("Unknown transaction type %s", typStr)
	}

	if typ == protocol.TransactionTypeSendTokens {
		txn := new(api2.TokenSend)
		err := Remarshal(res.Data, txn)
		if err != nil {
			return err
		}

		tx := txn
		for i := range tx.To {
			amt, err := formatAmount("acc://ACME", &tx.To[i].Amount)
			if err != nil {
				amt = "unknown"
			}
			fmt.Fprintf(w, "Send %s from %s to %s\n", amt, res.Origin, tx.To[i].Url)
			fmt.Fprintf(w, "  - Synthetic Transaction : %x\n", tx.To[i].Txid)
		}

		printGeneralTransactionParameters(w, res)
		return nil
	}

	txn, err := protocol.NewTransactionBody(typ)
	if err != nil {
		return err
	}

	err = Remarshal(res.Data, txn)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "    type: %s\n", typStr)

	switch txn := txn.(type) {
	case *protocol.SyntheticDepositTokens:
		deposit := txn
		fmt.Fprintln(w)
		amt, err := formatAmount(deposit.Token.String(), &deposit.Amount)
		if err != nil {
			amt = "unknown"
		}
		fmt.Fprintf(w, "Receive %s to %s (cause: %X)\n", amt, res.Origin, deposit.Cause)

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.SyntheticDepositCredits:
		deposit := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Source \t:%s\n", deposit.Source)
		fmt.Fprintf(w, "Credits \t:%d\n", deposit.Amount)

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.SyntheticCreateChain:
		scc := txn
		for _, cp := range scc.Chains {
			c, err := protocol.UnmarshalAccount(cp.Data)
			if err != nil {
				return err
			}
			// unmarshal
			verb := "Created"
			if cp.IsUpdate {
				verb = "Updated"
			}
			fmt.Fprintf(w, "%s %v (%v)\n", verb, c.GetUrl(), c.Type())
		}
		return nil
	case *protocol.CreateIdentity:
		id := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ADI URL \t\t:\t%s\n", id.Url)
		fmt.Fprintf(w, "Key Book URL\t\t:\t%s\n", id.KeyBookUrl)

		keyName, err := FindLabelFromPubKey(id.KeyHash)
		if err != nil {
			fmt.Fprintf(w, "Public Key \t:\t%x\n", id.KeyHash)
		} else {
			fmt.Fprintf(w, "Public Key (name) \t:\t%x (%s)\n", id.KeyHash, keyName)
		}

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.CreateDataAccount:
		dataAcc := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ADI URL \t\t:\t%s\n", dataAcc.Url)
		fmt.Fprintf(w, "Key Book URL\t\t:\t%s\n", dataAcc.KeyBookUrl)
		fmt.Fprintf(w, "Manager Book URL\t\t:\t%s\n", dataAcc.ManagerKeyBookUrl)
		fmt.Fprintf(w, "Is scratch account\t\t:\t%t\n", dataAcc.Scratch)

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.CreateKeyBook:
		keyBook := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ADI URL \t\t:\t%s\n", keyBook.Url)
		fmt.Fprintf(w, "Key Book URL\t\t:\t%s\n", keyBook.PublicKeyHash)
		fmt.Fprintf(w, "Manager Book URL\t\t:\t%s\n", keyBook.Manager)

		keyName, err := FindLabelFromPubKey(keyBook.PublicKeyHash)
		if err != nil {
			fmt.Fprintf(w, "Public Key \t:\t%x\n", keyBook.PublicKeyHash)
		} else {
			fmt.Fprintf(w, "Public Key (name) \t:\t%x (%s)\n", keyBook.PublicKeyHash, keyName)
		}

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.CreateKeyPage:
		page := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Manager URL\t\t:\t%s\n", page.Manager)

		for _, key := range page.Keys {
			keyName, err := FindLabelFromPubKey(key.KeyHash)
			if err != nil {
				fmt.Fprintf(w, "Public Key \t:\t%x\n", key.KeyHash)
			} else {
				fmt.Fprintf(w, "Public Key (name) \t:\t%x (%s)\n", key.KeyHash, keyName)
			}
			fmt.Fprintf(w, "Key owner \t:\t%s\n", key.Owner)
		}

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.SyntheticAnchor:
		synthAnchor := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Block \t\t\t:\t%d\n", synthAnchor.Block)
		fmt.Fprintf(w, "ACME burnt \t\t:\t%d\n", synthAnchor.AcmeBurnt.Int64())
		fmt.Fprintf(w, "ACME oracle price \t:\t%d\n", synthAnchor.AcmeOraclePrice)

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.UpdateKeyPage:
		page := txn
		fmt.Fprintln(w)
		for _, op := range page.Operation {
			fmt.Fprintf(w, "Type \t:\t%v\n", op.Type())
			// TODO switch operation types
		}
		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.SyntheticReceipt:
		synthRcpt := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Source \t\t:%s\n", synthRcpt.Source)
		fmt.Fprintf(w, "SynthTxHash \t:\t%s\n", hex.EncodeToString(synthRcpt.SynthTxHash[:]))
		fmt.Fprintf(w, "Status \t\t:\t%s\n", getTxStatus(synthRcpt.Status))

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.SyntheticBurnTokens:
		burn := txn
		fmt.Fprintln(w)
		amt, err := formatAmount(burn.Amount.String(), &burn.Amount)
		if err != nil {
			amt = "unknown"
		}

		fmt.Fprintf(w, "Source \t\t\t:\t%s\n", burn.Source)
		fmt.Fprintf(w, "Amount \t\t\t:\t%s\n", amt)

		printGeneralTransactionParameters(w, res)
		return nil
	case *protocol.SegWitDataEntry:
		seg := txn
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Source \t\t\t:\t%s\n", seg.Source)
		fmt.Fprintf(w, "EntryUrl \t\t\t:\t%s\n", seg.EntryUrl)
		fmt.Fprintf(w, "EntryHash \t\t\t:\t%s\n", hex.EncodeToString(seg.EntryHash[:]))

		printGeneralTransactionParameters(w, res)
		return nil

	default:
		fmt.Fprintln(w, printReflection("", "", reflect.ValueOf(txn)))
	}
	return nil
}

func getTxStatus(status *protocol.TransactionStatus) string {
	strStatus := ""
	if status.Code > 0 {
		strStatus = "error: " + status.Message
	} else if status.Delivered {
		strStatus = "delivered"
	} else if status.Pending {
		strStatus = "pending"
	}
	return strStatus
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
