package sync

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

const failSavesOnlyAtCommitSQL = `
CREATE TABLE commit_tripwire_parent (id TEXT PRIMARY KEY);
CREATE TABLE commit_tripwire (ref TEXT REFERENCES commit_tripwire_parent(id) DEFERRABLE INITIALLY DEFERRED);
CREATE TRIGGER user_saves_commit_tripwire AFTER INSERT ON user_saves BEGIN
    INSERT INTO commit_tripwire (ref) VALUES ('missing');
END;`

func TestSyncUser_FailsWhenTheSavesTransactionDoesNotCommit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tripwire  bool
		want      jobs.Status
		wantSaves int
	}{
		{name: "commit fails", tripwire: true, want: jobs.StatusFailed, wantSaves: 0},
		{name: "commit succeeds", want: jobs.StatusDone, wantSaves: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbs := openMigratedSyncDB(t)
			ctx := context.Background()
			if tc.tripwire {
				if _, err := dbs.Writer.ExecContext(ctx, failSavesOnlyAtCommitSQL); err != nil {
					t.Fatalf("tripwire: %v", err)
				}
			}
			lister := &fakeLister{
				subs:  []PDSSubscription{{URI: "at://did:plc:alice/blue.morgen.feed.subscription/3sub", Kind: "rss", Rkey: "3sub", FeedURL: "https://example.com/feed"}},
				saves: []PDSSave{{URI: "at://did:plc:alice/blue.morgen.feed.save/3save", Rkey: "3save", ItemURL: "https://example.com/post", CreatedAt: "2026-07-20T12:00:00Z"}},
			}
			tracker := jobs.New()
			eng := NewEngine(tracker, db.New(dbs.Writer), lister, &countingFetcher{}, &nopResumer{}, nil).WithTxRunner(dbs.Writer)
			did := mustDID("did:plc:alice")

			id, err := eng.SyncUser(ctx, did, "sid-1", jobs.TriggerManual)
			if err != nil {
				t.Fatal(err)
			}
			if job := waitForTerminalJob(t, tracker, id, did); job.Status != tc.want {
				t.Fatalf("status = %q, want %q", job.Status, tc.want)
			}
			saves, err := db.New(dbs.Reader).ListUserSavesForSync(ctx, did.String())
			if err != nil {
				t.Fatal(err)
			}
			if len(saves) != tc.wantSaves {
				t.Errorf("committed saves = %d, want %d", len(saves), tc.wantSaves)
			}
		})
	}
}

func TestSyncUser_ConvergesASaveRecreatedUnderANewRkey(t *testing.T) {
	dbs := openMigratedSyncDB(t)
	ctx := context.Background()
	did := mustDID("did:plc:alice")
	if err := db.New(dbs.Writer).UpsertUserSave(ctx, db.UpsertUserSaveParams{
		Did: did.String(), Rkey: "3old", AtUri: "at://did:plc:alice/blue.morgen.feed.save/3old",
		ItemUrl: "https://example.com/post", CreatedAt: "2026-07-20T12:00:00Z", UpdatedAt: "2026-07-20T12:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	lister := &fakeLister{saves: []PDSSave{{URI: "at://did:plc:alice/blue.morgen.feed.save/3new", Rkey: "3new", ItemURL: "https://example.com/post", CreatedAt: "2026-07-21T12:00:00Z"}}}
	tracker := jobs.New()
	eng := NewEngine(tracker, db.New(dbs.Writer), lister, &countingFetcher{}, &nopResumer{}, nil).WithTxRunner(dbs.Writer)

	id, err := eng.SyncUser(ctx, did, "sid-1", jobs.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if job := waitForTerminalJob(t, tracker, id, did); job.Status != jobs.StatusDone {
		t.Fatalf("status = %q, want done: the stale row must leave before the new rkey upserts, or UNIQUE(did, item_url) rejects the saves pass", job.Status)
	}
	saves, err := db.New(dbs.Reader).ListUserSavesForSync(ctx, did.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(saves) != 1 || saves[0].Rkey != "3new" {
		t.Errorf("saves = %+v, want one row under 3new", saves)
	}
}

func openMigratedSyncDB(t *testing.T) *database.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	goosePath, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("goose CLI is required for sync storage tests; install github.com/pressly/goose/v3/cmd/goose at the GOOSE_VERSION .github/workflows/ci.yml pins")
	}
	migrationDir, err := filepath.Abs(filepath.Join("..", "database", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(goosePath, "-dir", migrationDir, "sqlite3", path, "up").CombinedOutput(); err != nil {
		t.Fatalf("goose migration: %v\n%s", err, output)
	}
	t.Setenv("DB_PATH", path)
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	return dbs
}
