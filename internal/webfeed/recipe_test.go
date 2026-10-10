package webfeed

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const specRecipeJSON = `{
 "version": 1,
 "listing_url": "https://news.example.com/",
 "mode": "streams",
 "feed_url": "",
 "streams": [{
  "url_template": "news.example.com/posts/*",
  "container_hints": ["div.grid<section<main"],
  "title_hints": {}
 }],
 "css": null,
 "exclude_urls": [],
 "input": "raw",
 "lang": "en",
 "order": "date",
 "date_source": "listing",
 "fingerprint": {"count": 12, "url_templates": ["news.example.com/posts/*"], "date_frac": 1, "title_shape": 0.95},
 "setup": {"stages": ["heuristic", "judge"], "confidence": 0.85, "margin": 1.37, "judge": {"pick": [0, 2], "order": "date"}}
}`

func TestRecipeDecodesTheSpecShape(t *testing.T) {
	var r Recipe
	if err := json.Unmarshal([]byte(specRecipeJSON), &r); err != nil {
		t.Fatal(err)
	}
	if r.Version != 1 || r.Mode != ModeStreams || r.Order != OrderDate || r.DateSource != DateSourceListing || r.Lang != "en" {
		t.Fatalf("scalars = %+v", r)
	}
	if len(r.Streams) != 1 || r.Streams[0].URLTemplate != "news.example.com/posts/*" || r.Streams[0].ContainerHints[0] != "div.grid<section<main" {
		t.Fatalf("streams = %+v", r.Streams)
	}
	if r.CSS != nil {
		t.Fatalf("css = %+v, want nil", r.CSS)
	}
	if r.Fingerprint.Count != 12 || r.Fingerprint.DateFrac != 1 || r.Fingerprint.TitleShape != 0.95 {
		t.Fatalf("fingerprint = %+v", r.Fingerprint)
	}
	if r.Setup.Judge == nil || !reflect.DeepEqual(r.Setup.Judge.Pick, []int{0, 2}) || r.Setup.Margin != 1.37 {
		t.Fatalf("setup = %+v", r.Setup)
	}
}

func TestRecipeRoundTrips(t *testing.T) {
	in := Recipe{
		Version:    1,
		ListingURL: "https://news.example.com/blog",
		Mode:       ModeStreams,
		Streams: []Stream{{
			URLTemplate:    "news.example.com/blog/*",
			ContainerHints: []string{"ul.posts<main"},
			TitleHints:     TitleHints{Source: "heading", Tag: "h2"},
		}},
		ExcludeURLs: []string{"https://news.example.com/blog/archive"},
		Lang:        "de",
		Order:       OrderPage,
		DateSource:  DateSourceArticle,
		Fingerprint: Fingerprint{Count: 3, URLTemplates: []string{"news.example.com/blog/*"}, DateFrac: 0.5, TitleShape: 1},
		Setup: SetupRecord{
			Stages:     []string{"heuristic", "verify", "judge"},
			Confidence: 0.4,
			Margin:     0.5,
			Judge:      &JudgeRecord{Pick: []int{1}, Order: OrderPage, Calls: 3, InputTokens: 7500, OutputTokens: 1800, USD: 0.0016},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Recipe
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip changed the recipe:\n in=%+v\nout=%+v", in, out)
	}
}

func TestNewRecipeMarshalsEmptyListsAndNullCSS(t *testing.T) {
	raw, err := json.Marshal(newRecipe("https://news.example.com/", ModeSitemap))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"version":1`, `"mode":"sitemap"`, `"streams":[]`, `"exclude_urls":[]`, `"css":null`} {
		if !strings.Contains(s, want) {
			t.Errorf("recipe JSON %s missing %s", s, want)
		}
	}
}

func TestEmptyTitleHintsMarshalAsObject(t *testing.T) {
	raw, err := json.Marshal(Stream{URLTemplate: "news.example.com/*"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"title_hints":{}`) || !strings.Contains(string(raw), `"container_hints":[]`) {
		t.Fatalf("stream JSON = %s", raw)
	}
}
