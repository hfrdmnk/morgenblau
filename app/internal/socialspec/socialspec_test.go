package socialspec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func serve(t *testing.T, files map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

const vcard = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Jordan\r\n  Doe\r\nNICKNAME:@jordan.example.com\r\nitem1.EMAIL;TYPE=INTERNET,PREF:hello@jordan.example.com\r\nEMAIL:other@jordan.example.com\r\nURL:https://jordan.example.com\r\nURL:javascript:alert(1)\r\nEND:VCARD\r\n"

const opml = `<?xml version="1.0"?>
<opml version="2.0"><head><title>Follows</title></head><body>
  <outline text="Avery" htmlUrl="https://avery.example.com" xmlUrl="https://avery.example.com/feed.xml"/>
  <outline text="Folder">
    <outline title="Example Publication" xmlUrl="https://publication.example.com/rss"/>
    <outline text="Avery again" xmlUrl="https://avery.example.com/feed.xml"/>
  </outline>
  <outline text="No feed" htmlUrl="https://nofeed.example.com"/>
</body></opml>`

const about = "---\nsummary: \"Eager explorer: takes the long way home.\"\nlayout: profile\n---\n\n### About Jordan\n\nI take the **long way** home, see [my garden](/garden).\n\n![Out on the trail](/img/trail.png)\n\n* [Notes](notes.html)\n* `Tea`\n\n[Mail](mailto:hi@jordan.example.com) or [chat](https://chat.example.com/jordan).\n"

func TestFetch_ReadsAllThreeFiles(t *testing.T) {
	origin := serve(t, map[string]string{
		"/.well-known/contact.vcf":    vcard,
		"/.well-known/following.opml": opml,
		"/.well-known/about.md":       about,
	})
	got, err := Fetch(context.Background(), http.DefaultClient, origin)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("profile = nil")
	}
	if got.Name != "Jordan Doe" {
		t.Errorf("name = %q, want the unfolded FN", got.Name)
	}
	wantContact := []Contact{
		{Kind: "email", Value: "hello@jordan.example.com", Preferred: true},
		{Kind: "email", Value: "other@jordan.example.com"},
		{Kind: "url", Value: "https://jordan.example.com"},
	}
	if !reflect.DeepEqual(got.Contact, wantContact) {
		t.Errorf("contact = %+v, want %+v", got.Contact, wantContact)
	}
	wantFollowing := []Follow{
		{Title: "Avery", FeedURL: "https://avery.example.com/feed.xml", SiteURL: "https://avery.example.com"},
		{Title: "Example Publication", FeedURL: "https://publication.example.com/rss"},
	}
	if !reflect.DeepEqual(got.Following, wantFollowing) {
		t.Errorf("following = %+v, want %+v", got.Following, wantFollowing)
	}
	if got.Summary != "Eager explorer: takes the long way home." {
		t.Errorf("summary = %q, want the frontmatter summary", got.Summary)
	}
	for _, want := range []string{
		"<h3>About Jordan</h3>",
		"<strong>long way</strong>",
		`href="` + origin + `/garden"`,
		`src="` + origin + `/img/trail.png" alt="Out on the trail"`,
		`href="` + origin + `/.well-known/notes.html"`,
		"<code>Tea</code>",
		`href="mailto:hi@jordan.example.com"`,
		`href="https://chat.example.com/jordan"`,
	} {
		if !strings.Contains(got.AboutHTML, want) {
			t.Errorf("about html lacks %s:\n%s", want, got.AboutHTML)
		}
	}
	if strings.Contains(got.AboutHTML, "summary") || strings.Contains(got.AboutHTML, "layout") {
		t.Errorf("about html shows the frontmatter:\n%s", got.AboutHTML)
	}
}

// socialspec.org's own about.md has no frontmatter and is served as text/plain.
func TestFetch_AboutWithoutFrontmatter(t *testing.T) {
	md := "# About Example Publication\n\nA _small_ site.\n\n---\n\nsummary: not frontmatter\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/about.md" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(md))
	}))
	t.Cleanup(srv.Close)
	got, err := Fetch(context.Background(), http.DefaultClient, srv.URL)
	if err != nil || got == nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got.Summary != "" {
		t.Errorf("summary = %q, want none", got.Summary)
	}
	want := "<p>A <em>small</em> site.</p>\n<hr>\n<p>summary: not frontmatter</p>\n"
	if got.AboutHTML != want {
		t.Errorf("about html = %q, want %q", got.AboutHTML, want)
	}
}

func TestFetch_AboutIsSanitized(t *testing.T) {
	md := "Hi.\n\n<script>alert(1)</script>\n\n[click](javascript:alert(1)) <img src=x onerror=alert(1)>\n\n![pixel](data:image/png;base64,AAAA)\n"
	got, err := Fetch(context.Background(), http.DefaultClient, serve(t, map[string]string{"/.well-known/about.md": md}))
	if err != nil || got == nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, banned := range []string{"<script", "alert", "javascript:", "onerror", "data:"} {
		if strings.Contains(got.AboutHTML, banned) {
			t.Errorf("about html keeps %s:\n%s", banned, got.AboutHTML)
		}
	}
}

// A file that is only frontmatter still says something about its author.
func TestFetch_FrontmatterOnlyAbout(t *testing.T) {
	md := "\ufeff---\r\nsummary: |\r\n  Walks a lot.\r\n---\r\n"
	got, err := Fetch(context.Background(), http.DefaultClient, serve(t, map[string]string{"/.well-known/about.md": md}))
	if err != nil || got == nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got.Summary != "Walks a lot." || got.AboutHTML != "" {
		t.Errorf("summary = %q, about html = %q", got.Summary, got.AboutHTML)
	}
}

func TestFetch_NothingPublishedIsNil(t *testing.T) {
	got, err := Fetch(context.Background(), http.DefaultClient, serve(t, nil))
	if err != nil || got != nil {
		t.Fatalf("got %+v, %v; want nil, nil", got, err)
	}
}

// Single-page apps answer every path with their HTML shell, which must not read as a profile.
func TestFetch_IgnoresHTMLFallbacks(t *testing.T) {
	page := "<!doctype html><html><body>App</body></html>"
	origin := serve(t, map[string]string{
		"/.well-known/contact.vcf":    page,
		"/.well-known/following.opml": page,
		"/.well-known/about.md":       page,
	})
	got, err := Fetch(context.Background(), http.DefaultClient, origin)
	if err != nil || got != nil {
		t.Fatalf("got %+v, %v; want nil, nil", got, err)
	}
}

func TestFetch_FollowingOnlyStillMakesAProfile(t *testing.T) {
	origin := serve(t, map[string]string{"/.well-known/following.opml": opml})
	got, err := Fetch(context.Background(), http.DefaultClient, origin)
	if err != nil || got == nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got.Name != "" || len(got.Following) != 2 {
		t.Errorf("profile = %+v", got)
	}
}
