package server

// GetAllTools returns all available MCP tool definitions
func GetAllTools() []map[string]interface{} {
	return []map[string]interface{}{
		// Wallet Management Tools
		{
			"name":        "wallet_init",
			"description": "Initialize a new Accumulate wallet at the configured directory",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"password": map[string]interface{}{
						"type":        "string",
						"description": "Password to encrypt the wallet (optional if using --no-password)",
					},
					"no_password": map[string]interface{}{
						"type":        "boolean",
						"description": "Initialize wallet without password protection",
						"default":     false,
					},
				},
			},
		},
		{
			"name":        "wallet_vault_open",
			"description": "Open and unlock a vault in the wallet",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"vault": map[string]interface{}{
						"type":        "string",
						"description": "Name of the vault to open (default: 'default')",
						"default":     "default",
					},
					"password": map[string]interface{}{
						"type":        "string",
						"description": "Password to unlock the vault",
					},
				},
				"required": []string{"password"},
			},
		},
		{
			"name":        "wallet_vault_lock",
			"description": "Lock the currently opened vault",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "wallet_generate_key",
			"description": "Generate a new key pair in the wallet (requires unlocked vault)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key_name": map[string]interface{}{
						"type":        "string",
						"description": "Name for the new key",
					},
				},
				"required": []string{"key_name"},
			},
		},
		{
			"name":        "wallet_list_keys",
			"description": "List all keys in the wallet (requires unlocked vault)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "wallet_set_network",
			"description": "Set the network for wallet operations (mainnet, testnet, devnet, or custom URL)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network name: mainnet, testnet, devnet, or custom RPC URL",
					},
				},
				"required": []string{"network"},
			},
		},
		{
			"name":        "wallet_get_status",
			"description": "Get current wallet and network status",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{},
			},
		},

		// Original 4 tools
		{
			"name":        "accumulate_query_account",
			"description": "Query an Accumulate account by URL to get account details, balance, and state",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The Accumulate account URL (e.g., acc://example.acme/tokens)",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to query (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "accumulate_query_tx",
			"description": "Query a transaction by hash to get transaction details and status",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"txid": map[string]interface{}{
						"type":        "string",
						"description": "The transaction ID/hash to query",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to query (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"txid"},
			},
		},
		{
			"name":        "accumulate_create_lite_account",
			"description": "Create a new Accumulate lite account URL from a public key",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"public_key": map[string]interface{}{
						"type":        "string",
						"description": "Public key in hex format for the lite account",
					},
				},
				"required": []string{"public_key"},
			},
		},
		{
			"name":        "accumulate_send_tokens",
			"description": "Send ACME tokens from one account to another",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"from": map[string]interface{}{
						"type":        "string",
						"description": "Source account URL",
					},
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Destination account URL",
					},
					"amount": map[string]interface{}{
						"type":        "string",
						"description": "Amount of ACME tokens to send (in ACME, not credits)",
					},
					"private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the source account",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"from", "to", "amount", "private_key"},
			},
		},

		// Tier 1: Core Query Tools
		{
			"name":        "accumulate_query_chain",
			"description": "Query chain entries for an account (transaction history, data entries, etc.)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The Accumulate account URL",
					},
					"chain_name": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Name of specific chain (e.g., 'main', 'signature', 'scratch')",
					},
					"index": map[string]interface{}{
						"type":        "number",
						"description": "Optional: Specific chain entry index",
					},
					"entry_hash": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Specific entry hash to query",
					},
					"start": map[string]interface{}{
						"type":        "number",
						"description": "Optional: Starting index for pagination",
					},
					"count": map[string]interface{}{
						"type":        "number",
						"description": "Optional: Number of results to return",
					},
					"expand": map[string]interface{}{
						"type":        "boolean",
						"description": "Optional: Expand nested values",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "accumulate_query_data",
			"description": "Query data entries from a data account",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The data account URL",
					},
					"index": map[string]interface{}{
						"type":        "number",
						"description": "Optional: Specific data entry index",
					},
					"entry_hash": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Specific entry hash",
					},
					"start": map[string]interface{}{
						"type": "number",
					},
					"count": map[string]interface{}{
						"type": "number",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "accumulate_query_directory",
			"description": "Query the directory of an identity (lists sub-accounts)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The identity URL",
					},
					"start": map[string]interface{}{
						"type": "number",
					},
					"count": map[string]interface{}{
						"type": "number",
					},
					"expand": map[string]interface{}{
						"type": "boolean",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "accumulate_query_pending",
			"description": "Query pending transactions for an account",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The account URL",
					},
					"start": map[string]interface{}{
						"type": "number",
					},
					"count": map[string]interface{}{
						"type": "number",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "accumulate_query_minor_block",
			"description": "Query a minor block by partition and optional index",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"partition": map[string]interface{}{
						"type":        "string",
						"description": "Partition name (e.g., 'BVN0', 'Directory')",
					},
					"index": map[string]interface{}{
						"type":        "number",
						"description": "Optional: Block index (omit for latest)",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"partition"},
			},
		},
		{
			"name":        "accumulate_query_major_block",
			"description": "Query a major block by partition and optional index",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"partition": map[string]interface{}{
						"type":        "string",
						"description": "Partition name",
					},
					"index": map[string]interface{}{
						"type":        "number",
						"description": "Optional: Block index (omit for latest)",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"partition"},
			},
		},

		// Tier 3: Network Status Tools
		{
			"name":        "accumulate_node_info",
			"description": "Get information about the current Accumulate node",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
			},
		},
		{
			"name":        "accumulate_network_status",
			"description": "Get overall network status and globals (oracle price, routing table, etc.)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
			},
		},
		{
			"name":        "accumulate_consensus_status",
			"description": "Get consensus status of a partition",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"partition": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Specific partition",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
			},
		},
		{
			"name":        "accumulate_metrics",
			"description": "Get network metrics (TPS, transaction counts, etc.)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"partition": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Specific partition",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
			},
		},
		{
			"name":        "accumulate_faucet",
			"description": "Request ACME tokens from the testnet faucet",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Account URL to receive tokens",
					},
					"token": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Faucet token/captcha",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "testnet",
					},
				},
				"required": []string{"url"},
			},
		},

		// Tier 4: Advanced Search Tools
		{
			"name":        "accumulate_search_public_key",
			"description": "Search for accounts associated with a public key",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"public_key": map[string]interface{}{
						"type":        "string",
						"description": "Public key in hex format",
					},
					"start": map[string]interface{}{
						"type": "number",
					},
					"count": map[string]interface{}{
						"type": "number",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"public_key"},
			},
		},
		{
			"name":        "accumulate_search_public_key_hash",
			"description": "Search for accounts associated with a public key hash",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"public_key_hash": map[string]interface{}{
						"type":        "string",
						"description": "Public key hash in hex format",
					},
					"start": map[string]interface{}{
						"type": "number",
					},
					"count": map[string]interface{}{
						"type": "number",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"public_key_hash"},
			},
		},
		{
			"name":        "accumulate_search_anchor",
			"description": "Search for anchor transactions across the network",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"anchor": map[string]interface{}{
						"type":        "string",
						"description": "Anchor hash to search for",
					},
					"start": map[string]interface{}{
						"type": "number",
					},
					"count": map[string]interface{}{
						"type": "number",
					},
					"network": map[string]interface{}{
						"type":    "string",
						"default": "mainnet",
					},
				},
				"required": []string{"anchor"},
			},
		},

		// Phase 1: KeyBook and KeyPage Query Tools
		{
			"name":        "accumulate_query_keybook",
			"description": "Query a KeyBook account to see its KeyPages and authority structure",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The KeyBook URL (e.g., acc://alice.acme/book)",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to query (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "accumulate_query_keypage",
			"description": "Query a KeyPage to see its keys, weights, and signature thresholds",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The KeyPage URL (e.g., acc://alice.acme/book/1)",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to query (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url"},
			},
		},

		// Phase 1.5: ADI Management Tools
		{
			"name":        "accumulate_generate_key",
			"description": "Generate a new ED25519 key pair for creating lite accounts or ADIs",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "accumulate_add_credits",
			"description": "Add credits to an account (lite or ADI) by converting ACME tokens",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"recipient": map[string]interface{}{
						"type":        "string",
						"description": "Account URL to receive credits (e.g., acc://alice.acme/book or lite account)",
					},
					"payer": map[string]interface{}{
						"type":        "string",
						"description": "Account URL that pays for the credits (must have ACME tokens)",
					},
					"amount": map[string]interface{}{
						"type":        "string",
						"description": "Amount of ACME tokens to convert to credits",
					},
					"private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the payer account",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"recipient", "payer", "amount", "private_key"},
			},
		},
		{
			"name":        "accumulate_create_adi",
			"description": "Create a new Accumulate Digital Identifier (ADI) with a KeyBook and initial KeyPage",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The ADI URL to create (e.g., acc://myadi.acme)",
					},
					"public_key": map[string]interface{}{
						"type":        "string",
						"description": "Public key in hex format for the initial KeyPage",
					},
					"sponsor": map[string]interface{}{
						"type":        "string",
						"description": "Sponsor account URL (must have credits to create ADI)",
					},
					"sponsor_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the sponsor account",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url", "public_key", "sponsor", "sponsor_private_key"},
			},
		},

		// Phase 2: Data & Token Account Operations
		{
			"name":        "accumulate_create_data_account",
			"description": "Create a data account under an ADI for storing arbitrary data",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Data account URL (e.g., acc://myadi.acme/data)",
					},
					"sponsor": map[string]interface{}{
						"type":        "string",
						"description": "Sponsor account URL (must have credits)",
					},
					"sponsor_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the sponsor account",
					},
					"authorities": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Optional: Additional authority URLs",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url", "sponsor", "sponsor_private_key"},
			},
		},
		{
			"name":        "accumulate_write_data",
			"description": "Write data entry to a data account",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"account_url": map[string]interface{}{
						"type":        "string",
						"description": "Data account URL",
					},
					"data": map[string]interface{}{
						"type":        "string",
						"description": "Data to write (hex, base64, or UTF-8 string)",
					},
					"encoding": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"hex", "base64", "utf8"},
						"default":     "utf8",
						"description": "Data encoding format",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"write_to_state": map[string]interface{}{
						"type":        "boolean",
						"default":     false,
						"description": "Write data to account state (persistent)",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"account_url", "data", "signer", "signer_private_key"},
			},
		},
		{
			"name":        "accumulate_create_token_account",
			"description": "Create a token account under an ADI",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Token account URL (e.g., acc://myadi.acme/tokens)",
					},
					"token_url": map[string]interface{}{
						"type":        "string",
						"description": "Token type URL (e.g., acc://ACME)",
						"default":     "acc://ACME",
					},
					"sponsor": map[string]interface{}{
						"type":        "string",
						"description": "Sponsor account URL (must have credits)",
					},
					"sponsor_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the sponsor account",
					},
					"authorities": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Optional: Authority URLs (defaults to ADI's KeyBook)",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url", "sponsor", "sponsor_private_key"},
			},
		},

		// Phase 3: Key Management Operations
		{
			"name":        "accumulate_create_keypage",
			"description": "Create a new KeyPage in an existing KeyBook to expand multisig authority",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"keybook_url": map[string]interface{}{
						"type":        "string",
						"description": "KeyBook URL to add the KeyPage to (e.g., acc://myadi.acme/book)",
					},
					"keys": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Public keys (hex format) for the new KeyPage",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"keybook_url", "keys", "signer", "signer_private_key"},
			},
		},
		{
			"name":        "accumulate_update_keypage",
			"description": "Update an existing KeyPage by adding/removing keys or setting threshold for multisig",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"keypage_url": map[string]interface{}{
						"type":        "string",
						"description": "KeyPage URL to update (e.g., acc://myadi.acme/book/1)",
					},
					"operation": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"add", "remove", "set_threshold"},
						"description": "Operation to perform: add/remove key, or set_threshold",
					},
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Public key hash (hex) to add or remove (required for add/remove)",
					},
					"threshold": map[string]interface{}{
						"type":        "number",
						"description": "New signature threshold (required for set_threshold operation)",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"keypage_url", "operation", "signer", "signer_private_key"},
			},
		},
		{
			"name":        "accumulate_create_keybook",
			"description": "Create an additional KeyBook for an ADI to separate authorities for different operations",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "New KeyBook URL (e.g., acc://myadi.acme/book2)",
					},
					"public_key_hash": map[string]interface{}{
						"type":        "string",
						"description": "Initial public key hash (hex format) for the KeyBook",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url", "public_key_hash", "signer", "signer_private_key"},
			},
		},
		{
			"name":        "accumulate_update_account_auth",
			"description": "Manage account authorities by adding, removing, enabling, or disabling authority URLs",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"account_url": map[string]interface{}{
						"type":        "string",
						"description": "Account URL to update (e.g., acc://myadi.acme/tokens)",
					},
					"operations": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"type": map[string]interface{}{
									"type": "string",
									"enum": []string{"add", "remove", "enable", "disable"},
								},
								"authority": map[string]interface{}{
									"type": "string",
								},
							},
							"required": []string{"type", "authority"},
						},
						"description": "List of authority operations to perform",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"account_url", "operations", "signer", "signer_private_key"},
			},
		},

		// Phase 4: Token Issuance Operations
		{
			"name":        "accumulate_create_token",
			"description": "Create a new custom token type on the Accumulate network",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Token URL to create (e.g., acc://myadi.acme/my-token)",
					},
					"symbol": map[string]interface{}{
						"type":        "string",
						"description": "Token symbol (e.g., MTK)",
					},
					"precision": map[string]interface{}{
						"type":        "number",
						"description": "Token precision/decimals (e.g., 8 for BTC-style, 18 for ETH-style)",
						"default":     8,
					},
					"supply_limit": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Maximum token supply (0 for unlimited)",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"url", "symbol", "signer", "signer_private_key"},
			},
		},
		{
			"name":        "accumulate_issue_tokens",
			"description": "Issue (mint) new tokens to a recipient account",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"token_url": map[string]interface{}{
						"type":        "string",
						"description": "Token issuer URL (e.g., acc://myadi.acme/my-token)",
					},
					"recipient": map[string]interface{}{
						"type":        "string",
						"description": "Recipient account URL to receive tokens",
					},
					"amount": map[string]interface{}{
						"type":        "string",
						"description": "Amount of tokens to issue",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"token_url", "recipient", "amount", "signer", "signer_private_key"},
			},
		},
		{
			"name":        "accumulate_burn_tokens",
			"description": "Burn (destroy) tokens from an account",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"account_url": map[string]interface{}{
						"type":        "string",
						"description": "Token account URL to burn from",
					},
					"amount": map[string]interface{}{
						"type":        "string",
						"description": "Amount of tokens to burn",
					},
					"signer": map[string]interface{}{
						"type":        "string",
						"description": "Signing authority URL (e.g., acc://myadi.acme/book)",
					},
					"signer_private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format of the signer",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"account_url", "amount", "signer", "signer_private_key"},
			},
		},

		// Batch Submission Tools
		{
			"name":        "accumulate_submit_batch",
			"description": "Submit multiple transactions in a single envelope for convenience. WARNING: Each transaction is charged individually (no fee savings). WARNING: Transactions execute independently (no atomicity - partial failures possible).",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"transactions": map[string]interface{}{
						"type":        "array",
						"description": "Array of transactions to submit in a batch",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"type": map[string]interface{}{
									"type":        "string",
									"description": "Transaction type (sendTokens, createIdentity, createDataAccount, createTokenAccount, writeData)",
									"enum":        []string{"sendTokens", "createIdentity", "createDataAccount", "createTokenAccount", "writeData"},
								},
								"params": map[string]interface{}{
									"type":        "object",
									"description": "Transaction-specific parameters (see individual transaction tools for parameter details)",
								},
							},
							"required": []string{"type", "params"},
						},
						"minItems": 1,
					},
					"private_key": map[string]interface{}{
						"type":        "string",
						"description": "Private key in hex format to sign all transactions (0x prefix optional)",
					},
					"network": map[string]interface{}{
						"type":        "string",
						"description": "Network to use (mainnet, testnet, devnet, or custom RPC endpoint)",
						"default":     "mainnet",
					},
				},
				"required": []string{"transactions", "private_key"},
			},
		},

		// Historical Database Tools
		{
			"name":        "accumulate_db_list",
			"description": "List all known historical Accumulate databases with availability and size information",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "accumulate_db_query_account",
			"description": "Query an account directly from a historical database file (bypasses network). Database parameter is optional - will auto-route to the correct partition database if not specified.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Database name (e.g., 'dn-primary', 'bvn0-backup') or absolute path. If not specified, automatically routes to the correct database.",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Account URL to query (e.g., 'acc://ACME', 'acc://dn.acme')",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "accumulate_db_list_accounts",
			"description": "List accounts from a historical database's BPT (Binary Patricia Tree). Database parameter is optional - defaults to DN database if not specified.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Database name (e.g., 'dn-primary', 'bvn0-backup') or absolute path. Defaults to DN database if not specified.",
					},
					"limit": map[string]interface{}{
						"type":        "number",
						"description": "Maximum number of accounts to return (default: 100)",
						"default":     100,
					},
				},
				"required": []string{},
			},
		},
		{
			"name":        "accumulate_db_iterate_accounts",
			"description": "Paginated iteration over all accounts in a database with cursor support for processing large datasets",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Database name (e.g., 'dn-primary', 'bvn0-backup') or absolute path. Defaults to DN database if not specified.",
					},
					"cursor": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Pagination cursor from previous call (base64-encoded)",
					},
					"page_size": map[string]interface{}{
						"type":        "number",
						"description": "Number of accounts per page (default: 100, max: 1000)",
						"default":     100,
					},
				},
				"required": []string{},
			},
		},
		{
			"name":        "accumulate_db_full_scan",
			"description": "Perform a full database scan to extract ALL account URLs from keys and values (like staking extractor). Scans up to 2M database keys to find all account references, not just BPT-indexed accounts.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Required: Database name (e.g., '2025-07-13-dn', '2025-07-13-bvn0') or absolute path",
					},
					"max_keys": map[string]interface{}{
						"type":        "number",
						"description": "Maximum number of database keys to scan (default: 2000000)",
						"default":     2000000,
					},
				},
				"required": []string{"database"},
			},
		},
		{
			"name":        "accumulate_db_build_fulldb",
			"description": "Build a complete fulldb by extracting all BPT entries from source partition databases and creating a new BPT in the destination database. This combines all partitions (DN, BVN0, BVN1, BVN2) into a single database with a comprehensive BPT.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"snapshot": map[string]interface{}{
						"type":        "string",
						"description": "Required: Snapshot name/date (e.g., '2025-07-13', '2025-10-22'). Will extract from <snapshot>-dn, <snapshot>-bvn0, <snapshot>-bvn1, <snapshot>-bvn2",
					},
					"output": map[string]interface{}{
						"type":        "string",
						"description": "Required: Output database path (e.g., '/tmp/fulldb-2025-07-13.db')",
					},
				},
				"required": []string{"snapshot", "output"},
			},
		},
		{
			"name":        "accumulate_db_extract_accounts_batch",
			"description": "Extract multiple accounts efficiently in a single call with optional chain and transaction data",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Database name (e.g., 'dn-primary', 'bvn0-backup') or absolute path",
					},
					"accounts": map[string]interface{}{
						"type":        "array",
						"description": "Array of account URLs to extract",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"include_chains": map[string]interface{}{
						"type":        "boolean",
						"description": "Include chain data (default: false)",
						"default":     false,
					},
					"include_transactions": map[string]interface{}{
						"type":        "boolean",
						"description": "Include transactions (default: false)",
						"default":     false,
					},
				},
				"required": []string{"accounts"},
			},
		},
		{
			"name":        "accumulate_db_get_bpt_hash",
			"description": "Get the BPT (Binary Patricia Tree) root hash from a historical database",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Database name or absolute path to database directory",
					},
				},
				"required": []string{"database"},
			},
		},
		{
			"name":        "accumulate_db_query_transaction",
			"description": "Query a transaction directly from a historical database file (bypasses network)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Database name or absolute path to database directory",
					},
					"txid": map[string]interface{}{
						"type":        "string",
						"description": "Transaction ID (e.g., 'acc://account.acme@tx-hash')",
					},
				},
				"required": []string{"database", "txid"},
			},
		},
		{
			"name":        "accumulate_db_query_chain",
			"description": "Query chain entries from a historical database to get transaction hashes. Database parameter is optional - will auto-route to the correct partition database if not specified.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Database name (e.g., 'dn-primary', 'bvn0-backup') or absolute path. If not specified, automatically routes to the correct database.",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Account URL (e.g., 'acc://ACME', 'acc://dn.acme')",
					},
					"chain_name": map[string]interface{}{
						"type":        "string",
						"description": "Chain name (e.g., 'main', 'signature', 'index')",
					},
					"start": map[string]interface{}{
						"type":        "number",
						"description": "Starting entry index (default: 0)",
					},
					"count": map[string]interface{}{
						"type":        "number",
						"description": "Number of entries to retrieve (default: 10, max: 100)",
					},
				},
				"required": []string{"url", "chain_name"},
			},
		},
	}
}
