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

	// go-sdk v1.8.0 caps inbound JSON-RPC frames and ends the session on overflow. The
	// SDK default (16 MiB) is smaller than an escaped note body the handlers accept, so
	// the cap is sized by server.MaxFrameSize instead (see its doc for the escape hatch).
	transport := &mcp.StdioTransport{MaxLineLength: server.MaxFrameSize}
	if err := srv.Run(context.Background(), transport); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
