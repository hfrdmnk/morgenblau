package lexicon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// lexicons/ is the schema authority; the embedded copies validate PDS writes, so a drifted copy validates against a schema nobody published.
func TestEmbeddedSchemasMatchTheLexiconsDirectory(t *testing.T) {
	entries, err := schemaFS.ReadDir("schemas")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded schemas found")
	}
	for _, entry := range entries {
		embeddedJSON, err := schemaFS.ReadFile("schemas/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		nsid := strings.TrimSuffix(entry.Name(), ".json")
		source := filepath.Join("..", "..", "lexicons", filepath.Join(strings.Split(nsid, ".")...)+".json")
		sourceJSON, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		embedded, authority := decodeSchema(t, embeddedJSON), decodeSchema(t, sourceJSON)
		// The embedded copy is the published com.atproto.lexicon.schema record, which adds only its $type.
		delete(embedded, "$type")
		if !reflect.DeepEqual(embedded, authority) {
			t.Errorf("schemas/%s differs from %s; lexicons/ is the schema authority, so the runtime validator must embed exactly what it defines", entry.Name(), source)
		}
	}
}

func decodeSchema(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
