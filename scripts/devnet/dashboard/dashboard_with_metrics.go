package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

type PartitionInfo struct {
	ID             string `json:"id"`
	Height         uint64 `json:"height"`
	Type           string `json:"type"`
	IsPaused       bool   `json:"isPaused"`
	BasePort       int    `json:"basePort"`
	ValidatorCount int    `json:"validatorCount"`
}

type CrosschainInfo struct {
	Source       string `json:"source"`
	Destination  string `json:"destination"`
	Type         string `json:"type"`
	SourceHeight uint64 `json:"sourceHeight"`
	DestHeight   uint64 `json:"destHeight"`
}

// Load Generator Metrics
type LoadMetrics struct {
	FaucetRequests      uint64    `json:"faucetRequests"`
	FaucetSuccess       uint64    `json:"faucetSuccess"`
	CreditsPurchased    uint64    `json:"creditsPurchased"`
	ADICreations        uint64    `json:"adiCreations"`
	TokenTransfers      uint64    `json:"tokenTransfers"`
	DataWrites          uint64    `json:"dataWrites"`
	TotalAccounts       uint64    `json:"totalAccounts"`
	TotalADIs           uint64    `json:"totalADIs"`
	TransactionsPerSec  float64   `json:"tps"`
	SuccessRate         float64   `json:"successRate"`
	LastUpdate          time.Time `json:"lastUpdate"`
}

type DashboardData struct {
	Partitions []PartitionInfo  `json:"partitions"`
	Crosschain []CrosschainInfo `json:"crosschain"`
	LoadMetrics *LoadMetrics    `json:"loadMetrics,omitempty"`
	Timestamp  time.Time        `json:"timestamp"`
}

type DashboardServer struct {
	mu               sync.RWMutex
	data             DashboardData
	pausedPartitions map[string]bool
	loadMetrics      *LoadMetrics
}

func NewDashboardServer() *DashboardServer {
	return &DashboardServer{
		pausedPartitions: make(map[string]bool),
		loadMetrics:      &LoadMetrics{LastUpdate: time.Now()},
	}
}

// Query API for partition data
func (ds *DashboardServer) queryAPI(url string) (map[string]interface{}, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "query",
		"params":  map[string]interface{}{"url": url},
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Try multiple ports for each partition
	ports := []string{
		"27004", // DN
		"27001", // BVN1
		"27007", // BVN2
		"27013", // BVN3
		"27019", // BVN4
	}

	for _, port := range ports {
		resp, err := http.Post(
			fmt.Sprintf("http://127.0.0.1:%s/v3", port),
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		if res, ok := result["result"].(map[string]interface{}); ok {
			return res, nil
		}
	}

	return nil, fmt.Errorf("failed to query %s", url)
}

// Fetch load generator metrics
func (ds *DashboardServer) fetchLoadMetrics() {
	// Try to fetch metrics from load generator endpoint
	resp, err := http.Get("http://127.0.0.1:9090/metrics")
	if err != nil {
		// Load generator not running or no metrics endpoint
		return
	}
	defer resp.Body.Close()

	var metrics LoadMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return
	}

	ds.mu.Lock()
	ds.loadMetrics = &metrics
	ds.loadMetrics.LastUpdate = time.Now()
	ds.mu.Unlock()
}

