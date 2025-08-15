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

type DashboardData struct {
	Partitions []PartitionInfo  `json:"partitions"`
	Crosschain []CrosschainInfo `json:"crosschain"`
	Timestamp  time.Time        `json:"timestamp"`
}

type DashboardServer struct {
	mu               sync.RWMutex
	data             DashboardData
	pausedPartitions map[string]bool
}

func NewDashboardServer() *DashboardServer {
	return &DashboardServer{
		pausedPartitions: make(map[string]bool),
	}
}

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

func (ds *DashboardServer) updateData() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	newData := DashboardData{
		Partitions: []PartitionInfo{},
		Crosschain: []CrosschainInfo{},
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
	bvnIDs := []string{"BVN1", "BVN2", "BVN3", "BVN4"}
	
	// DN to each BVN and each BVN to DN
	for _, bvn := range bvnIDs {
		// DN -> BVN
		dnToBvn := CrosschainInfo{
			Source:      "DN",
			Destination: bvn,
			Type:        "anchor",
		}

		// Try to get anchor pool info for DN -> BVN
		url := fmt.Sprintf("acc://DN/anchors/%s", bvn)
		if result, err := ds.queryAPI(url); err == nil {
			if data, ok := result["data"].(map[string]interface{}); ok {
				if anchorLedger, ok := data["anchorLedger"].(map[string]interface{}); ok {
					if produced, ok := anchorLedger["produced"].(float64); ok {
						dnToBvn.SourceHeight = uint64(produced)
					}
					if received, ok := anchorLedger["received"].(float64); ok {
						dnToBvn.DestHeight = uint64(received)
					}
				}
			}
		}
		newData.Crosschain = append(newData.Crosschain, dnToBvn)

		// BVN -> DN
		bvnToDn := CrosschainInfo{
			Source:      bvn,
			Destination: "DN",
			Type:        "anchor",
		}

		// Try to get anchor pool info for BVN -> DN
		url = fmt.Sprintf("acc://%s/anchors/DN", bvn)
		if result, err := ds.queryAPI(url); err == nil {
			if data, ok := result["data"].(map[string]interface{}); ok {
				if anchorLedger, ok := data["anchorLedger"].(map[string]interface{}); ok {
					if produced, ok := anchorLedger["produced"].(float64); ok {
						bvnToDn.SourceHeight = uint64(produced)
					}
					if received, ok := anchorLedger["received"].(float64); ok {
						bvnToDn.DestHeight = uint64(received)
					}
				}
			}
		}
		newData.Crosschain = append(newData.Crosschain, bvnToDn)
	}

	ds.data = newData
}

func (ds *DashboardServer) handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ds.mu.RLock()
	data := ds.data
	ds.mu.RUnlock()

	json.NewEncoder(w).Encode(data)
}

