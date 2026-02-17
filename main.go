package main

import (
	"context"
	"log/slog"
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
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if len(os.Args) < 2 {
		logger.Error("usage: mcp-obsidian <vault-path>")
		os.Exit(1)
	}

	v, err := vault.New(os.Args[1])
	if err != nil {
		logger.Error("failed to initialize vault", "error", err)
		os.Exit(1)
	}

	h := handler.New(v, logger)
	srv := server.New(h, Version, logger)

	logger.Info("server starting", "vault", v.Path, "version", Version)

	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
