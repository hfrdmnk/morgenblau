# Deepening

How to deepen a cluster of shallow modules safely, given its dependencies. Assumes the vocabulary in [SKILL.md](SKILL.md): **module**, **interface**, **seam**, **adapter**.

## Dependency categories

When assessing a candidate for deepening, classify its dependencies. The category determines how the deepened module is tested across its seam.

### 1. In-process

Pure computation, in-memory state, no I/O (ranking, key derivation, parsing). Always deepenable: merge the modules and test through the new interface directly. No adapter needed.

### 2. Local-substitutable

Dependencies that have local test stand-ins. In this repo that is SQLite: tests point `DB_PATH` at `filepath.Join(t.TempDir(), "test.db")`, call `Open()`, and exec a schema constant scoped to what the test touches, so the deepened module runs against a real database that the temp dir cleans up. Use a file, not `:memory:`, since separate pools must share one database. Deepenable if the stand-in exists. The seam is internal; no port at the module's external interface.

A deepening that widens a module's schema footprint widens those per-test schema constants too. Say so in the recommendation: it is the main cost of merging across `internal/database`.

### 3. Remote but owned (Ports & Adapters)

Services you control across a network boundary, plus protocol services you write against a spec: the tap sidecar, a user's PDS over XRPC. Define a **port** (interface) at the seam, the way `atprepo.Writer` and `tapingest.TapStore` do. The deep module owns the logic; the transport is injected as an **adapter**. Tests use an in-memory adapter. Production uses the XRPC or HTTP adapter.

Recommendation shape: *"Define a port at the seam, implement an XRPC adapter for production and an in-memory adapter for testing, so the logic sits in one deep module even though it's deployed across a network."*

### 4. True external (Mock)

Services you don't control: publisher RSS/Atom endpoints, the relay firehose, favicon and page fetches. The deepened module takes the dependency as an injected port (see `feedfinder.HTTPDoer`); tests provide a mock adapter, or stand up an `httptest.NewServer` when the wire format is what's under test.

## Seam discipline

- **One adapter means a hypothetical seam. Two adapters means a real one.** Don't introduce a port unless at least two adapters are justified (typically production + test). A single-adapter seam is just indirection.
- **Internal seams vs external seams.** A deep module can have internal seams (private to its implementation, used by its own tests) as well as the external seam at its interface. Don't expose internal seams through the interface just because tests use them.

## Testing strategy: replace, don't layer

- Old unit tests on shallow modules become waste once tests at the deepened module's interface exist; delete them.
- Write new tests at the deepened module's interface. The **interface is the test surface**.
- Tests assert on observable outcomes through the interface, not internal state.
- Tests should survive internal refactors, since they describe behaviour, not implementation. If a test has to change when the implementation changes, it's testing past the interface.