func (ds *DashboardServer) updateData() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	newData := DashboardData{
		Partitions: []PartitionInfo{},
		Crosschain: []CrosschainInfo{},
		LoadMetrics: ds.loadMetrics,
		Timestamp:  time.Now(),
	}

	// Get partition info
	partitions := []struct {
		id        string
		partType  string
		basePort  int
		validators int
	}{
		{"DN", "directory", 27004, 1},
		{"BVN1", "bvn", 27001, 3},
		{"BVN2", "bvn", 27007, 3},
		{"BVN3", "bvn", 27013, 3},
		{"BVN4", "bvn", 27019, 3},
	}

	for _, p := range partitions {
		info := PartitionInfo{
			ID:             p.id,
			Type:           p.partType,
			BasePort:       p.basePort,
			ValidatorCount: p.validators,
			IsPaused:       ds.pausedPartitions[p.id],
		}

		// Try to get height from ledger
		url := fmt.Sprintf("acc://%s/ledger", p.id)
		if result, err := ds.queryAPI(url); err == nil {
			if data, ok := result["data"].(map[string]interface{}); ok {
				if ledger, ok := data["systemLedger"].(map[string]interface{}); ok {
					if index, ok := ledger["index"].(float64); ok {
						info.Height = uint64(index)
					}
				}
			}
		}

		newData.Partitions = append(newData.Partitions, info)
	}

	// Get crosschain info - anchors only between DN and BVNs
	for _, bvn := range []string{"BVN1", "BVN2", "BVN3", "BVN4"} {
		// DN -> BVN
		dnToBvn := CrosschainInfo{
			Source:      "DN",
			Destination: bvn,
			Type:        "anchor",
		}
		
		// BVN -> DN
		bvnToDn := CrosschainInfo{
			Source:      bvn,
			Destination: "DN",
			Type:        "anchor",
		}
		
		newData.Crosschain = append(newData.Crosschain, dnToBvn, bvnToDn)
	}

	ds.data = newData
}

func (ds *DashboardServer) handleData(w http.ResponseWriter, r *http.Request) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ds.data)
}

