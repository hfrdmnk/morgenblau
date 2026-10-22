package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLexiconFreeze(t *testing.T) {
	script, err := filepath.Abs("check-lexicons.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"none", "prose", "owned", "interop", "added", "deleted", "renamed", "missing base"} {
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
			owned := "lexicons/blue/morgen/feed/save.json"
			interop := "lexicons/site/standard/document.json"
			write(owned, `{"revision":1}`)
			write(interop, `{"revision":1}`)
			git("add", ".")
			git("commit", "-qm", "baseline")
			base := git("rev-parse", "HEAD")
			switch change {
			case "prose":
				write("lexicons/AGENTS.md", "Example guidance")
			case "owned":
				write(owned, `{"revision":2}`)
			case "interop":
				write(interop, `{"revision":2}`)
			case "added":
				write("lexicons/blue/morgen/new.json", `{}`)
			case "deleted":
				git("rm", owned)
			case "renamed":
				git("mv", owned, "lexicons/blue/morgen/feed/renamed.json")
			case "missing base":
				base = strings.Repeat("0", 40)
			}
			git("add", ".")
			git("commit", "--allow-empty", "-qm", "change")
			cmd := exec.Command("bash", script, base)
			cmd.Dir = dir
			output, err := cmd.CombinedOutput()
			wantError := change != "none" && change != "prose"
			if (err != nil) != wantError {
				t.Fatalf("check error = %v, want error %v\n%s", err, wantError, output)
			}
			if wantError {
				message := "SPEC.md freezes external compatibility"
				if change == "missing base" {
					message = "base commit is unavailable"
				}
				if !strings.Contains(string(output), message) {
					t.Fatalf("refusal must explain the lexicon boundary: %s", output)
				}
			}
		})
	}
}
