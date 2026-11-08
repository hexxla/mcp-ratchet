package main

import (
	"fmt"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

// MCPConfig holds server-level configuration (separate from ratchet core config).
// These settings control how the MCP server exposes ratchet functionality.
type MCPConfig struct {
	Observability struct {
		HTTPEnabled      bool   `yaml:"http_enabled"`
		WebSocketEnabled bool   `yaml:"websocket_enabled"`
		WebSocketPath    string `yaml:"websocket_path"`
	} `yaml:"observability"`
	Server struct {
		Addr    string `yaml:"addr"`
		MCPPath string `yaml:"mcp_path"`
	} `yaml:"server"`
}

func loadMCPConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 - path from flag, controlled by admin
	if err != nil {
		return nil, fmt.Errorf("failed to read MCP config: %w", err)
	}
	var cfg MCPConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse MCP config: %w", err)
	}
	// Set defaults
	if cfg.Observability.WebSocketPath == "" {
		cfg.Observability.WebSocketPath = "/observability/stream"
	}
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = "127.0.0.1:8080"
	}
	if cfg.Server.MCPPath == "" {
		cfg.Server.MCPPath = "/mcp"
	}
	return &cfg, nil
}

func validateDemoAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid demo listen address: %w", err)
	}
	if ip := net.ParseIP(host); !ip.IsLoopback() {
		return fmt.Errorf("demo listen address must use a loopback IP: %q", addr)
	}
	return nil
}
