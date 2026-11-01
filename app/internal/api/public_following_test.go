package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"morgenblau/internal/database/db"
	"morgenblau/internal/tags"
)

const (
	publicReader = "did:plc:reader"
	publicPub    = "at://did:plc:publisher/site.standard.publication/3pub"
)

type fakePublicFollowing struct {
	enabled map[string]bool
	rows    map[string][]db.ListPublicFollowingSourcesRow
}

func (f *fakePublicFollowing) PublicFollowingEnabled(_ context.Context, did string) (bool, error) {
	return f.enabled[did], nil
}

func (f *fakePublicFollowing) ListPublicFollowingSources(_ context.Context, did string) ([]db.ListPublicFollowingSourcesRow, error) {
	return f.rows[did], nil
}

func (f *fakePublicFollowing) SetPublicFollowing(_ context.Context, arg db.SetPublicFollowingParams) error {
	f.enabled[arg.Did] = arg.PublicFollowing == 1
	return nil
}

func publicRow(kind, feedURL, title, siteURL string, primary bool, tagList ...string) db.ListPublicFollowingSourcesRow {
	row := db.ListPublicFollowingSourcesRow{Kind: kind, FeedUrl: feedURL, Tags: tags.Marshal(tagList)}
	if title != "" {
		row.Title = &title
	}
	if siteURL != "" {
		row.SiteUrl = &siteURL
	}
	if primary {
		row.IsPrimary = 1
	}
	return row
}

func publicStore() *fakePublicFollowing {
	catalog := "Example Publication"
	pub := publicRow("standardfeed", publicPub, "", "https://publication.example.com/", true, "Publications")
	pub.CatalogTitle = &catalog
	return &fakePublicFollowing{
		enabled: map[string]bool{publicReader: true},
		rows: map[string][]db.ListPublicFollowingSourcesRow{publicReader: {
			publicRow("rss", "https://blog.example.com/feed.xml", "Example Blog", "https://blog.example.com/", false, "Personal"),
			publicRow("rss", "https://www.youtube.com/feeds/videos.xml?channel_id=UCaaaaaaaaaaaaaaaaaaaaaa", "Example Channel", "https://www.youtube.com/channel/UCaaaaaaaaaaaaaaaaaaaaaa", false, "Personal", "Video"),
			pub,
			publicRow("rss", "https://untagged.example.com/rss", "", "", true),
			// A publication whose site URL is not known yet has nothing to link in OPML.
			publicRow("standardfeed", "at://did:plc:publisher/site.standard.publication/3new", "Unlinked Publication", "", false),
		}},
	}
}

func getPublic(t *testing.T, store PublicFollowingReader, format, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /user/{did}/following."+format, PublicFollowingHandler(store, format))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))
	return rr
}

type publicJSON struct {
	DID     string `json:"did"`
	Sources []struct {
		Title       string   `json:"title"`
		Kind        string   `json:"kind"`
		FeedURL     *string  `json:"feedUrl"`
		Publication *string  `json:"publication"`
		SiteURL     string   `json:"siteUrl"`
		Tags        []string `json:"tags"`
		Primary     bool     `json:"primary"`
	} `json:"sources"`
}

func decodePublicJSON(t *testing.T, rr *httptest.ResponseRecorder) publicJSON {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	var out publicJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func publicTitles(out publicJSON) string {
	var titles []string
	for _, s := range out.Sources {
		titles = append(titles, s.Title)
	}
	return strings.Join(titles, " | ")
}

func TestPublicFollowing_UnknownOptedOutAndInvalidDIDsLookTheSame(t *testing.T) {
	store := publicStore()
	store.rows["did:plc:quiet"] = store.rows[publicReader]
	for _, format := range []string{"json", "opml"} {
		var bodies []string
		for _, did := range []string{"did:plc:quiet", "did:plc:stranger", "not-a-did"} {
			rr := getPublic(t, store, format, "/user/"+did+"/following."+format)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("%s %s = %d, want 404", format, did, rr.Code)
			}
			bodies = append(bodies, rr.Body.String())
		}
		if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
			t.Errorf("%s: 404 bodies differ, so they reveal the setting: %q", format, bodies)
		}
	}
}

func TestPublicFollowing_JSONListsEveryFeedSource(t *testing.T) {
	rr := getPublic(t, publicStore(), "json", "/user/"+publicReader+"/following.json")
	if cc := rr.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "public, max-age=") {
		t.Errorf("Cache-Control = %q", cc)
	}
	out := decodePublicJSON(t, rr)
	if out.DID != publicReader || len(out.Sources) != 5 {
		t.Fatalf("got %+v", out)
	}
	byTitle := map[string]int{}
	for i, s := range out.Sources {
		byTitle[s.Title] = i
	}
	channel := out.Sources[byTitle["Example Channel"]]
	if channel.Kind != "youtube" || channel.FeedURL == nil || channel.Publication != nil {
		t.Errorf("youtube source = %+v", channel)
	}
	pub, ok := byTitle["Example Publication"]
	if !ok {
		t.Fatalf("standardfeed source should fall back to its catalog title: %s", publicTitles(out))
	}
	if s := out.Sources[pub]; s.Kind != "standardfeed" || s.FeedURL != nil || s.Publication == nil || *s.Publication != publicPub || s.SiteURL != "https://publication.example.com/" || !s.Primary {
		t.Errorf("standardfeed source = %+v", s)
	}
	untagged, ok := byTitle["https://untagged.example.com/rss"]
	if !ok {
		t.Fatalf("untitled feed should fall back to its feed URL: %s", publicTitles(out))
	}
	if s := out.Sources[untagged]; s.Kind != "rss" || s.Tags == nil || len(s.Tags) != 0 {
		t.Errorf("untagged rss source = %+v, want kind rss and tags []", s)
	}
}

