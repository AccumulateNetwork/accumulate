// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
)

type WebServer struct {
	checker *BalanceChecker
	port    int
}

func NewWebServer(checker *BalanceChecker, port int) *WebServer {
	return &WebServer{
		checker: checker,
		port:    port,
	}
}

func (s *WebServer) Start() error {
	http.HandleFunc("/", s.handleHome)
	http.HandleFunc("/api/balances", s.handleAPIBalances)
	
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Starting web server at http://localhost%s", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *WebServer) handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Accumulate Balance Checker</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #fff;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        h1 {
            text-align: center;
            margin-bottom: 30px;
            font-size: 2.5em;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.2);
        }
        .input-section {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 15px;
            padding: 25px;
            margin-bottom: 30px;
            border: 1px solid rgba(255, 255, 255, 0.2);
        }
        .input-group {
            display: flex;
            gap: 15px;
            align-items: center;
        }
        input[type="text"] {
            flex: 1;
            padding: 12px 20px;
            border: 1px solid rgba(255, 255, 255, 0.3);
            background: rgba(255, 255, 255, 0.1);
            color: white;
            border-radius: 10px;
            font-size: 1em;
        }
        input[type="text"]::placeholder {
            color: rgba(255, 255, 255, 0.6);
        }
        button {
            padding: 12px 30px;
            background: rgba(255, 255, 255, 0.2);
            border: 1px solid rgba(255, 255, 255, 0.3);
            color: white;
            border-radius: 10px;
            cursor: pointer;
            font-size: 1em;
            transition: all 0.3s ease;
        }
        button:hover {
            background: rgba(255, 255, 255, 0.3);
            transform: scale(1.05);
        }
        .results {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 15px;
            padding: 25px;
            border: 1px solid rgba(255, 255, 255, 0.2);
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }
        th {
            font-weight: bold;
            background: rgba(255, 255, 255, 0.05);
        }
        tr:hover {
            background: rgba(255, 255, 255, 0.05);
        }
        .balance-value {
            font-weight: bold;
            font-size: 1.1em;
        }
        .status-ok { color: #4ade80; }
        .status-error { color: #f87171; }
        .loading {
            text-align: center;
            padding: 40px;
            font-size: 1.2em;
        }
        .error-message {
            color: #f87171;
            padding: 10px;
            background: rgba(248, 113, 113, 0.1);
            border-radius: 5px;
            margin-top: 10px;
        }
        .quick-links {
            margin-top: 15px;
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }
        .quick-link {
            padding: 8px 15px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 20px;
            cursor: pointer;
            font-size: 0.9em;
            transition: all 0.3s ease;
        }
        .quick-link:hover {
            background: rgba(255, 255, 255, 0.2);
        }
        .summary-card {
            background: rgba(255, 255, 255, 0.05);
            padding: 20px;
            border-radius: 10px;
            margin-top: 20px;
        }
        .summary-title {
            font-size: 1.2em;
            margin-bottom: 15px;
            opacity: 0.9;
        }
        .summary-item {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>💰 Accumulate Balance Checker</h1>
        
        <div class="input-section">
            <div class="input-group">
                <input type="text" 
                       id="accountsInput" 
                       placeholder="Enter account URLs (comma-separated, e.g., acc://ACME, acc://my-account.acme)"
                       value="acc://ACME">
                <button onclick="checkBalances()">Check Balances</button>
            </div>
            <div class="quick-links">
                <div class="quick-link" onclick="setAccounts('acc://ACME')">ACME Token</div>
                <div class="quick-link" onclick="setAccounts('acc://ACME, acc://dn.acme')">ACME + DN</div>
                <div class="quick-link" onclick="setAccounts('acc://ACME, acc://dn.acme/tokens')">Common Tokens</div>
            </div>
        </div>
        
        <div id="results" class="results" style="display: none;">
            <div id="resultsContent"></div>
        </div>
    </div>

    <script>
        function setAccounts(accounts) {
            document.getElementById('accountsInput').value = accounts;
            checkBalances();
        }
        
        async function checkBalances() {
            const accountsInput = document.getElementById('accountsInput').value;
            if (!accountsInput.trim()) {
                alert('Please enter at least one account URL');
                return;
            }
            
            const accounts = accountsInput.split(',').map(a => a.trim());
            const resultsDiv = document.getElementById('results');
            const resultsContent = document.getElementById('resultsContent');
            
            resultsDiv.style.display = 'block';
            resultsContent.innerHTML = '<div class="loading">Loading balances...</div>';
            
            try {
                const response = await fetch('/api/balances', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ accounts: accounts })
                });
                
                const data = await response.json();
                displayResults(data);
            } catch (error) {
                resultsContent.innerHTML = '<div class="error-message">Error: ' + error.message + '</div>';
            }
        }
        
        function displayResults(data) {
            const resultsContent = document.getElementById('resultsContent');
            
            let html = '<h2>Balance Report</h2>';
            html += '<p style="opacity: 0.8; margin: 10px 0;">Last updated: ' + new Date().toLocaleString() + '</p>';
            
            html += '<table>';
            html += '<thead><tr>';
            html += '<th>Account</th>';
            html += '<th>Type</th>';
            html += '<th>Symbol</th>';
            html += '<th>Balance</th>';
            html += '<th>Credits</th>';
            html += '<th>Status</th>';
            html += '</tr></thead>';
            html += '<tbody>';
            
            let totalValue = 0;
            let hasIssuers = false;
            
            for (const balance of data.balances) {
                const statusClass = balance.error ? 'status-error' : 'status-ok';
                const statusIcon = balance.error ? '❌' : '✅';
                
                html += '<tr>';
                html += '<td>' + balance.url + '</td>';
                html += '<td>' + (balance.type || '-') + '</td>';
                html += '<td>' + (balance.symbol || '-') + '</td>';
                
                if (balance.balanceFloat > 0) {
                    html += '<td class="balance-value">' + balance.balanceFloat.toFixed(8) + '</td>';
                    totalValue += balance.balanceFloat;
                } else {
                    html += '<td>' + (balance.balance || '-') + '</td>';
                }
                
                html += '<td>' + (balance.credits || '-') + '</td>';
                html += '<td class="' + statusClass + '">' + statusIcon + '</td>';
                html += '</tr>';
                
                if (balance.type === 'tokenIssuer') {
                    hasIssuers = true;
                }
            }
            
            html += '</tbody></table>';
            
            // Summary section for token issuers
            if (hasIssuers) {
                html += '<div class="summary-card">';
                html += '<div class="summary-title">📊 Token Issuer Summary</div>';
                
                for (const balance of data.balances) {
                    if (balance.type === 'tokenIssuer' && !balance.error) {
                        html += '<div style="margin-bottom: 15px;">';
                        html += '<strong>' + balance.url + ' (' + balance.symbol + ')</strong>';
                        
                        if (balance.issued) {
                            html += '<div class="summary-item">';
                            html += '<span>Issued:</span>';
                            html += '<span>' + (balance.balanceFloat || 0).toFixed(8) + ' ' + balance.symbol + '</span>';
                            html += '</div>';
                        }
                        
                        if (balance.supplyLimit) {
                            const limitFloat = balance.supplyLimit / Math.pow(10, 8);
                            html += '<div class="summary-item">';
                            html += '<span>Supply Limit:</span>';
                            html += '<span>' + limitFloat.toFixed(8) + ' ' + balance.symbol + '</span>';
                            html += '</div>';
                            
                            if (balance.issued && balance.supplyLimit > 0) {
                                const utilization = (balance.balanceFloat / limitFloat) * 100;
                                html += '<div class="summary-item">';
                                html += '<span>Utilization:</span>';
                                html += '<span>' + utilization.toFixed(2) + '%</span>';
                                html += '</div>';
                            }
                        }
                        
                        html += '</div>';
                    }
                }
                
                html += '</div>';
            }
            
            // Display any errors
            const errors = data.balances.filter(b => b.error);
            if (errors.length > 0) {
                html += '<div class="error-message">';
                html += '<strong>Errors:</strong><br>';
                for (const err of errors) {
                    html += err.url + ': ' + err.error + '<br>';
                }
                html += '</div>';
            }
            
            resultsContent.innerHTML = html;
        }
    </script>
