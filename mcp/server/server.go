package server

import (
	"encoding/json"
	"fmt"
	"log"

	cometLog "github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/mcp/wallet"
)

// Server represents the MCP server
type Server struct {
	name      string
	version   string
	state     *State
	wallet    *wallet.Client
	dbManager *DatabaseManager
}

// NewServer creates a new MCP server instance
func NewServer() *Server {
	// Load configuration
	cfg := LoadConfig()

	// Create state
	state := NewState(cfg)

	// Initialize wallet client
	walletClient, err := wallet.NewClient(cfg.WalletDir)
	if err != nil {
		log.Printf("Warning: Failed to initialize wallet client: %v", err)
		// Continue without wallet - will fail on wallet operations
	}

	log.Printf("MCP Server initialized:")
	log.Printf("  Wallet: %s", cfg.WalletDir)
	log.Printf("  Network: %s", cfg.Network)
	log.Printf("  Server: %s", cfg.Server)

	// Initialize database manager with connection pooling
	logger := cometLog.NewNopLogger()
	dbManager := NewDatabaseManager(logger)

	return &Server{
		name:      "mcp-accumulate",
		version:   "0.2.0",
		state:     state,
		wallet:    walletClient,
		dbManager: dbManager,
	}
}

// HandleRequest processes incoming MCP requests
func (s *Server) HandleRequest(request map[string]interface{}) map[string]interface{} {
	method, ok := request["method"].(string)
	if !ok {
		return s.errorResponse(request, -32600, "Invalid Request: missing method")
	}

	id := request["id"]

	switch method {
	case "initialize":
		return s.handleInitialize(id, request)
	case "tools/list":
		return s.handleListTools(id)
	case "tools/call":
		return s.handleCallTool(id, request)
	case "resources/list":
		return s.handleListResources(id)
	case "resources/read":
		return s.handleReadResource(id, request)
	case "prompts/list":
		return s.handleListPrompts(id)
	case "prompts/get":
		return s.handleGetPrompt(id, request)
	default:
		return s.errorResponse(request, -32601, fmt.Sprintf("Method not found: %s", method))
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(id interface{}, request map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]interface{}{
				"name":    s.name,
				"version": s.version,
			},
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
				"prompts":   map[string]interface{}{},
			},
		},
	}
}

// handleListTools returns the list of available tools
func (s *Server) handleListTools(id interface{}) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"tools": GetAllTools(),
		},
	}
}

// handleListPrompts returns the list of available prompts
func (s *Server) handleListPrompts(id interface{}) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"prompts": GetAllPrompts(),
		},
	}
}

// handleGetPrompt returns a specific prompt with arguments applied
func (s *Server) handleGetPrompt(id interface{}, request map[string]interface{}) map[string]interface{} {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		return s.errorResponse(request, -32602, "Invalid params")
	}

	promptName, ok := params["name"].(string)
	if !ok {
		return s.errorResponse(request, -32602, "Missing prompt name")
	}

	// Extract arguments
	args := make(map[string]string)
	if argsParam, ok := params["arguments"].(map[string]interface{}); ok {
		for k, v := range argsParam {
			if str, ok := v.(string); ok {
				args[k] = str
			} else {
				args[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	// Validate required arguments
	if err := ValidatePromptArguments(promptName, args); err != nil {
		return s.errorResponse(request, -32602, fmt.Sprintf("Invalid arguments: %v", err))
	}

	// Generate template
	template, err := GetPromptTemplate(promptName, args)
	if err != nil {
		return s.errorResponse(request, -32602, fmt.Sprintf("Failed to get prompt: %v", err))
	}

	// Return prompt result with messages array
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"description": fmt.Sprintf("Generated prompt: %s", promptName),
			"messages": []map[string]interface{}{
				{
					"role": "user",
					"content": map[string]interface{}{
						"type": "text",
						"text": template,
					},
				},
			},
		},
	}
}

// handleCallTool executes a tool
func (s *Server) handleCallTool(id interface{}, request map[string]interface{}) map[string]interface{} {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		return s.errorResponse(request, -32602, "Invalid params")
	}

	toolName, ok := params["name"].(string)
	if !ok {
		return s.errorResponse(request, -32602, "Missing tool name")
	}

	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		args = make(map[string]interface{})
	}

	result, err := s.executeTool(toolName, args)
	if err != nil {
		log.Printf("Tool execution error: %v", err)
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": fmt.Sprintf("Error: %v", err),
					},
				},
				"isError": true,
			},
		}
	}

	// Wrap result in MCP protocol format
	resultJSON, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to marshal result: %v", err)
		return s.errorResponse(request, -32603, fmt.Sprintf("Failed to marshal result: %v", err))
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": string(resultJSON),
				},
			},
		},
	}
}

