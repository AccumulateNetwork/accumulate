//go:build !testnet
// +build !testnet

package load_test

import (
	"fmt"
	"math/big"
	"strings"
)

func (ctx *LoadTestContext) GenerateReport(results *LoadResults) string {
	var report strings.Builder
	
	report.WriteString("\n=== LOAD TEST REPORT ===\n")
	report.WriteString(fmt.Sprintf("Total Transactions: %d\n", results.TotalSent))
	report.WriteString(fmt.Sprintf("Successful: %d\n", results.TotalSuccess))
	report.WriteString(fmt.Sprintf("Failed: %d\n", results.TotalFailed))
	report.WriteString(fmt.Sprintf("TPS: %.2f\n", results.TPS))
	report.WriteString(fmt.Sprintf("Duration: %v\n\n", results.Duration))
	
	report.WriteString(ctx.PrintSenderTable())
	report.WriteString("\n")
	report.WriteString(ctx.PrintReceiverTable())
	report.WriteString("\n")
	
	summary := ctx.CalculateSummary()
	report.WriteString("=== SUMMARY ===\n")
	report.WriteString(fmt.Sprintf("Total Expected Sent: %.4f ACME\n", float64(summary.TotalExpectedSent)/1e8))
	report.WriteString(fmt.Sprintf("Total Actual Sent: %.4f ACME\n", float64(summary.TotalActualSent)/1e8))
	report.WriteString(fmt.Sprintf("Total Expected Received: %.4f ACME\n", float64(summary.TotalExpectedReceived)/1e8))
	report.WriteString(fmt.Sprintf("Total Actual Received: %.4f ACME\n", float64(summary.TotalActualReceived)/1e8))
	report.WriteString(fmt.Sprintf("Sender Discrepancy: %.4f ACME\n", float64(summary.SenderDiscrepancy)/1e8))
	report.WriteString(fmt.Sprintf("Receiver Discrepancy: %.4f ACME\n", float64(summary.ReceiverDiscrepancy)/1e8))
	
	return report.String()
}

func (ctx *LoadTestContext) PrintSenderTable() string {
	var table strings.Builder
	
	headers := []string{"Sender", "Expected", "Actual", "Diff", "Status"}
	rows := [][]string{}
	
	txPerSender := ctx.Config.NumTxs / ctx.Config.NumSenders
	remainder := ctx.Config.NumTxs % ctx.Config.NumSenders
	
	for i, account := range ctx.KAccounts {
		txCount := txPerSender
		if i < remainder {
			txCount++
		}
		
		expectedSpend := int64(txCount) * ctx.Config.TxAmount
		expectedBalance := ctx.Config.ACMEPerK - expectedSpend
		
		balance, _ := ctx.GetBalance(account.URL)
		var actualBalance int64
		if balance != nil {
			actualBalance = balance.Int64()
		}
		diff := actualBalance - expectedBalance
		
		status := "✓"
		if diff > 1e4 {
			status = "⚠️ NOT DEBITED"
		} else if diff < -1e4 {
			status = "❌ WRONG"
		}
		
		rows = append(rows, []string{
			fmt.Sprintf("k%d", i+1),
			fmt.Sprintf("%.4f", float64(expectedBalance)/1e8),
			fmt.Sprintf("%.4f", float64(actualBalance)/1e8),
			fmt.Sprintf("%.4f", float64(diff)/1e8),
			status,
		})
	}
	
	table.WriteString(FormatTable(headers, rows))
	return table.String()
}

