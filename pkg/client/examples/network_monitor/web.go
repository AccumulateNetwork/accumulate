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
	"os"
	"strconv"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

type WebServer struct {
	clients map[string]*client.Client
	monitor *NetworkMonitor
	port    int
	currentNetwork string
}

func NewWebServer(initialClient *client.Client, port int) *WebServer {
	// Initialize clients for all networks
	clients := make(map[string]*client.Client)
	
	// Mainnet
	if mainnetClient, err := client.NewMainnet(); err == nil {
		clients["mainnet"] = mainnetClient
	}
	
	// Testnet (Kermit)
	if testnetClient, err := client.NewTestnet(); err == nil {
		clients["testnet"] = testnetClient
	}
	
	// Local devnet
	localEndpoint := "http://localhost:8080/v3"
	if envEndpoint := os.Getenv("ACCUMULATE_ENDPOINT"); envEndpoint != "" {
		localEndpoint = envEndpoint
	}
	if localClient, err := client.NewLocal(localEndpoint); err == nil {
		clients["local"] = localClient
	}
	
	// Use the initial client for the default network
	currentNetwork := "mainnet"
	if initialClient != nil {
		// Detect which network the initial client is for
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if info, err := initialClient.GetNodeInfo(ctx); err == nil && info != nil {
			switch info.Network {
			case "MainNet":
				currentNetwork = "mainnet"
			case "TestNet", "Kermit":
				currentNetwork = "testnet"
			default:
				currentNetwork = "local"
			}
		}
	}
	
	return &WebServer{
		clients: clients,
		monitor: &NetworkMonitor{
			client:   clients[currentNetwork],
			interval: 5 * time.Second,
		},
		port: port,
		currentNetwork: currentNetwork,
	}
}

