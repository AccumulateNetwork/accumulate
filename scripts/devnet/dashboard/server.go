package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gitlab.com/AccumulateNetwork/accumulate2/pkg/api/v3"
	"gitlab.com/AccumulateNetwork/accumulate2/pkg/api/v3/jsonrpc"
	"gitlab.com/AccumulateNetwork/accumulate2/pkg/url"
	"gitlab.com/AccumulateNetwork/accumulate2/protocol"
)

type PartitionInfo struct {
	ID            string `json:"id"`
	Height        uint64 `json:"height"`
	Type          string `json:"type"`
	IsPaused      bool   `json:"isPaused"`
	BasePort      int    `json:"basePort"`
	ValidatorCount int   `json:"validatorCount"`
}

type CrosschainInfo struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Type        string `json:"type"`
	SourceHeight uint64 `json:"sourceHeight"`
	DestHeight   uint64 `json:"destHeight"`
}

type DashboardData struct {
	Partitions []PartitionInfo  `json:"partitions"`
	Crosschain []CrosschainInfo `json:"crosschain"`
	Timestamp  time.Time        `json:"timestamp"`
}

type DashboardServer struct {
	mu          sync.RWMutex
	data        DashboardData
	pausedPartitions map[string]bool
	clients     map[string]*jsonrpc.Client
}

func NewDashboardServer() *DashboardServer {
	return &DashboardServer{
		pausedPartitions: make(map[string]bool),
		clients:         make(map[string]*jsonrpc.Client),
	}
}

func (ds *DashboardServer) initClients() {
	// Initialize clients for DN and BVNs
	ds.clients["DN"] = &jsonrpc.Client{
		Client: api.ClientOptions{
			Servers: []string{"http://127.0.0.1:27004"},
		}.NewClient(),
	}

	// BVN clients - assuming 4 BVNs with 3 validators each
	for i := 1; i <= 4; i++ {
		bvnID := fmt.Sprintf("BVN%d", i)
		port := 27001 + (i-1)*3*2 // Each validator uses 2 ports
		ds.clients[bvnID] = &jsonrpc.Client{
			Client: api.ClientOptions{
				Servers: []string{fmt.Sprintf("http://127.0.%d.1:%d", i, port)},
			}.NewClient(),
		}
	}
}

func (ds *DashboardServer) updateData(ctx context.Context) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	newData := DashboardData{
		Partitions: []PartitionInfo{},
		Crosschain: []CrosschainInfo{},
		Timestamp:  time.Now(),
	}

	// Get DN info
	if client, ok := ds.clients["DN"]; ok {
		if info, err := ds.getPartitionInfo(ctx, client, "DN", "directory", 27004, 1); err == nil {
			info.IsPaused = ds.pausedPartitions["DN"]
			newData.Partitions = append(newData.Partitions, info)
		}
	}

	// Get BVN info
	for i := 1; i <= 4; i++ {
		bvnID := fmt.Sprintf("BVN%d", i)
		if client, ok := ds.clients[bvnID]; ok {
			basePort := 27001 + (i-1)*3*2
			if info, err := ds.getPartitionInfo(ctx, client, bvnID, "bvn", basePort, 3); err == nil {
				info.IsPaused = ds.pausedPartitions[bvnID]
				newData.Partitions = append(newData.Partitions, info)
			}
		}
	}

	// Get crosschain info for all partition pairs
	partitionIDs := []string{"DN", "BVN1", "BVN2", "BVN3", "BVN4"}
	messageTypes := []string{"synthetic", "anchor", "user"}
	
	for _, source := range partitionIDs {
		for _, dest := range partitionIDs {
			if source == dest {
				continue
			}
			for _, msgType := range messageTypes {
				if ccInfo, err := ds.getCrosschainInfo(ctx, source, dest, msgType); err == nil {
					newData.Crosschain = append(newData.Crosschain, ccInfo)
				}
			}
		}
	}

	ds.data = newData
}

