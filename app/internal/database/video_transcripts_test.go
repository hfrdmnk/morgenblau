package database

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	videoTranscriptTable     = "video_transcripts"
	videoTranscriptQueryFile = "video_transcripts.sql"
	// videoTranscriptReader is the one file that may hold transcript rows: the reader's transcript endpoint.
	videoTranscriptReader = "internal/api/entry_transcript.go"
)

func TestVideoTranscriptsStayInTheirOwnQueries(t *testing.T) {
	seen := false
	for _, query := range loadSQLQueries(t) {
		for _, ref := range sqlTableRefs(query.sql) {
			isTranscript := ref.table == videoTranscriptTable
			seen = seen || isTranscript
			if query.file == videoTranscriptQueryFile && !isTranscript {
				t.Errorf("%s reads or writes %s; transcript queries touch only %s so caption text never joins a digest, feed or export (law 4)", query.name, ref.table, videoTranscriptTable)
			}
			if query.file != videoTranscriptQueryFile && isTranscript {
				t.Errorf("%s in %s touches %s; it belongs to %s only, so transcripts stay in the reader (law 4)", query.name, query.file, videoTranscriptTable, videoTranscriptQueryFile)
			}
		}
	}
	if !seen {
		t.Fatal("no query touches video_transcripts; the check is no longer looking at the transcript queries")
	}
}

func TestVideoTranscriptsReachOnlyTheReaderEndpoint(t *testing.T) {
	methods := transcriptQueryNames(t)
	table := regexp.MustCompile(`\b` + videoTranscriptTable + `\b`)
	eachGoFile(t, false, func(path string, _ *token.FileSet, file *ast.File) {
		allowed := strings.HasSuffix(filepath.ToSlash(path), videoTranscriptReader)
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.BasicLit:
				if n.Kind == token.STRING && table.MatchString(n.Value) {
					t.Errorf("%s embeds SQL for %s; add it to queries/%s instead (law 4)", path, videoTranscriptTable, videoTranscriptQueryFile)
				}
			case *ast.Ident:
				if methods[n.Name] && !allowed {
					t.Errorf("%s uses %s; only %s may read or write transcripts, so caption text never reaches the PDS, a feed or an export (law 4)", path, n.Name, videoTranscriptReader)
				}
			}
			return true
		})
	})
}

func transcriptQueryNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, query := range loadSQLQueries(t) {
		if query.file == videoTranscriptQueryFile {
			names[query.name] = true
		}
	}
	if len(names) == 0 {
		t.Fatalf("queries/%s holds no queries", videoTranscriptQueryFile)
	}
	return names
}
