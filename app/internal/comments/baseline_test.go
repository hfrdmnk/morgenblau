package comments

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const appRoot = "../.."

func TestCommentsStayWithinBaseline(t *testing.T) {
	reports, err := Scan(appRoot)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := ReadBaseline(filepath.Join(appRoot, BaselineFile))
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	for _, file := range slices.Sorted(maps.Keys(reports)) {
		counted := reports[file].Counted
		if len(counted) <= baseline[file] {
			continue
		}
		var listing strings.Builder
		for _, pos := range counted {
			fmt.Fprintf(&listing, "\n\t%s:%d", pos.Filename, pos.Line)
		}
		problems = append(problems, fmt.Sprintf("%s has %d comments, %s allows %d:%s", file, len(counted), BaselineFile, baseline[file], listing.String()))
	}
	for _, file := range slices.Sorted(maps.Keys(baseline)) {
		if n := len(reports[file].Counted); n < baseline[file] {
			problems = append(problems, fmt.Sprintf("%s has %d comments, %s still allows %d: run `make comments-prune` so a new comment can't take a deleted one's place", file, n, BaselineFile, baseline[file]))
		}
	}
	if len(problems) > 0 {
		t.Fatalf("%s\n\n%s", Policy, strings.Join(problems, "\n\n"))
	}
}

func TestNoNolint(t *testing.T) {
	reports, err := Scan(appRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range slices.Sorted(maps.Keys(reports)) {
		for _, pos := range reports[file].Nolint {
			t.Errorf("%s:%d: %s", pos.Filename, pos.Line, NolintPolicy)
		}
	}
}
