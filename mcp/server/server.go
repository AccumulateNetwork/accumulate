// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

const (
	ProtocolVersion = "2024-11-05"
	ServerName      = "accumulate-mcp"
	ServerVersion   = "1.0.0"
)

// ToolHandler is a function that handles a tool call
type ToolHandler func(args map[string]any) (ToolCallResult, error)

// Server is an MCP server that communicates over stdio
type Server struct {
	tools    map[string]Tool
	handlers map[string]ToolHandler

	resources        map[string]Resource
	resourceHandlers map[string]ResourceHandler

	mu sync.RWMutex

	input  io.Reader
	output io.Writer
	logger *log.Logger

	initialized bool

	// Key management for transaction signing
	keys *KeyManager
}

// Config holds server configuration
type Config struct {
	Input  io.Reader
	Output io.Writer
	Logger *log.Logger
}

// New creates a new MCP server
func New(cfg *Config) *Server {
	input := cfg.Input
	if input == nil {
		input = os.Stdin
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	logger := cfg.Logger
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	return &Server{
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
		input:    input,
		output:   output,
		logger:   logger,
		keys:     NewKeyManager(),
	}
}

// RegisterTool registers a tool with its handler
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// Run starts the server and processes messages until EOF or error
func (s *Server) Run() error {
	scanner := bufio.NewScanner(s.input)
	// Increase buffer size for large messages
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		response := s.handleMessage(line)
		if response != nil {
			if err := s.writeResponse(response); err != nil {
				s.logger.Printf("error writing response: %v", err)
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

// handleMessage processes a single JSON-RPC message
func (s *Server) handleMessage(data []byte) *JSONRPCResponse {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    ParseError,
				Message: "Parse error",
				Data:    err.Error(),
			},
		}
	}

	if req.JSONRPC != "2.0" {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    InvalidRequest,
				Message: "Invalid Request",
				Data:    "jsonrpc must be 2.0",
			},
		}
	}

	s.logger.Printf("handling method: %s", req.Method)

	var result any
	var rpcErr *JSONRPCError

	switch req.Method {
	case "initialize":
		result, rpcErr = s.handleInitialize(req.Params)
	case "initialized":
		// Notification, no response needed
		return nil
	case "tools/list":
		result, rpcErr = s.handleToolsList()
	case "tools/call":
		result, rpcErr = s.handleToolsCall(req.Params)
	case "resources/list":
		result, rpcErr = s.handleResourcesList()
	case "resources/read":
		result, rpcErr = s.handleResourcesRead(req.Params)
	case "ping":
		result = map[string]any{}
	default:
		rpcErr = &JSONRPCError{
			Code:    MethodNotFound,
			Message: "Method not found",
			Data:    req.Method,
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	}
}

func (s *Server) handleInitialize(params json.RawMessage) (any, *JSONRPCError) {
	var initParams InitializeParams
	if params != nil {
		if err := json.Unmarshal(params, &initParams); err != nil {
			return nil, &JSONRPCError{
				Code:    InvalidParams,
				Message: "Invalid params",
				Data:    err.Error(),
			}
		}
	}

	s.logger.Printf("client: %s %s", initParams.ClientInfo.Name, initParams.ClientInfo.Version)
	s.initialized = true

	caps := Capabilities{
		Tools: &ToolsCapability{},
	}

	s.mu.RLock()
	hasResources := len(s.resources) > 0
	s.mu.RUnlock()

	if hasResources {
		caps.Resources = &ResourcesCapability{}
	}

	return InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    caps,
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}, nil
}

func (s *Server) handleToolsList() (any, *JSONRPCError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}

	return ToolsListResult{Tools: tools}, nil
}

func (s *Server) handleToolsCall(params json.RawMessage) (any, *JSONRPCError) {
	var callParams ToolCallParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: "Invalid params",
			Data:    err.Error(),
		}
	}

	s.mu.RLock()
	handler, ok := s.handlers[callParams.Name]
	s.mu.RUnlock()

	if !ok {
		return NewErrorResult(fmt.Errorf("unknown tool: %s", callParams.Name)), nil
	}

	result, err := handler(callParams.Arguments)
	if err != nil {
		return NewErrorResult(err), nil
	}

	return result, nil
}

func (s *Server) writeResponse(resp *JSONRPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	data = append(data, '\n')
	_, err = s.output.Write(data)
	return err
}