// executeTool executes the specified tool with given arguments
func (s *Server) executeTool(name string, args map[string]interface{}) (map[string]interface{}, error) {
	switch name {
	// Wallet Management Tools
	case "wallet_init":
		return s.walletInit(args)
	case "wallet_vault_open":
		return s.vaultOpen(args)
	case "wallet_vault_lock":
		return s.vaultLock(args)
	case "wallet_generate_key":
		return s.walletGenerateKey(args)
	case "wallet_list_keys":
		return s.walletListKeys(args)
	case "wallet_set_network":
		return s.walletSetNetwork(args)
	case "wallet_get_status":
		return s.walletGetStatus(args)

	// Original 4 tools
	case "accumulate_query_account":
		return s.queryAccount(args)
	case "accumulate_query_tx":
		return s.queryTransaction(args)
	case "accumulate_create_lite_account":
		return s.createLiteAccount(args)
	case "accumulate_send_tokens":
		return s.sendTokens(args)

	// Tier 1: Core Query Tools
	case "accumulate_query_chain":
		return s.queryChain(args)
	case "accumulate_query_data":
		return s.queryData(args)
	case "accumulate_query_directory":
		return s.queryDirectory(args)
	case "accumulate_query_pending":
		return s.queryPending(args)
	case "accumulate_query_minor_block":
		return s.queryMinorBlock(args)
	case "accumulate_query_major_block":
		return s.queryMajorBlock(args)

	// Tier 3: Network Status Tools
	case "accumulate_node_info":
		return s.nodeInfo(args)
	case "accumulate_network_status":
		return s.networkStatus(args)
	case "accumulate_consensus_status":
		return s.consensusStatus(args)
	case "accumulate_metrics":
		return s.metrics(args)
	case "accumulate_faucet":
		return s.faucet(args)

	// Tier 4: Advanced Search Tools
	case "accumulate_search_public_key":
		return s.searchPublicKey(args)
	case "accumulate_search_public_key_hash":
		return s.searchPublicKeyHash(args)
	case "accumulate_search_anchor":
		return s.searchAnchor(args)

	// Phase 1: KeyBook and KeyPage Query Tools
	case "accumulate_query_keybook":
		return s.queryKeyBook(args)
	case "accumulate_query_keypage":
		return s.queryKeyPage(args)

	// Phase 1.5: ADI Management Tools
	case "accumulate_generate_key":
		return s.generateKey(args)
	case "accumulate_add_credits":
		return s.addCredits(args)
	case "accumulate_create_adi":
		return s.createADI(args)

	// Phase 2: Data & Token Account Operations
	case "accumulate_create_data_account":
		return s.createDataAccount(args)
	case "accumulate_write_data":
		return s.writeData(args)
	case "accumulate_create_token_account":
		return s.createTokenAccount(args)

	// Phase 3: Key Management Operations
	case "accumulate_create_keypage":
		return s.createKeyPage(args)
	case "accumulate_update_keypage":
		return s.updateKeyPage(args)
	case "accumulate_create_keybook":
		return s.createKeyBook(args)
	case "accumulate_update_account_auth":
		return s.updateAccountAuth(args)

	// Phase 4: Token Issuance Operations
	case "accumulate_create_token":
		return s.createToken(args)
	case "accumulate_issue_tokens":
		return s.issueTokens(args)
	case "accumulate_burn_tokens":
		return s.burnTokens(args)

	// Batch Submission
	case "accumulate_submit_batch":
		return s.submitBatch(args)

	// Historical Database Tools
	case "accumulate_db_list":
		return s.dbList(args)
	case "accumulate_db_query_account":
		return s.dbQueryAccount(args)
	case "accumulate_db_list_accounts":
		return s.dbListAccounts(args)
	case "accumulate_db_iterate_accounts":
		return s.dbIterateAccounts(args)
	case "accumulate_db_full_scan":
		return s.dbFullScan(args)
	case "accumulate_db_build_fulldb":
		return s.dbBuildFulldb(args)
	case "accumulate_db_extract_accounts_batch":
		return s.dbExtractAccountsBatch(args)
	case "accumulate_db_get_bpt_hash":
		return s.dbGetBPTHash(args)
	case "accumulate_db_query_transaction":
		return s.dbQueryTransaction(args)
	case "accumulate_db_query_chain":
		return s.dbQueryChain(args)

	// Follower Node Management (Docker-based)
	case "accumulate_init_follower":
		return s.initFollower(args)
	case "accumulate_restore_from_snapshots":
		return s.restoreFromSnapshots(args)
	case "accumulate_run_follower":
		return s.runFollower(args)
	case "accumulate_follower_status":
		return s.getFollowerStatus(args)
	case "accumulate_stop_follower":
		return s.stopFollower(args)
	case "accumulate_remove_follower":
		return s.removeFollower(args)

	// Accman Deployment Artifacts
	case "accumulate_prepare_accman_artifacts":
		return s.prepareAccmanArtifacts(args)
	case "accumulate_create_node_archive":
		return s.createNodeArchive(args)
	case "accumulate_get_bootstrap_peers":
		return s.getBootstrapPeers(args)
	case "accumulate_compare_bootstrap_peers":
		return s.compareBootstrapPeers(args)
	case "accumulate_get_genesis_files":
		return s.getGenesisFiles(args)

	// Build Tools
	case "accumulate_build_binary":
		return s.buildBinary(args)

	// Bootstrap Server Monitoring
	case "accumulate_query_bootstrap_server":
		return s.getBootstrapServerStatus(args)

	// Deployment Prerequisites and Monitoring
	case "accumulate_validate_prerequisites":
		return s.validatePrerequisites(args)
	case "accumulate_get_sync_progress":
		return s.getSyncProgress(args)
	case "accumulate_analyze_logs":
		return s.analyzeLogs(args)

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// errorResponse creates an error response
func (s *Server) errorResponse(request map[string]interface{}, code int, message string) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      request["id"],
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
}
