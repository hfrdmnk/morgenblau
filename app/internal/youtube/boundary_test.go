package youtube

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Every other package that held a transcript could hand it to the PDS, a feed or an export.
var allowedImporters = []string{
	"morgenblau/internal/api",
	"morgenblau/internal/server",
}

func TestOnlyAPIAndServerImportYouTube(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("go toolchain is required on PATH for import boundary tests")
	}
	command := exec.Command(goPath, "list", "-f", "{{.ImportPath}}{{range .Imports}} {{.}}{{end}}", "./cmd/...", "./internal/...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !slices.Contains(fields[1:], "morgenblau/internal/youtube") {
			continue
		}
		if !slices.Contains(allowedImporters, fields[0]) {
			t.Errorf("%s imports internal/youtube; only api and server may hold transcripts, so they never reach the PDS, a feed or an export (law 4)", fields[0])
		}
	}
}