func (ds *DashboardServer) handlePause(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req struct {
		Partition string `json:"partition"`
		Pause     bool   `json:"pause"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ds.mu.Lock()
	ds.pausedPartitions[req.Partition] = req.Pause
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
            margin-top: 10px;
        }
        .pause-btn:hover { background: #45a049; }
        .pause-btn.paused {
            background: #f44336;
        }
        .pause-btn.paused:hover { background: #da190b; }
        .crosschain-table {
            background: rgba(255, 255, 255, 0.05);
            border-radius: 12px;
            padding: 20px;
            overflow-x: auto;
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
        tr:hover {
            background: rgba(255, 255, 255, 0.05);
        }
        .status-indicator {
            display: inline-block;
            width: 10px;
            height: 10px;
            border-radius: 50%;
            margin-right: 5px;
        }
        .status-ok { background: #4CAF50; }
        .status-behind { background: #ff9800; }
        .status-error { background: #f44336; }
        .last-update {
            text-align: center;
            opacity: 0.7;
            font-size: 0.9em;
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
            .card {
                padding: 15px;
            }
            .partition-title {
                font-size: 1.2em;
            }
            .height-display {
                font-size: 1.5em;
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
            h1 {
                font-size: 2.2em;
            }
        }
        
        /* Desktop: 1024px-1400px (triple/quad column layout) */
        @media (min-width: 1024px) and (max-width: 1400px) {
            .grid {
                grid-template-columns: repeat(3, 1fr);
            }
        }
        
        /* Ultra-wide: >1400px (quad+ column with centered container) */
        @media (min-width: 1400px) {
            .grid {
                grid-template-columns: repeat(4, 1fr);
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
            ℹ️ Dashboard monitoring devnet with 4 BVNs (3 validators each)
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
        let data = { partitions: [], crosschain: [] };

        async function fetchData() {
            try {
                const response = await fetch('/api/data');
                data = await response.json();
                updateUI();
            } catch (error) {
                console.error('Error fetching data:', error);
            }
        }

        function updateUI() {
            // Update partitions
            const partitionsDiv = document.getElementById('partitions');
            partitionsDiv.innerHTML = data.partitions.map(p => ` + "`" + `
                <div class="card partition-card">
                    <div class="partition-header">
                        <div class="partition-title">${p.id}</div>
                        <div class="partition-type">${p.type.toUpperCase()}</div>
                    </div>
                    <div class="height-display">Height: ${p.height.toLocaleString()}</div>
                    <div>Port: ${p.basePort}</div>
                    <div>Validators: ${p.validatorCount}</div>
                    <button class="pause-btn ${p.isPaused ? 'paused' : ''}" 
                            onclick="togglePause('${p.id}', ${!p.isPaused})">
                        ${p.isPaused ? '▶ Resume' : '⏸ Pause'} (UI Only)
                    </button>
                </div>
            ` + "`" + `).join('');

            // Update crosschain table
            const tbody = document.querySelector('#crosschain tbody');
            tbody.innerHTML = data.crosschain.map(cc => {
                const gap = cc.sourceHeight - cc.destHeight;
                let statusClass = 'status-ok';
                let statusText = 'OK';
                
                if (gap > 10) {
                    statusClass = 'status-behind';
                    statusText = 'Behind';
                }
                if (gap > 50) {
                    statusClass = 'status-error';
                    statusText = 'Critical';
                }

                return ` + "`" + `
                    <tr>
                        <td>${cc.source}</td>
                        <td>${cc.destination}</td>
                        <td>${cc.type}</td>
                        <td>${cc.sourceHeight.toLocaleString()}</td>
                        <td>${cc.destHeight.toLocaleString()}</td>
                        <td>${gap.toLocaleString()}</td>
                        <td>
                            <span class="status-indicator ${statusClass}"></span>
                            ${statusText}
                        </td>
                    </tr>
                ` + "`" + `;
            }).join('');

            // Update timestamp
            if (data.timestamp) {
                document.getElementById('lastUpdate').textContent = 
                    'Last updated: ' + new Date(data.timestamp).toLocaleTimeString();
            }
        }

        async function togglePause(partition, pause) {
            try {
                await fetch('/api/pause', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ partition, pause })
                });
                await fetchData();
            } catch (error) {
                console.error('Error toggling pause:', error);
            }
        }

        // Fetch data every 2 seconds
        fetchData();
        setInterval(fetchData, 2000);
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

	// Start background updater
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			server.updateData()
			<-ticker.C
		}
	}()

	http.HandleFunc("/", server.handleIndex)
	http.HandleFunc("/api/data", server.handleAPI)
	http.HandleFunc("/api/pause", server.handlePause)

	log.Printf("Dashboard server starting on port %s", port)
	log.Printf("Open http://localhost:%s in your browser", port)

	// Auto-open browser
	go func() {
		time.Sleep(1 * time.Second)
		exec.Command("xdg-open", fmt.Sprintf("http://localhost:%s", port)).Start()
	}()

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}