func (ds *DashboardServer) handlePause(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Partition string `json:"partition"`
		IsPaused  bool   `json:"isPaused"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	ds.mu.Lock()
	ds.pausedPartitions[req.Partition] = req.IsPaused
	ds.mu.Unlock()
	
	// Note: Actual pause functionality would require integration with the conductor
	// For now, this just tracks the state in the UI
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (ds *DashboardServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Accumulate Devnet Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: 'Segoe UI', system-ui, sans-serif;
            background: linear-gradient(135deg, #1e3c72 0%, #2a5298 100%);
            color: #fff;
            min-height: 100vh;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        h1 { 
            text-align: center; 
            margin-bottom: 30px;
            font-size: 2.5em;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        
        /* Load Generator Metrics Card */
        .metrics-card {
            background: rgba(255, 255, 255, 0.15);
            backdrop-filter: blur(10px);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid rgba(255, 255, 255, 0.2);
            margin-bottom: 30px;
        }
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
            margin-top: 15px;
        }
        .metric-item {
            text-align: center;
        }
        .metric-value {
            font-size: 1.8em;
            font-weight: bold;
            color: #4CAF50;
        }
        .metric-label {
            font-size: 0.9em;
            opacity: 0.8;
            margin-top: 5px;
        }
        
        .grid { 
            display: grid; 
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .card {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid rgba(255, 255, 255, 0.2);
            transition: transform 0.3s, box-shadow 0.3s;
        }
        .card:hover {
            transform: translateY(-5px);
            box-shadow: 0 10px 30px rgba(0,0,0,0.3);
        }
        .partition-card {
            position: relative;
        }
        .partition-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 15px;
        }
        .partition-title {
            font-size: 1.4em;
            font-weight: bold;
        }
        .partition-type {
            background: rgba(255, 255, 255, 0.2);
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.9em;
        }
        .height-display {
            font-size: 2em;
            font-weight: 300;
            margin: 10px 0;
            font-family: 'Courier New', monospace;
        }
        .pause-btn {
            background: #4CAF50;
            color: white;
            border: none;
            padding: 8px 20px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 14px;
            transition: background 0.3s;
        }
        .pause-btn:hover {
            background: #45a049;
        }
        .pause-btn.paused {
            background: #f44336;
        }
        .crosschain-table {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid rgba(255, 255, 255, 0.2);
            margin-bottom: 30px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }
        th {
            background: rgba(255, 255, 255, 0.1);
            font-weight: 600;
        }
        .status-ok { color: #4CAF50; }
        .status-behind { color: #FFC107; }
        .status-critical { color: #f44336; }
        .last-update {
            text-align: center;
            opacity: 0.7;
            margin-top: 20px;
        }
        .note {
            background: rgba(255, 255, 255, 0.1);
            padding: 10px;
            border-radius: 8px;
            margin-bottom: 20px;
            text-align: center;
        }
        
        /* Responsive Design Breakpoints per Design Requirements */
        
        /* Mobile: 320px-768px (single column layout) */
        @media (max-width: 768px) {
            body {
                padding: 10px;
            }
            h1 {
                font-size: 1.8em;
            }
            .grid {
                grid-template-columns: 1fr;
                gap: 15px;
            }
            .metrics-grid {
                grid-template-columns: repeat(2, 1fr);
            }
            .card {
                padding: 15px;
            }
            .partition-title {
                font-size: 1.2em;
            }
            .height-display {
                font-size: 1.5em;
            }
            .metric-value {
                font-size: 1.4em;
            }
            table {
                font-size: 0.9em;
            }
            th, td {
                padding: 8px 5px;
            }
            .container {
                padding: 0 5px;
            }
        }
        
        /* Tablet: 768px-1024px (dual column layout) */
        @media (min-width: 768px) and (max-width: 1024px) {
            .grid {
                grid-template-columns: repeat(2, 1fr);
            }
            .metrics-grid {
                grid-template-columns: repeat(3, 1fr);
            }
            h1 {
                font-size: 2.2em;
            }
        }
        
        /* Desktop: 1024px-1400px (triple/quad column layout) */
        @media (min-width: 1024px) and (max-width: 1400px) {
            .grid {
                grid-template-columns: repeat(3, 1fr);
            }
            .metrics-grid {
                grid-template-columns: repeat(4, 1fr);
            }
        }
        
        /* Ultra-wide: >1400px (quad+ column with centered container) */
        @media (min-width: 1400px) {
            .grid {
                grid-template-columns: repeat(4, 1fr);
            }
            .metrics-grid {
                grid-template-columns: repeat(5, 1fr);
            }
            .container {
                max-width: 1400px;
                margin: 0 auto;
            }
        }
        
        /* Ensure minimum 320px width support */
        @media (max-width: 320px) {
            body {
                min-width: 320px;
                overflow-x: auto;
            }
            .grid {
                grid-template-columns: 1fr;
            }
            .metrics-grid {
                grid-template-columns: 1fr;
            }
            .card {
                min-width: 280px;
            }
        }
        
        /* Make crosschain table scrollable on small screens */
        @media (max-width: 768px) {
            .table-container {
                overflow-x: auto;
                -webkit-overflow-scrolling: touch;
            }
            table {
                min-width: 500px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 Accumulate Devnet Dashboard</h1>
        
        <div class="note">
            <strong>Note:</strong> Pause/Resume requires testnet build. BVNs exchange anchors through DN only.
        </div>
        
        <!-- Load Generator Metrics Section -->
        <div class="metrics-card" id="loadMetrics" style="display: none;">
            <h2>📊 Load Generator Metrics</h2>
            <div class="metrics-grid">
                <div class="metric-item">
                    <div class="metric-value" id="tps">0</div>
                    <div class="metric-label">TPS</div>
                </div>
                <div class="metric-item">
                    <div class="metric-value" id="totalAccounts">0</div>
                    <div class="metric-label">Accounts</div>
                </div>
                <div class="metric-item">
                    <div class="metric-value" id="totalADIs">0</div>
                    <div class="metric-label">ADIs</div>
                </div>
                <div class="metric-item">
                    <div class="metric-value" id="successRate">0%</div>
                    <div class="metric-label">Success Rate</div>
                </div>
                <div class="metric-item">
                    <div class="metric-value" id="transfers">0</div>
                    <div class="metric-label">Transfers</div>
                </div>
            </div>
        </div>
        
        <div class="grid" id="partitions"></div>
        
        <div class="crosschain-table">
            <h2 style="margin-bottom: 15px;">Anchor Exchange (DN ↔ BVNs)</h2>
            <div class="table-container">
                <table id="crosschain">
                    <thead>
                        <tr>
                            <th>Source</th>
                            <th>Destination</th>
                            <th>Type</th>
                            <th>Source Height</th>
                            <th>Dest Height</th>
                            <th>Gap</th>
                            <th>Status</th>
                        </tr>
                    </thead>
                    <tbody></tbody>
                </table>
            </div>
        </div>
        
        <div class="last-update" id="lastUpdate"></div>
    </div>

    <script>
        async function updateDashboard() {
            try {
                const response = await fetch('/api/data');
                const data = await response.json();
                
                // Update Load Generator Metrics if available
                if (data.loadMetrics) {
                    document.getElementById('loadMetrics').style.display = 'block';
                    document.getElementById('tps').textContent = data.loadMetrics.tps?.toFixed(2) || '0';
                    document.getElementById('totalAccounts').textContent = data.loadMetrics.totalAccounts || '0';
                    document.getElementById('totalADIs').textContent = data.loadMetrics.totalADIs || '0';
                    document.getElementById('successRate').textContent = 
                        (data.loadMetrics.successRate ? (data.loadMetrics.successRate * 100).toFixed(1) + '%' : '0%');
                    document.getElementById('transfers').textContent = data.loadMetrics.tokenTransfers || '0';
                }
                
                // Update partitions
                const partitionsDiv = document.getElementById('partitions');
                partitionsDiv.innerHTML = '';
                
                data.partitions.forEach(p => {
                    const card = document.createElement('div');
                    card.className = 'card partition-card';
                    card.innerHTML = ` + "`" + `
                        <div class="partition-header">
                            <span class="partition-title">${p.id}</span>
                            <span class="partition-type">${p.type}</span>
                        </div>
                        <div class="height-display">Block #${p.height || 0}</div>
                        <div style="font-size: 0.9em; opacity: 0.7;">
                            Validators: ${p.validatorCount} | Port: ${p.basePort}
                        </div>
                        <button class="pause-btn ${p.isPaused ? 'paused' : ''}" 
                                onclick="togglePause('${p.id}', ${!p.isPaused})">
                            ${p.isPaused ? 'Resume' : 'Pause'} CCC
                        </button>
                    ` + "`" + `;
                    partitionsDiv.appendChild(card);
                });
                
                // Update crosschain table
                const tbody = document.querySelector('#crosschain tbody');
                tbody.innerHTML = '';
                
                data.crosschain.forEach(cc => {
                    const gap = Math.abs(cc.sourceHeight - cc.destHeight);
                    let status = 'OK';
                    let statusClass = 'status-ok';
                    
                    if (gap > 10) {
                        status = 'Behind';
                        statusClass = 'status-behind';
                    }
                    if (gap > 50) {
                        status = 'Critical';
                        statusClass = 'status-critical';
                    }
                    
                    const row = document.createElement('tr');
                    row.innerHTML = ` + "`" + `
                        <td>${cc.source}</td>
                        <td>${cc.destination}</td>
                        <td>${cc.type}</td>
                        <td>${cc.sourceHeight || 0}</td>
                        <td>${cc.destHeight || 0}</td>
                        <td>${gap}</td>
                        <td class="${statusClass}">${status}</td>
                    ` + "`" + `;
                    tbody.appendChild(row);
                });
                
                // Update timestamp
                document.getElementById('lastUpdate').textContent = 
                    'Last updated: ' + new Date(data.timestamp).toLocaleTimeString();
                
            } catch (error) {
                console.error('Failed to update dashboard:', error);
            }
        }
        
        async function togglePause(partition, isPaused) {
            try {
                await fetch('/api/pause', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ partition, isPaused })
                });
                updateDashboard();
            } catch (error) {
                console.error('Failed to toggle pause:', error);
            }
        }
        
        // Update every 2 seconds
        updateDashboard();
        setInterval(updateDashboard, 2000);
    </script>
</body>
</html>`
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	port := os.Getenv("DASHBOARD_PORT")
	if port == "" {
		port = "8080"
	}

	server := NewDashboardServer()

	// Start background update loop
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		for {
			server.updateData()
			server.fetchLoadMetrics()
			<-ticker.C
		}
	}()

	// Set up HTTP routes
	http.HandleFunc("/", server.handleIndex)
	http.HandleFunc("/api/data", server.handleData)
	http.HandleFunc("/api/pause", server.handlePause)

	// Auto-open browser
	go func() {
		time.Sleep(2 * time.Second)
		url := fmt.Sprintf("http://localhost:%s", port)
		
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "linux":
			cmd = exec.Command("xdg-open", url)
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		}
		
		if cmd != nil {
			cmd.Start()
		}
	}()

	log.Printf("Dashboard server starting on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}