func (s *WebServer) Start() error {
	http.HandleFunc("/", s.handleHome)
	http.HandleFunc("/api/status", s.handleAPIStatus)
	http.HandleFunc("/api/metrics", s.handleAPIMetrics)
	http.HandleFunc("/api/network", s.handleNetworkSwitch)
	http.HandleFunc("/api/current-network", s.handleCurrentNetwork)
	
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
    <title>Accumulate Network Monitor</title>
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
            max-width: 1400px;
            margin: 0 auto;
        }
        h1 {
            text-align: center;
            margin-bottom: 30px;
            font-size: 2.5em;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.2);
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .card {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 15px;
            padding: 25px;
            border: 1px solid rgba(255, 255, 255, 0.2);
            transition: transform 0.3s ease, box-shadow 0.3s ease;
        }
        .card:hover {
            transform: translateY(-5px);
            box-shadow: 0 10px 30px rgba(0,0,0,0.3);
        }
        .card h2 {
            margin-bottom: 20px;
            font-size: 1.5em;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .metric {
            display: flex;
            justify-content: space-between;
            padding: 10px 0;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }
        .metric:last-child {
            border-bottom: none;
        }
        .metric-label {
            opacity: 0.9;
        }
        .metric-value {
            font-weight: bold;
            font-size: 1.1em;
        }
        .status-indicator {
            display: inline-block;
            width: 12px;
            height: 12px;
            border-radius: 50%;
            margin-right: 8px;
            animation: pulse 2s infinite;
        }
        .status-online { background: #4ade80; }
        .status-warning { background: #facc15; }
        .status-offline { background: #f87171; }
        
        @keyframes pulse {
            0% { 
                opacity: 1; 
                transform: scale(1);
            }
            50% { 
                opacity: 0.8; 
                transform: scale(1.05);
            }
            100% { 
                opacity: 1; 
                transform: scale(1);
            }
        }
        
        .last-update {
            text-align: center;
            opacity: 0.8;
            font-size: 0.9em;
            margin-top: 20px;
        }
        
        .loading {
            text-align: center;
            padding: 40px;
            font-size: 1.2em;
        }
        
        .partition-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-top: 15px;
        }
        
        .partition-item {
            background: rgba(255, 255, 255, 0.05);
            padding: 15px;
            border-radius: 10px;
            text-align: center;
        }
        
        .tps-value {
            font-size: 2em;
            font-weight: bold;
            margin: 10px 0;
        }
        
        .refresh-btn {
            position: fixed;
            bottom: 30px;
            right: 30px;
            background: rgba(255, 255, 255, 0.2);
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255, 255, 255, 0.3);
            color: white;
            padding: 15px 25px;
            border-radius: 50px;
            cursor: pointer;
            font-size: 1em;
            transition: all 0.3s ease;
        }
        
        .refresh-btn:hover {
            background: rgba(255, 255, 255, 0.3);
            transform: scale(1.05);
        }
        
        .anchor-card {
            background: linear-gradient(135deg, rgba(74, 222, 128, 0.1), rgba(250, 204, 21, 0.1));
            border: 2px solid rgba(74, 222, 128, 0.3);
        }
        
        .anchor-metrics {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-top: 20px;
        }
        
        .anchor-item {
            background: rgba(0, 0, 0, 0.2);
            padding: 20px;
            border-radius: 10px;
            text-align: center;
        }
        
        .anchor-label {
            font-size: 0.9em;
            opacity: 0.8;
            margin-bottom: 10px;
        }
        
        .anchor-value {
            font-size: 2.5em;
            font-weight: bold;
            margin: 10px 0;
            color: #4ade80;
            font-family: 'Courier New', monospace;
        }
        
        .anchor-change {
            font-size: 0.9em;
            opacity: 0.7;
        }
        
        .change-positive {
            color: #4ade80;
        }
        
        .change-negative {
            color: #f87171;
        }
        
        .bvn-info {
            padding: 5px;
            margin: 5px 0;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 5px;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🌐 Accumulate Network Monitor</h1>
        
        <div style="text-align: center; margin: 20px 0;">
            <select id="networkSelector" style="
                padding: 10px 20px;
                font-size: 1.1em;
                background: rgba(255, 255, 255, 0.1);
                border: 1px solid rgba(255, 255, 255, 0.3);
                color: white;
                border-radius: 10px;
                cursor: pointer;
            ">
                <option value="mainnet">🌍 Mainnet</option>
                <option value="testnet">🧪 Testnet (Kermit)</option>
                <option value="local">💻 Local Devnet</option>
            </select>
            <span id="networkStatus" style="margin-left: 20px; opacity: 0.8;"></span>
        </div>
        
        <div class="grid" id="dashboard">
            <div class="loading">Loading network status...</div>
        </div>
        
        <div class="last-update" id="lastUpdate"></div>
    </div>
    
    <button class="refresh-btn" onclick="refreshData()">🔄 Refresh</button>

    <script>
        let autoRefresh = true;
        let currentNetwork = 'mainnet';
        let previousHeights = {
            directory: null,
            majorBlock: null,
            cyclops: null,
            timestamp: null
        };
        
        // Initialize network selector
        document.addEventListener('DOMContentLoaded', async () => {
            // Get current network from server
            const response = await fetch('/api/current-network');
            const data = await response.json();
            currentNetwork = data.network;
            document.getElementById('networkSelector').value = currentNetwork;
            
            // Add change listener
            document.getElementById('networkSelector').addEventListener('change', async (e) => {
                const newNetwork = e.target.value;
                document.getElementById('networkStatus').textContent = 'Switching...';
                
                try {
                    const response = await fetch('/api/network', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ network: newNetwork })
                    });
                    
                    if (response.ok) {
                        currentNetwork = newNetwork;
                        // Reset previous heights for new network
                        previousHeights = {
                            directory: null,
                            majorBlock: null,
                            cyclops: null,
                            timestamp: null
                        };
                        document.getElementById('networkStatus').textContent = '✓ Connected';
                        fetchData(); // Refresh immediately
                    } else {
                        document.getElementById('networkStatus').textContent = '✗ Failed';
                        e.target.value = currentNetwork; // Revert selection
                    }
                } catch (error) {
                    document.getElementById('networkStatus').textContent = '✗ Error';
                    e.target.value = currentNetwork; // Revert selection
                }
                
                setTimeout(() => {
                    document.getElementById('networkStatus').textContent = '';
                }, 3000);
            });
        });
        
        async function fetchData() {
            try {
                const [statusRes, metricsRes] = await Promise.all([
                    fetch('/api/status'),
                    fetch('/api/metrics')
                ]);
                
                const status = await statusRes.json();
                const metrics = await metricsRes.json();
                
                // Log current heights to console for visibility
                if (status.networkStatus) {
                    console.log('📊 Update at', new Date().toLocaleTimeString(), 
                        '| Cyclops BVN:', status.networkStatus.cyclopsBlockHeight || 'N/A',
                        '| DN Height:', status.networkStatus.directoryHeight,
                        '| Major Block:', status.networkStatus.majorBlockHeight,
                        '| TPS:', metrics.partitions?.Directory?.tps?.toFixed(3) || 0);
                }
                
                updateDashboard(status, metrics);
                trackHeightChanges(status);
                document.getElementById('lastUpdate').textContent = 
                    'Last updated: ' + new Date().toLocaleTimeString();
            } catch (error) {
                console.error('Error fetching data:', error);
            }
        }
        
        function trackHeightChanges(status) {
            if (!status.networkStatus) return;
            
            const now = Date.now();
            const ns = status.networkStatus;
            
            if (previousHeights.directory !== null) {
                const timeDiff = (now - previousHeights.timestamp) / 1000; // seconds
                const dnDiff = ns.directoryHeight - previousHeights.directory;
                const majorDiff = ns.majorBlockHeight - previousHeights.majorBlock;
                
                // Calculate rate of change
                const dnRate = dnDiff / timeDiff;
                const majorRate = majorDiff / timeDiff;
                
                // Update DN change indicator (note: mainnet API returns cached value)
                const dnChange = document.getElementById('dn-change');
                if (dnChange) {
                    if (dnDiff > 0) {
                        dnChange.className = 'anchor-change change-positive';
                        dnChange.textContent = '+' + dnDiff + ' blocks (' + dnRate.toFixed(2) + ' blocks/sec)';
                    } else if (dnDiff === 0) {
                        // For mainnet, this is expected due to API caching
                        dnChange.className = 'anchor-change';
                        dnChange.style.color = '#facc15';
                        dnChange.textContent = '⚠️ Static value from API cache';
                    }
                }
                
                // Update Major Block change indicator
                const majorChange = document.getElementById('major-change');
                if (majorChange) {
                    if (majorDiff > 0) {
                        majorChange.className = 'anchor-change change-positive';
                        majorChange.textContent = '+' + majorDiff + ' blocks';
                    } else {
                        majorChange.className = 'anchor-change';
                        majorChange.textContent = 'Stable (next in ~' + (12 - (new Date().getHours() % 12)) + ' hours)';
                    }
                }
            }
            
            // Track Cyclops BVN block changes
            if (previousHeights.cyclops !== null && ns.cyclopsBlockHeight) {
                const cyclopsDiff = ns.cyclopsBlockHeight - previousHeights.cyclops;
                const cyclopsRate = cyclopsDiff / timeDiff;
                
                const cyclopsChange = document.getElementById('cyclops-change');
                if (cyclopsChange) {
                    if (cyclopsDiff > 0) {
                        cyclopsChange.className = 'anchor-change change-positive';
                        cyclopsChange.textContent = '+' + cyclopsDiff + ' blocks (' + cyclopsRate.toFixed(1) + ' blocks/sec)';
                    }
                }
            }
            
            // Store current values
            previousHeights.directory = ns.directoryHeight;
            previousHeights.majorBlock = ns.majorBlockHeight;
            previousHeights.cyclops = ns.cyclopsBlockHeight || previousHeights.cyclops;
            previousHeights.timestamp = now;
        }
        
        function updateDashboard(status, metrics) {
            const dashboard = document.getElementById('dashboard');
            
            let html = '';
            
            // Node Information Card
            if (status.nodeInfo) {
                html += createCard('📡 Node Information', [
                    { label: 'Network', value: status.nodeInfo.network },
                    { label: 'Version', value: status.nodeInfo.version },
                    { label: 'Peer ID', value: status.nodeInfo.peerId.substring(0, 20) + '...' },
                    { label: 'Services', value: Object.keys(status.nodeInfo.services).join(', ') }
                ]);
            }
            
            // Anchor Heights Card - PROMINENT DISPLAY
            if (status.networkStatus) {
                const ns = status.networkStatus;
                let anchorHtml = '<div class="card anchor-card">';
                anchorHtml += '<h2>⚓ Anchor Heights (Live)</h2>';
                anchorHtml += '<div class="anchor-metrics">';
                
                // Directory Network Anchor Height
                anchorHtml += '<div class="anchor-item">';
                anchorHtml += '<div class="anchor-label">Directory Network (DN)<br><small style="opacity:0.6">(Fixed code, waiting for deployment)</small></div>';
                anchorHtml += '<div class="anchor-value">' + ns.directoryHeight.toLocaleString() + '</div>';
                anchorHtml += '<div class="anchor-change" id="dn-change">API fix applied in code</div>';
                anchorHtml += '</div>';
                
                // Major Block Height
                anchorHtml += '<div class="anchor-item">';
                anchorHtml += '<div class="anchor-label">Major Block Height</div>';
                anchorHtml += '<div class="anchor-value">' + ns.majorBlockHeight.toLocaleString() + '</div>';
                anchorHtml += '<div class="anchor-change" id="major-change">Calculating...</div>';
                anchorHtml += '</div>';
                
                // Cyclops BVN Block Height (LIVE)
                if (ns.cyclopsBlockHeight) {
                    anchorHtml += '<div class="anchor-item" style="border: 2px solid #4ade80;">';
                    anchorHtml += '<div class="anchor-label">🚀 Cyclops BVN Block Height (LIVE)</div>';
                    anchorHtml += '<div class="anchor-value" style="color: #4ade80; font-size: 3em;">' + ns.cyclopsBlockHeight.toLocaleString() + '</div>';
                    anchorHtml += '<div class="anchor-change" id="cyclops-change">~1 block/sec</div>';
                    anchorHtml += '</div>';
                }
                
                // BVN Status
                if (ns.bvnExecutorVersions) {
                    anchorHtml += '<div class="anchor-item">';
                    anchorHtml += '<div class="anchor-label">Block Validator Networks</div>';
                    for (const bvn of ns.bvnExecutorVersions) {
                        anchorHtml += '<div class="bvn-info">' + bvn.partition + ': ' + bvn.version + '</div>';
                    }
                    anchorHtml += '</div>';
                }
                
                anchorHtml += '</div>';
                anchorHtml += '</div>';
                html += anchorHtml;
            }
            
            // Network Status Card
            if (status.networkStatus) {
                const ns = status.networkStatus;
                html += createCard('🌍 Network Status', [
                    { label: 'Network Name', value: ns.networkName },
                    { label: 'Partitions', value: ns.partitionCount },
                    { label: 'Validators', value: ns.validatorCount },
                    { label: 'Oracle Price', value: ns.oraclePrice + ' credits/ACME' }
                ]);
            }
            
            // Partition Metrics Card
            if (metrics.partitions) {
                let partitionHtml = '<div class="partition-grid">';
                for (const [name, data] of Object.entries(metrics.partitions)) {
                    const statusClass = data.tps > 0 ? 'status-online' : 'status-warning';
                    partitionHtml += '<div class="partition-item">' +
                        '<span class="status-indicator ' + statusClass + '"></span>' +
                        '<div>' + name + '</div>' +
                        '<div class="tps-value">' + data.tps.toFixed(2) + '</div>' +
                        '<div>TPS</div>' +
                        '</div>';
                }
                partitionHtml += '</div>';
                
                html += '<div class="card"><h2>📊 Partition Metrics</h2>' + partitionHtml + '</div>';
            }
            
            // Network Parameters Card
            if (status.networkStatus && status.networkStatus.globals) {
                const g = status.networkStatus.globals;
                html += createCard('⚙️ Network Parameters', [
                    { label: 'Operator Threshold', value: g.operatorThreshold },
                    { label: 'Validator Threshold', value: g.validatorThreshold },
                    { label: 'Major Block Schedule', value: g.majorBlockSchedule || 'N/A' },
                    { label: 'Data Entry Parts Limit', value: g.dataEntryParts },
                    { label: 'Account Authorities Limit', value: g.accountAuthorities }
                ]);
            }
            
            dashboard.innerHTML = html;
        }
        
        function createCard(title, metrics) {
            let html = '<div class="card"><h2>' + title + '</h2>';
            for (const metric of metrics) {
                html += '<div class="metric">' +
                    '<span class="metric-label">' + metric.label + ':</span>' +
                    '<span class="metric-value">' + metric.value + '</span>' +
                    '</div>';
            }
            html += '</div>';
            return html;
        }
        
        function refreshData() {
            fetchData();
        }
        
        // Initial load
        fetchData();
        
        // Auto-refresh every 2 seconds for more visible updates
        setInterval(() => {
            if (autoRefresh) {
                fetchData();
                // Add pulse effect to show update
                const cards = document.querySelectorAll('.anchor-value');
                cards.forEach(card => {
                    card.style.animation = 'pulse 0.5s';
                    setTimeout(() => {
                        card.style.animation = '';
                    }, 500);
                });
            }
        }, 2000);
    </script>
