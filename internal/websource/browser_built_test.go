package websource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/webfeed"
)

func TestWebfeedGeneratorSetupReportsABrowserBuiltListing(t *testing.T) {
	web := newFakeWeb()
	web.set("https://app.example.com/", `<!doctype html><html lang="en"><head><title>Example Publication</title></head>`+
		`<body><div id="root"></div><script type="module" src="/assets/index-4f2a.js"></script></body></html>`)
	_, err := NewWebfeedGenerator(webfeed.Options{Client: web.client()}).Setup(context.Background(), "https://app.example.com/", "", false)
	if !errors.Is(err, ErrBrowserBuilt) {
		t.Fatalf("err = %v, want ErrBrowserBuilt", err)
	}
}

func TestCreateStoresNothingForABrowserBuiltSite(t *testing.T) {
	env := newTestEnv(t)
	env.gen.setupErr = ErrBrowserBuilt

	_, err := env.service.Create(context.Background(), testDID, homeURL, "")
	if !errors.Is(err, ErrBrowserBuilt) {
		t.Fatalf("err = %v, want ErrBrowserBuilt", err)
	}
	if n := sourceCount(t, env); n != 0 {
		t.Fatalf("%d sources stored, want none", n)
	}
}

func TestPollDueMarksASourceWhoseCheckFindsABrowserBuiltPage(t *testing.T) {
	env := newTestEnv(t)
	id := uncheckedSource(t, env, homeURL)
	env.gen.setupErr = fmt.Errorf("%w: %w", ErrGeneration, ErrBrowserBuilt)
	env.advance(31 * time.Minute)
	pollDue(t, env)

	if row := readKeyed(t, env, id); !strings.Contains(row.recipe, `"section_checked":true`) || row.setupSeq != 0 {
		t.Fatalf("row = %+v, want the source kept and marked checked", row)
	}
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if got := setupsOf(env); len(got) != 2 {
		t.Errorf("setups = %v, want no second check", got)
	}
}