func TestPublicFollowing_FiltersNarrowTheList(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  string
	}{
		{"tag=PERSONAL", "Example Blog | Example Channel"},
		{"tag=video,publications", "Example Channel | Example Publication"},
		{"tag=Personal&tag=Publications", "Example Blog | Example Channel | Example Publication"},
		{"excludeTag=video", "Example Blog | Example Publication | https://untagged.example.com/rss | Unlinked Publication"},
		{"excludeKind=youtube", "Example Blog | Example Publication | https://untagged.example.com/rss | Unlinked Publication"},
		{"kind=standardfeed", "Example Publication | Unlinked Publication"},
		{"kind=rss", "Example Blog | https://untagged.example.com/rss"},
		{"primary=true", "Example Publication | https://untagged.example.com/rss"},
		{"tag=personal&excludeKind=youtube", "Example Blog"},
		{"tag=personal&excludeTag=Video", "Example Blog"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			out := decodePublicJSON(t, getPublic(t, publicStore(), "json", "/user/"+publicReader+"/following.json?"+tc.query))
			if got := publicTitles(out); got != tc.want {
				t.Errorf("titles = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPublicFollowing_RejectsUnknownFilterValues(t *testing.T) {
	for _, target := range []string{
		"/user/" + publicReader + "/following.json?kind=podcast",
		"/user/" + publicReader + "/following.json?excludeKind=blog",
		"/user/" + publicReader + "/following.json?primary=yes",
		"/user/" + publicReader + "/following.opml?folders=flat",
	} {
		format := target[strings.LastIndex(target, ".")+1 : strings.Index(target, "?")]
		if rr := getPublic(t, publicStore(), format, target); rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"code":"invalid_request"`) {
			t.Errorf("%s = %d %s, want 400 invalid_request", target, rr.Code, rr.Body)
		}
	}
}

type opmlTree struct {
	Title    string        `xml:"head>title"`
	Outlines []opmlOutline `xml:"body>outline"`
}

func decodePublicOPML(t *testing.T, rr *httptest.ResponseRecorder) opmlTree {
	t.Helper()
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/x-opml") {
		t.Fatalf("status = %d, content type %q, body = %s", rr.Code, rr.Header().Get("Content-Type"), rr.Body)
	}
	var doc opmlTree
	if err := xml.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestPublicFollowing_OPMLGroupsByTagAndLinksPublications(t *testing.T) {
	doc := decodePublicOPML(t, getPublic(t, publicStore(), "opml", "/user/"+publicReader+"/following.opml"))
	var top []string
	folders := map[string][]opmlOutline{}
	for _, o := range doc.Outlines {
		if o.Outlines != nil {
			folders[o.Text] = o.Outlines
			continue
		}
		top = append(top, o.Text)
		if o.XMLURL != "https://untagged.example.com/rss" || o.Type != "rss" {
			t.Errorf("top-level outline = %+v", o)
		}
	}
	if strings.Join(top, ",") != "https://untagged.example.com/rss" {
		t.Errorf("untagged outlines = %q; the unlinked publication must be left out", top)
	}
	if len(folders) != 3 || len(folders["Personal"]) != 2 || len(folders["Video"]) != 1 {
		t.Errorf("folders = %+v", folders)
	}
	pubs := folders["Publications"]
	if len(pubs) != 1 {
		t.Fatalf("Publications folder = %+v", pubs)
	}
	if o := pubs[0]; o.Type != "link" || o.XMLURL != "" || o.URL != "https://publication.example.com/" || o.HTMLURL != "https://publication.example.com/" || o.Text != "Example Publication" {
		t.Errorf("publication outline = %+v", o)
	}
}

func TestPublicFollowing_OPMLWithoutFoldersListsEachSourceOnce(t *testing.T) {
	doc := decodePublicOPML(t, getPublic(t, publicStore(), "opml", "/user/"+publicReader+"/following.opml?folders=none&excludeKind=youtube"))
	var texts []string
	for _, o := range doc.Outlines {
		if o.Outlines != nil {
			t.Errorf("folders=none produced folder %q", o.Text)
		}
		texts = append(texts, o.Text)
	}
	if got := strings.Join(texts, " | "); got != "Example Blog | Example Publication | https://untagged.example.com/rss" {
		t.Errorf("outlines = %q", got)
	}
}

func TestPublicFollowingSetting_TogglesOnlyTheSessionReader(t *testing.T) {
	store := &fakePublicFollowing{enabled: map[string]bool{"did:plc:other": true}}
	get := func() string {
		rr := httptest.NewRecorder()
		PublicFollowingSettingHandler(store).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/public-following", nil), publicReader, "sid-1"))
		return strings.TrimSpace(rr.Body.String())
	}
	put := func(body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		PublicFollowingSettingUpdateHandler(store).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPut, "/api/public-following", strings.NewReader(body)), publicReader, "sid-1"))
		return rr
	}
	if got := get(); got != `{"enabled":false}` {
		t.Fatalf("default = %s", got)
	}
	if rr := put(`{"enabled":true}`); rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != `{"enabled":true}` {
		t.Fatalf("enable = %d %s", rr.Code, rr.Body)
	}
	if got := get(); got != `{"enabled":true}` {
		t.Errorf("after enable = %s", got)
	}
	if rr := put(`{}`); rr.Code != http.StatusBadRequest {
		t.Errorf("missing enabled = %d, want 400", rr.Code)
	}
	if rr := put(`{"enabled":false}`); rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != `{"enabled":false}` {
		t.Fatalf("disable = %d %s", rr.Code, rr.Body)
	}
	if !store.enabled["did:plc:other"] {
		t.Error("toggling one reader changed another reader's setting")
	}
}