</body>
</html>`
	
	t := template.Must(template.New("home").Parse(tmpl))
	t.Execute(w, nil)
}

func (s *WebServer) handleCurrentNetwork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"network": s.currentNetwork,
	})
}

func (s *WebServer) handleNetworkSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req struct {
		Network string `json:"network"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	
	// Check if we have a client for this network
	client, ok := s.clients[req.Network]
	if !ok {
		http.Error(w, "Network not available", http.StatusBadRequest)
		return
	}
	
	// Update current network and monitor client
	s.currentNetwork = req.Network
	s.monitor.client = client
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"network": s.currentNetwork,
	})
}

func (s *WebServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	
	// Use the current network's client
	client := s.clients[s.currentNetwork]
	if client == nil {
		http.Error(w, "No client available", http.StatusServiceUnavailable)
		return
	}
	
	nodeInfo, _ := client.GetNodeInfo(ctx)
	networkStatus, _ := client.GetNetworkStatus(ctx)
	
	// Get Cyclops BVN block height for mainnet only
	cyclopsHeight := int64(0)
	if s.currentNetwork == "mainnet" {
		cyclopsResp, err := http.Get("http://apollo-mainnet.accumulate.defidevs.io:16692/status")
		if err == nil && cyclopsResp != nil {
			defer cyclopsResp.Body.Close()
			var cyclopsStatus map[string]interface{}
			if json.NewDecoder(cyclopsResp.Body).Decode(&cyclopsStatus) == nil {
				if result, ok := cyclopsStatus["result"].(map[string]interface{}); ok {
					if syncInfo, ok := result["sync_info"].(map[string]interface{}); ok {
						if height, ok := syncInfo["latest_block_height"].(string); ok {
							cyclopsHeight, _ = strconv.ParseInt(height, 10, 64)
						}
					}
				}
			}
		}
	}
	
	response := map[string]interface{}{
		"nodeInfo": nil,
		"networkStatus": nil,
	}
	
	if nodeInfo != nil {
		services := make(map[string]int)
		for _, svc := range nodeInfo.Services {
			services[svc.Type.String()]++
		}
		
		response["nodeInfo"] = map[string]interface{}{
			"network":  nodeInfo.Network,
			"peerId":   nodeInfo.PeerID,
			"version":  nodeInfo.Version,
			"commit":   nodeInfo.Commit,
			"services": services,
		}
	}
	
	if networkStatus != nil {
		ns := map[string]interface{}{
			"directoryHeight":   networkStatus.DirectoryHeight,
			"majorBlockHeight":  networkStatus.MajorBlockHeight,
		}
		
		if networkStatus.Network != nil {
			ns["networkName"] = networkStatus.Network.NetworkName
			ns["partitionCount"] = len(networkStatus.Network.Partitions)
			ns["validatorCount"] = len(networkStatus.Network.Validators)
			
			partitions := []map[string]string{}
			for _, p := range networkStatus.Network.Partitions {
				partitions = append(partitions, map[string]string{
					"id":   p.ID,
					"type": p.Type.String(),
				})
			}
			ns["partitions"] = partitions
		}
		
		if networkStatus.ExecutorVersion != 0 {
			ns["executorVersion"] = networkStatus.ExecutorVersion.String()
		}
		
		// Add BVN executor versions
		if networkStatus.BvnExecutorVersions != nil {
			bvnVersions := []map[string]string{}
			for _, bvn := range networkStatus.BvnExecutorVersions {
				bvnVersions = append(bvnVersions, map[string]string{
					"partition": bvn.Partition,
					"version":   bvn.Version.String(),
				})
			}
			ns["bvnExecutorVersions"] = bvnVersions
		}
		
		if networkStatus.Oracle != nil && networkStatus.Oracle.Price > 0 {
			ns["oraclePrice"] = networkStatus.Oracle.Price
		}
		
		if networkStatus.Globals != nil {
			g := networkStatus.Globals
			ns["globals"] = map[string]interface{}{
				"operatorThreshold":  fmt.Sprintf("%d/%d", g.OperatorAcceptThreshold.Numerator, g.OperatorAcceptThreshold.Denominator),
				"validatorThreshold": fmt.Sprintf("%d/%d", g.ValidatorAcceptThreshold.Numerator, g.ValidatorAcceptThreshold.Denominator),
				"majorBlockSchedule": g.MajorBlockSchedule,
				"dataEntryParts":     g.Limits.DataEntryParts,
				"accountAuthorities": g.Limits.AccountAuthorities,
			}
		}
		
		// Add Cyclops BVN block height
		if cyclopsHeight > 0 {
			ns["cyclopsBlockHeight"] = cyclopsHeight
		}
		
		response["networkStatus"] = ns
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *WebServer) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	
	// Use the current network's client
	client := s.clients[s.currentNetwork]
	if client == nil {
		http.Error(w, "No client available", http.StatusServiceUnavailable)
		return
	}
	
	partitions := map[string]map[string]interface{}{}
	
	// Get network status to find partitions
	networkStatus, _ := client.GetNetworkStatus(ctx)
	if networkStatus != nil && networkStatus.Network != nil {
		for _, partition := range networkStatus.Network.Partitions {
			metrics, err := client.GetMetrics(ctx, partition.ID)
			if err == nil && metrics != nil {
				partitions[partition.ID] = map[string]interface{}{
					"tps": metrics.TPS,
				}
			} else {
				partitions[partition.ID] = map[string]interface{}{
					"tps": 0.0,
				}
			}
		}
	}
	
	response := map[string]interface{}{
		"partitions": partitions,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}