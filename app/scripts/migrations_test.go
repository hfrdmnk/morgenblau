package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingMigrationsAreUnchanged(t *testing.T) {
	script, err := filepath.Abs("check-migrations.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"none", "added", "edited", "deleted", "renamed", "missing base"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, output)
				}
				return strings.TrimSpace(string(output))
			}
			write := func(path, body string) {
				t.Helper()
				file := filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			git("init", "-q")
			git("config", "user.name", "Example Contributor")
			git("config", "user.email", "contributor@example.com")
			existing := "internal/database/migrations/20260101000000_example.sql"
			write(existing, "-- +goose Up\n")
			git("add", ".")
			git("commit", "-qm", "baseline")
			base := git("rev-parse", "HEAD")
			switch change {
			case "added":
				write("internal/database/migrations/20260102000000_next.sql", "-- +goose Up\n")
			case "edited":
				write(existing, "-- +goose Up\n-- +goose Down\n")
			case "deleted":
				git("rm", "-q", existing)
			case "renamed":
				git("mv", existing, "internal/database/migrations/20260103000000_example.sql")
			case "missing base":
				base = strings.Repeat("0", 40)
			}
			git("add", ".")
			git("commit", "--allow-empty", "-qm", "change")
			cmd := exec.Command("bash", script, base)
			cmd.Dir = dir
			output, err := cmd.CombinedOutput()
			wantError := change != "none" && change != "added"
			if (err != nil) != wantError {
				t.Fatalf("check error = %v, want error %v\n%s", err, wantError, output)
			}
			message := "Add a new migration instead"
			if change == "missing base" {
				message = "base commit is unavailable"
			}
			if wantError && !strings.Contains(string(output), message) {
				t.Fatalf("refusal must explain why: %s", output)
			}
		})
	}
}
