package discoverperson

import (
	"reflect"
	"strings"
	"testing"

	"morgenblau/internal/discovercrawl"
)

// The cached crawlers must satisfy this package's fetcher seams directly, with no adapters.
var (
	_ SubscriptionFetcher = (*discovercrawl.CachedCrawler)(nil)
	_ AuthoredFetcher     = (*discovercrawl.CachedAuthoredCrawler)(nil)
	_ ShareFetcher        = (*discovercrawl.CachedShareCrawler)(nil)
)

// Save privacy is structural: no public type here may name a save, in a field or a tag.
func TestNoSaveInPublicTypes(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(SourceItem{}),
		reflect.TypeOf(ShareItem{}),
		reflect.TypeOf(Records{}),
		reflect.TypeOf(Preview{}),
	}
	for _, ty := range types {
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			if strings.Contains(strings.ToLower(f.Name), "save") {
				t.Errorf("%s.%s mentions save", ty.Name(), f.Name)
			}
			if strings.Contains(strings.ToLower(string(f.Tag)), "save") {
				t.Errorf("%s.%s tag mentions save", ty.Name(), f.Name)
			}
		}
	}
}
