package newsletter

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Private newsletter storage may only depend on the local database; PDS, sync, and feed-cache code stay unreachable.
var allowedNewsletterInternalDeps = []string{
	"morgenblau/internal/database",
	"morgenblau/internal/database/db",
	"morgenblau/internal/newsletter",
}

// Every other package that held newsletter values could hand them to the PDS or the shared feed cache.
var allowedNewsletterImporters = []string{
	"morgenblau/internal/api",
	"morgenblau/internal/server",
}

func TestNewsletterPackageCannotReachPDSOrFeedCache(t *testing.T) {
	for _, dep := range goList(t, ".", "-deps", "-f", "{{.ImportPath}}") {
		if strings.HasPrefix(dep, "morgenblau/") && !slices.Contains(allowedNewsletterInternalDeps, dep) {
			t.Errorf("internal/newsletter depends on %s; newsletter storage may reach only the database, never the PDS, sync or feed cache (law 3)", dep)
		}
		if strings.HasPrefix(dep, "github.com/bluesky-social/indigo") {
			t.Errorf("internal/newsletter depends on atproto package %s; newsletter data never enters the PDS (law 3)", dep)
		}
	}
}

func TestOnlyAPIAndServerImportNewsletter(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range goList(t, root, "-f", "{{.ImportPath}}{{range .Imports}} {{.}}{{end}}", "./cmd/...", "./internal/...") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !slices.Contains(fields[1:], "morgenblau/internal/newsletter") {
			continue
		}
		if !slices.Contains(allowedNewsletterImporters, fields[0]) {
			t.Errorf("%s imports internal/newsletter; only api and server may hold newsletter values, so nothing else can hand them to the PDS or the shared cache (law 3)", fields[0])
		}
	}
}

func goList(t *testing.T, dir string, args ...string) []string {
	t.Helper()
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("go toolchain is required on PATH for import boundary tests")
	}
	command := exec.Command(goPath, append([]string{"list"}, args...)...)
	command.Dir = dir
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list %s: %v", strings.Join(args, " "), err)
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n")
}
