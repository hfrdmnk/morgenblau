# Laws

These are repo-wide invariants for changes to Morgenblau. [SPEC.md](SPEC.md) owns the product rules. The files linked below own the implementation detail. The full Go race suite in [CI](.github/workflows/ci.yml) checks these laws on every change, including code outside a changed-file diff. Frontend tests, lint, and build run there too. Fallow's PR audit separately gates newly introduced findings.

## 1. The reader's PDS owns public reading records

Feed subscriptions and URL saves commit to the reader's PDS before their SQLite indexes change. Creating a customized Standardfeed subscription commits its existence record and metadata sidecar in one PDS transaction. A failed SQLite mirror cannot turn an already committed PDS write into a reported PDS failure; reconciliation catches up on the next authenticated app entry.

The write paths live in [`internal/api/`](internal/api/) and [`internal/atprepo/`](internal/atprepo/). [`mirrorOrRepair`](internal/api/mirror.go) owns the failed-mirror handoff. See the ownership rules in [SPEC.md](SPEC.md#data-ownership).

**Check by hand:** Trace a subscription or URL save from validation through its PDS call to `mirrorOrRepair`. For a customized Standardfeed subscription, verify that existence and sidecar are passed to one `applyWrites` request.

## 2. A completed sync means the local reading index caught up

`sync_user` reaches `done` only after subscriptions and saves have both reconciled and committed locally. A PDS listing taken before a local mirror write cannot erase that newer write. Sidecar cleanup can retry; individual feed fetch failures do not invalidate a successful record reconciliation.

The sync engine and reconciliation guards live in [`internal/sync/`](internal/sync/); job status lives in [`internal/jobs/`](internal/jobs/). See [SPEC.md](SPEC.md#data-ownership) for the authority boundary.

**Check by hand:** Follow both reconciliation results into the job's `done` transition. Inspect the stale-listing guard before any local deletion, and confirm a failed local transaction cannot return success.

## 3. Newsletter data stays private to its reader

Newsletter addresses, messages, and saves are server-owned and scoped by DID in both service operations and relational constraints. They never enter the PDS or shared feed cache. A newsletter save stays private even when the message has a public web URL. Remote newsletter images remain blocked until the reader enables them for that message.

The schema and owner-scoped queries live in [`internal/database/`](internal/database/); private behavior lives in [`internal/newsletter/`](internal/newsletter/) and the newsletter API. See [SPEC.md](SPEC.md#private-newsletter-data).

**Check by hand:** Inspect the newsletter foreign keys and DID filters, then follow a newsletter save and a public URL save from the reader to their separate storage paths. Open an unapproved newsletter message and confirm remote images are blocked by default.

An intentional change to a law updates [SPEC.md](SPEC.md), this file, and its behavioral tests together. There is no changed-file exception for the law checks.