func (ds *DashboardServer) getPartitionInfo(ctx context.Context, client *jsonrpc.Client, id, partType string, basePort, validatorCount int) (PartitionInfo, error) {
	info := PartitionInfo{
		ID:             id,
		Type:           partType,
		BasePort:       basePort,
		ValidatorCount: validatorCount,
	}

	// Get consensus status
	req := new(api.GeneralQuery)
	req.Url = protocol.PartitionUrl(id).JoinPath(protocol.Ledger).AsString()
	
	var resp *api.ChainQueryResponse
	err := client.RequestAPIv3(ctx, "query", req, &resp)
	if err == nil && resp != nil && len(resp.Records) > 0 {
		if ledger, ok := resp.Records[0].Value.(*protocol.SystemLedger); ok {
			info.Height = ledger.Index
		}
	}

	return info, err
}

func (ds *DashboardServer) getCrosschainInfo(ctx context.Context, source, dest, msgType string) (CrosschainInfo, error) {
	info := CrosschainInfo{
		Source:      source,
		Destination: dest,
		Type:        msgType,
	}

	// Get source sequence
	sourceClient, ok := ds.clients[source]
	if !ok {
		return info, fmt.Errorf("no client for %s", source)
	}

	destUrl := protocol.PartitionUrl(strings.ToLower(dest))
	anchorSeqUrl := protocol.PartitionUrl(strings.ToLower(source)).
		JoinPath(protocol.AnchorPool).
		JoinPath(destUrl.String())

	req := new(api.GeneralQuery)
	req.Url = anchorSeqUrl.AsString()
	
	var resp *api.ChainQueryResponse
	err := sourceClient.RequestAPIv3(ctx, "query", req, &resp)
	if err == nil && resp != nil && len(resp.Records) > 0 {
		if pool, ok := resp.Records[0].Value.(*protocol.AnchorLedger); ok {
			info.SourceHeight = pool.Produced
		}
	}

	// Get destination sequence
	destClient, ok := ds.clients[dest]
	if !ok {
		return info, nil
	}

	srcUrl := protocol.PartitionUrl(strings.ToLower(source))
	anchorSeqUrl = protocol.PartitionUrl(strings.ToLower(dest)).
		JoinPath(protocol.AnchorPool).
		JoinPath(srcUrl.String())

	req = new(api.GeneralQuery)
	req.Url = anchorSeqUrl.AsString()
	
	err = destClient.RequestAPIv3(ctx, "query", req, &resp)
	if err == nil && resp != nil && len(resp.Records) > 0 {
		if pool, ok := resp.Records[0].Value.(*protocol.AnchorLedger); ok {
			info.DestHeight = pool.Received
		}
	}

	return info, nil
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

	// Execute pause/resume command
	action := "resume"
	if req.Pause {
		action = "pause"
	}

	// Call the debug endpoint to pause/resume (if available)
	partitionURL := ""
	switch req.Partition {
	case "DN":
		partitionURL = "http://127.0.0.1:27004"
	case "BVN1":
		partitionURL = "http://127.0.1.1:27001"
	case "BVN2":
		partitionURL = "http://127.0.2.1:27001"
	case "BVN3":
		partitionURL = "http://127.0.3.1:27001"
	case "BVN4":
		partitionURL = "http://127.0.4.1:27001"
	}

	if partitionURL != "" {
		pauseURL := fmt.Sprintf("%s/debug/ccc/%s", partitionURL, action)
		httpReq, _ := http.NewRequest("POST", pauseURL, nil)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("Failed to %s partition %s: %v", action, req.Partition, err)
		} else {
			resp.Body.Close()
		}
	}

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
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 Accumulate Devnet Dashboard</h1>
        
        <div class="grid" id="partitions"></div>
        
        <div class="crosschain-table">
            <h2 style="margin-bottom: 15px;">Crosschain Message Tracking</h2>
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
                        ${p.isPaused ? '▶ Resume' : '⏸ Pause'}
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
	server.initClients()

	// Start background updater
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			server.updateData(ctx)
			cancel()
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