</body>
</html>`
	
	t := template.Must(template.New("home").Parse(tmpl))
	t.Execute(w, nil)
}

func (s *WebServer) handleAPIBalances(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req struct {
		Accounts []string `json:"accounts"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	
	balances := make([]map[string]interface{}, 0, len(req.Accounts))
	
	for _, accountURL := range req.Accounts {
		accountURL = strings.TrimSpace(accountURL)
		if accountURL == "" {
			continue
		}
		
		balance := s.checker.getAccountBalance(ctx, accountURL)
		
		b := map[string]interface{}{
			"url":     balance.URL,
			"type":    balance.Type,
			"symbol":  balance.Symbol,
			"credits": balance.Credits,
		}
		
		if balance.Error != nil {
			b["error"] = balance.Error.Error()
		}
		
		if balance.Balance != nil {
			b["balance"] = balance.Balance.String()
			b["balanceFloat"] = balance.BalanceFloat
		}
		
		if balance.Issued != nil {
			b["issued"] = balance.Issued.String()
		}
		
		if balance.SupplyLimit != nil {
			b["supplyLimit"] = balance.SupplyLimit.String()
		}
		
		balances = append(balances, b)
	}
	
	response := map[string]interface{}{
		"balances": balances,
		"timestamp": time.Now().Unix(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}