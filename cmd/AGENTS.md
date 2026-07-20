# Go conventions

- Keep `cmd/api` limited to process startup. `internal/server.NewServer()` is the sole composition root: every dependency is wired there by hand. No globals, no `init()` wiring, no service containers.
- Use `log/slog`; code must be gofmt-clean and `go vet`-clean.
- Generic fixtures only. Test data uses reserved namespaces and invented names, never real people, handles, domains, or publications.
