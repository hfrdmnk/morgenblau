package main

import (
	"encoding/json"
	"fmt"
	"os"

	"morgenblau/internal/comments"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "comments-prune:", err)
		os.Exit(1)
	}
}

func run() error {
	reports, err := comments.Scan(".")
	if err != nil {
		return err
	}
	baseline, err := comments.ReadBaseline(comments.BaselineFile)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for file, report := range reports {
		counts[file] = len(report.Counted)
	}
	data, err := json.MarshalIndent(comments.Prune(baseline, counts), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(comments.BaselineFile, append(data, '\n'), 0o644)
}
