package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nsega/mcp-obsidian/internal/handler"
	"github.com/nsega/mcp-obsidian/internal/server"
	"github.com/nsega/mcp-obsidian/internal/vault"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: mcp-obsidian <vault-path>")
	}

	v, err := vault.New(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to initialize vault: %v", err)
	}

	h := handler.New(v)
	srv := server.New(h, Version)

	fmt.Fprintf(os.Stderr, "MCP Obsidian server starting...\n")
	fmt.Fprintf(os.Stderr, "Vault path: %s\n", v.Path)

	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
