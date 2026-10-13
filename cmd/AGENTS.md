# cmd

`cmd/api` starts the application; `internal/server.NewServer()` wires its dependencies. Developer tools may compose their own clients and own tool-specific orchestration here; reusable application behavior belongs in `internal/`. Review those boundaries under the Go conventions in [internal/AGENTS.md](../internal/AGENTS.md).
