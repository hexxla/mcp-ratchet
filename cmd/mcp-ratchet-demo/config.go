package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// MCPConfig holds server-level configuration (separate from ratchet core config).
// These settings control how the MCP server exposes ratchet functionality.
type MCPConfig struct {
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

// validateDemoPath excludes ServeMux patterns and paths that require cleaning.
func validateDemoPath(endpoint string) error {
	if endpoint == "/" || !strings.HasPrefix(endpoint, "/") || path.Clean(endpoint) != endpoint || strings.ContainsAny(endpoint, "{}?#\\%") ||
		strings.ContainsFunc(endpoint, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return fmt.Errorf("demo MCP endpoint must be a canonical literal path: %q", endpoint)
	}
	u, err := url.ParseRequestURI(endpoint)
	if err != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("demo MCP endpoint must be a valid literal path: %q", endpoint)
	}
	return nil
}
