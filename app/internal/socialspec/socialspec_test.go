package socialspec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
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

const about = "# About Jordan\n\nI take the **long way** home, see [my garden](https://jordan.example.com/garden).\nStill walking.\n\n* Trails\n* `Tea`\n"

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
	wantAbout := []string{"I take the long way home, see my garden. Still walking.", "Trails", "Tea"}
	if !reflect.DeepEqual(got.About, wantAbout) {
		t.Errorf("about = %q, want %q", got.About, wantAbout)
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
