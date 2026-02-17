# Migrate Logging to `log/slog`

**Status: COMPLETED**

## Context

The project currently uses the legacy `log` package and `fmt.Fprintf` for logging. Go 1.21+ provides `log/slog` as the standard structured logging solution. Additionally, the MCP Go SDK (v1.3.0) natively accepts `*slog.Logger` via `mcp.ServerOptions.Logger`, which enables automatic structured logging of server lifecycle events (session connect/disconnect, errors, client log level changes, etc.).

Currently `mcp.NewServer()` is called with `nil` options, leaving all SDK logging discarded.

## Goals

- Replace `log` and `fmt.Fprintf` with `log/slog`
- Pass logger to MCP SDK via `ServerOptions.Logger`
- Inject logger into `Handler` for operational visibility
- Log currently-swallowed errors in `filepath.Walk` callbacks
- Zero new dependencies (slog is stdlib)

## Files Changed

```
main.go                     — create slog.Logger, replace log/fmt calls
internal/server/server.go   — accept *slog.Logger, pass to ServerOptions
internal/handler/handler.go — add Logger field, log swallowed errors
```

## Implementation Steps

### Step 1: Update `internal/server/server.go` ✅

Accept a `*slog.Logger` parameter and pass it to `mcp.ServerOptions`:

```go
func New(h *handler.Handler, version string, logger *slog.Logger) *mcp.Server {
    srv := mcp.NewServer(&mcp.Implementation{
        Name:    "mcp-obsidian",
        Version: version,
    }, &mcp.ServerOptions{
        Logger: logger,
    })
    // ... tool registrations unchanged
    return srv
}
```

**Rationale:** The SDK will use this logger for built-in events like `"server session connected"`, `"server session disconnected"`, `"server connect error"`, etc. — all structured with key-value pairs.

### Step 2: Add `*slog.Logger` to `internal/handler/handler.go` ✅

Add logger field to `Handler` and accept it in constructor:

```go
type Handler struct {
    Vault  *vault.Vault
    Logger *slog.Logger
}

func New(v *vault.Vault, logger *slog.Logger) *Handler {
    return &Handler{Vault: v, Logger: logger}
}
```

Then replace all silently-swallowed errors in `filepath.Walk` callbacks with debug-level logging. There are 8 locations across 4 handler methods:

**SearchNotes** (line ~43):
```go
if err != nil {
    h.Logger.Debug("skipping path", "path", path, "error", err)
    return nil
}
```

**SearchContent** (lines ~337, ~359):
```go
// Walk error
if err != nil {
    h.Logger.Debug("skipping path", "path", path, "error", err)
    return nil
}
// ReadFile error
data, err := os.ReadFile(path)
if err != nil {
    h.Logger.Debug("skipping unreadable file", "path", path, "error", err)
    return nil
}
```

**GetBacklinks** (lines ~422, ~450):
```go
// Walk error
if err != nil {
    h.Logger.Debug("skipping path", "path", path, "error", err)
    return nil
}
// ReadFile error
data, err := os.ReadFile(path)
if err != nil {
    h.Logger.Debug("skipping unreadable file", "path", path, "error", err)
    return nil
}
```

**ListTags** (lines ~505, ~527):
```go
// Walk error
if err != nil {
    h.Logger.Debug("skipping path", "path", path, "error", err)
    return nil
}
// ReadFile error
data, err := os.ReadFile(path)
if err != nil {
    h.Logger.Debug("skipping unreadable file", "path", path, "error", err)
    return nil
}
```

**Log level choice:** `Debug` is appropriate here because these are expected conditions (permission errors, disappeared files) during normal vault traversal. They should not clutter output by default but must be available for troubleshooting.

### Step 3: Rewrite `main.go` logging ✅

Replace all `log` and `fmt.Fprintf` usage with `slog`:

```go
import (
    "context"
    "log/slog"
    "os"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/nsega/mcp-obsidian/internal/handler"
    "github.com/nsega/mcp-obsidian/internal/server"
    "github.com/nsega/mcp-obsidian/internal/vault"
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
```

**Key changes:**
- `log.Fatal(...)` → `logger.Error(...)` + `os.Exit(1)` (explicit, no hidden `os.Exit`)
- `fmt.Fprintf(os.Stderr, ...)` → `logger.Info(...)` (structured, timestamped, leveled)
- Remove `"log"` and `"fmt"` imports, add `"log/slog"`
- `TextHandler` to stderr (JSON would break human readability for a CLI tool; stderr is correct since stdout is reserved for MCP protocol)

### Step 4: Update tests ✅

Update `handler.New()` calls in tests to pass a discard logger:

```go
// In handler_test.go
h := handler.New(v, slog.New(slog.NewTextHandler(io.Discard, nil)))
```

This keeps tests silent while ensuring the logger interface is satisfied.

### Step 5: Verify ✅

1. `go build ./...` — compilation
2. `go test -v -race ./...` — all tests pass
3. `go vet ./...` — no issues
4. Manual test: run the server and observe structured log output on stderr

## Dependency Graph (unchanged)

```
main → server → handler → vault
                        → note
```

No new dependencies. `log/slog` is part of the Go standard library (Go 1.21+).

## What This Does NOT Change

- Error propagation via return values in handlers (idiomatic, stays as-is)
- Error wrapping with `%w` (stays as-is)
- MCP protocol error responses to clients (stays as-is)
- `vault.go` — no logging needed here; errors are propagated via returns
- `note/` package — pure types and utilities, no logging needed
