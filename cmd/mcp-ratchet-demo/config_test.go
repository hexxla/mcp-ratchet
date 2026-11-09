package main

import (
	"flag"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
)

func TestValidateDemoPath(t *testing.T) {
	for _, path := range []string{"/mcp", "/tools/mcp", "/mcp-v2"} {
		if err := validateDemoPath(path); err != nil {
			t.Fatalf("literal path %q rejected: %v", path, err)
		}
	}
	for _, path := range []string{"", "/", "mcp", "/mcp/", "/mcp//call", "/mcp/../call", "/mcp/{", "/mcp/{tool}", "/mcp?debug", "/mcp#fragment", "/mcp%2fcall", "/mcp\\call", "/mcp call", "/mcp\tcall", "/mcp\x00call"} {
		if err := validateDemoPath(path); err == nil {
			t.Fatalf("pattern or noncanonical path %q accepted", path)
		}
	}
}

func TestRunRejectsMalformedPathBeforeListening(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	originalArgs, originalFlags := os.Args, flag.CommandLine
	defer func() { os.Args, flag.CommandLine = originalArgs, originalFlags }()
	os.Args = []string{"mcp-ratchet-demo", "-addr", listener.Addr().String(), "-path", "/mcp/{"}
	flag.CommandLine = flag.NewFlagSet("mcp-ratchet-demo", flag.ContinueOnError)
	if err := run(slog.New(slog.DiscardHandler)); err == nil || !strings.Contains(err.Error(), "canonical literal path") {
		t.Fatalf("want path validation before binding an occupied address, got %v", err)
	}
}