func (ctx *LoadTestContext) PrintReceiverTable() string {
	var table strings.Builder
	
	headers := []string{"Receiver", "Expected", "Actual", "Diff", "Status"}
	rows := [][]string{}
	
	txPerReceiver := make([]int, ctx.Config.NumReceivers)
	for i := 0; i < ctx.Config.NumTxs; i++ {
		receiverIdx := i % ctx.Config.NumReceivers
		txPerReceiver[receiverIdx]++
	}
	
	for i, account := range ctx.AAccounts {
		expectedReceive := int64(txPerReceiver[i]) * ctx.Config.TxAmount
		
		balance, _ := ctx.GetBalance(account.URL)
		var actualBalance int64
		if balance != nil {
			actualBalance = balance.Int64()
		}
		diff := actualBalance - expectedReceive
		
		status := "✓"
		if actualBalance == 0 && expectedReceive > 0 {
			status = "❌ MISSING"
		} else if diff < -1e4 {
			status = "❌ WRONG"
		}
		
		rows = append(rows, []string{
			fmt.Sprintf("a%d", i+1),
			fmt.Sprintf("%.4f", float64(expectedReceive)/1e8),
			fmt.Sprintf("%.4f", float64(actualBalance)/1e8),
			fmt.Sprintf("%.4f", float64(diff)/1e8),
			status,
		})
	}
	
	table.WriteString(FormatTable(headers, rows))
	return table.String()
}

func (ctx *LoadTestContext) CalculateSummary() *Summary {
	summary := &Summary{}
	
	txPerSender := ctx.Config.NumTxs / ctx.Config.NumSenders
	remainder := ctx.Config.NumTxs % ctx.Config.NumSenders
	
	for i, account := range ctx.KAccounts {
		txCount := txPerSender
		if i < remainder {
			txCount++
		}
		
		expectedSpend := int64(txCount) * ctx.Config.TxAmount
		summary.TotalExpectedSent += expectedSpend
		
		balance, _ := ctx.GetBalance(account.URL)
		actualSpend := ctx.Config.ACMEPerK - balance.Int64()
		summary.TotalActualSent += actualSpend
	}
	
	txPerReceiver := make([]int, ctx.Config.NumReceivers)
	for i := 0; i < ctx.Config.NumTxs; i++ {
		receiverIdx := i % ctx.Config.NumReceivers
		txPerReceiver[receiverIdx]++
	}
	
	for i, account := range ctx.AAccounts {
		expectedReceive := int64(txPerReceiver[i]) * ctx.Config.TxAmount
		summary.TotalExpectedReceived += expectedReceive
		
		balance, _ := ctx.GetBalance(account.URL)
		summary.TotalActualReceived += balance.Int64()
	}
	
	summary.SenderDiscrepancy = summary.TotalExpectedSent - summary.TotalActualSent
	summary.ReceiverDiscrepancy = summary.TotalExpectedReceived - summary.TotalActualReceived
	
	return summary
}

func (ctx *LoadTestContext) DetectIssues() []Issue {
	issues := []Issue{}
	
	for i, account := range ctx.KAccounts {
		balance, _ := ctx.GetBalance(account.URL)
		if balance.Cmp(big.NewInt(ctx.Config.ACMEPerK)) >= 0 {
			issues = append(issues, Issue{
				Account:     fmt.Sprintf("k%d", i+1),
				Type:        "NOT_DEBITED",
				Description: "Sender was not debited for transactions",
			})
		}
	}
	
	for i, account := range ctx.AAccounts {
		balance, _ := ctx.GetBalance(account.URL)
		if balance.Cmp(big.NewInt(0)) == 0 {
			issues = append(issues, Issue{
				Account:     fmt.Sprintf("a%d", i+1),
				Type:        "NOT_CREDITED",
				Description: "Receiver did not receive any transactions",
			})
		}
	}
	
	return issues
}

func FormatTable(headers []string, rows [][]string) string {
	var table strings.Builder
	
	colWidths := make([]int, len(headers))
	for i, header := range headers {
		colWidths[i] = len(header)
	}
	
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}
	
	for i, header := range headers {
		table.WriteString(fmt.Sprintf("%-*s  ", colWidths[i], header))
	}
	table.WriteString("\n")
	
	for i := range headers {
		table.WriteString(strings.Repeat("-", colWidths[i]))
		table.WriteString("  ")
	}
	table.WriteString("\n")
	
	for _, row := range rows {
		for i, cell := range row {
			table.WriteString(fmt.Sprintf("%-*s  ", colWidths[i], cell))
		}
		table.WriteString("\n")
	}
	
	return table.String()
}