package lexicon

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bluesky-social/indigo/atproto/atdata"
	"github.com/bluesky-social/indigo/atproto/lexicon"
)

//go:embed schemas/*.json
var schemaFS embed.FS

var (
	catalogOnce sync.Once
	catalog     *lexicon.BaseCatalog
	catalogErr  error
)

func loadCatalog() (*lexicon.BaseCatalog, error) {
	catalogOnce.Do(func() {
		cat := lexicon.NewBaseCatalog()
		if err := cat.LoadEmbedFS(schemaFS); err != nil {
			catalogErr = fmt.Errorf("lexicon: load embedded schemas: %w", err)
			return
		}
		catalog = cat
	})
	return catalog, catalogErr
}

func ValidateRecord(nsid string, record map[string]any) error {
	return validateRecord(nsid, record, 0)
}

func ValidateRecordLenient(nsid string, record map[string]any) error {
	return validateRecord(nsid, record, lexicon.LenientMode)
}

func validateRecord(nsid string, record map[string]any, flags lexicon.ValidateFlags) error {
	cat, err := loadCatalog()
	if err != nil {
		return err
	}
	typed := make(map[string]any, len(record)+1)
	for k, v := range record {
		typed[k] = v
	}
	typed["$type"] = nsid
	raw, err := json.Marshal(typed)
	if err != nil {
		return fmt.Errorf("lexicon: %s: marshal record: %w", nsid, err)
	}
	data, err := atdata.UnmarshalJSON(raw)
	if err != nil {
		return fmt.Errorf("lexicon: %s: unmarshal record: %w", nsid, err)
	}
	if err := lexicon.ValidateRecord(cat, data, nsid, flags); err != nil {
		return fmt.Errorf("lexicon: %s: %w", nsid, err)
	}
	return nil
}
