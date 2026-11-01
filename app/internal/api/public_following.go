package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/tags"
)

// PublicFollowingReader is the narrow read behind /user/{did}/following.*.
type PublicFollowingReader interface {
	PublicFollowingEnabled(ctx context.Context, did string) (bool, error)
	ListPublicFollowingSources(ctx context.Context, did string) ([]db.ListPublicFollowingSourcesRow, error)
}

type publicSource struct {
	Title       string   `json:"title"`
	Kind        string   `json:"kind"`
	FeedURL     string   `json:"feedUrl,omitempty"`
	Publication string   `json:"publication,omitempty"`
	SiteURL     string   `json:"siteUrl,omitempty"`
	Tags        []string `json:"tags"`
	Primary     bool     `json:"primary"`
}

type publicFollowing struct {
	DID     string         `json:"did"`
	Sources []publicSource `json:"sources"`
}

var publicKinds = []string{"rss", "youtube", "standardfeed"}

// PublicFollowingHandler serves an opted-in reader's feed subscriptions as format "json" or "opml" (SPEC <feed-sources>).
func PublicFollowingHandler(store PublicFollowingReader, format string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		did, err := syntax.ParseDID(r.PathValue("did"))
		if err != nil {
			NotFound(w, r)
			return
		}
		enabled, err := store.PublicFollowingEnabled(r.Context(), did.String())
		if err != nil {
			slog.Warn("public following: setting lookup failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		if !enabled {
			NotFound(w, r)
			return
		}
		keep, folders, problem := publicFilter(r.URL.Query(), format)
		if problem != "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, problem)
			return
		}
		rows, err := store.ListPublicFollowingSources(r.Context(), did.String())
		if err != nil {
			slog.Warn("public following: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		sources := []publicSource{}
		for _, row := range rows {
			if s := toPublicSource(row); keep(s) {
				sources = append(sources, s)
			}
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		if format == "json" {
			writeJSON(w, publicFollowing{DID: did.String(), Sources: sources})
			return
		}
		opml, err := encodeOPMLDocument("Following on Morgenblau", publicOutlines(sources), folders)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternalError, "Could not create the OPML file")
			return
		}
		w.Header().Set("Content-Type", "text/x-opml; charset=utf-8")
		_, _ = w.Write([]byte(opml))
	})
}

func toPublicSource(row db.ListPublicFollowingSourcesRow) publicSource {
	s := publicSource{Kind: "rss", SiteURL: derefStr(row.SiteUrl), Tags: tags.Unmarshal(row.Tags), Primary: row.IsPrimary != 0}
	if s.Tags == nil {
		s.Tags = []string{}
	}
	if row.Kind == "standardfeed" {
		s.Kind, s.Publication = "standardfeed", row.FeedUrl
	} else {
		s.FeedURL = row.FeedUrl
		if isYouTubeFeed(row.FeedUrl) {
			s.Kind = "youtube"
		}
	}
	for _, title := range []string{derefStr(row.Title), derefStr(row.CatalogTitle), s.SiteURL, row.FeedUrl} {
		if title != "" {
			s.Title = title
			break
		}
	}
	return s
}

func isYouTubeFeed(feedURL string) bool {
	u, err := url.Parse(feedURL)
	return err == nil && (u.Host == "www.youtube.com" || u.Host == "youtube.com") && u.Path == "/feeds/videos.xml"
}

// publicOutlines links a publication through its site, since it has no feed URL; one whose site is not known yet has nothing to link.
func publicOutlines(sources []publicSource) []taggedOutline {
	outlines := make([]taggedOutline, 0, len(sources))
	for _, s := range sources {
		o := opmlOutline{Text: s.Title, Title: s.Title, Type: "rss", XMLURL: s.FeedURL, HTMLURL: s.SiteURL}
		if s.Kind == "standardfeed" {
			if s.SiteURL == "" {
				continue
			}
			o = opmlOutline{Text: s.Title, Title: s.Title, Type: "link", URL: s.SiteURL, HTMLURL: s.SiteURL}
		}
		outlines = append(outlines, taggedOutline{o, s.Tags})
	}
	return outlines
}

// publicFilter parses the query into a predicate; a non-empty problem means a value it does not understand.
func publicFilter(q url.Values, format string) (keep func(publicSource) bool, folders bool, problem string) {
	onlyTags, skipTags := publicList(q, "tag"), publicList(q, "excludeTag")
	onlyKinds, skipKinds := publicList(q, "kind"), publicList(q, "excludeKind")
	for _, kind := range append(slices.Clone(onlyKinds), skipKinds...) {
		if !slices.Contains(publicKinds, kind) {
			return nil, false, "kind must be one of " + strings.Join(publicKinds, ", ")
		}
	}
	primary := q.Get("primary")
	if primary != "" && primary != "true" {
		return nil, false, "primary must be true"
	}
	folders = true
	if format == "opml" {
		switch q.Get("folders") {
		case "", "tags":
		case "none":
			folders = false
		default:
			return nil, false, "folders must be tags or none"
		}
	}
	hasTag := func(s publicSource, want []string) bool {
		return slices.ContainsFunc(s.Tags, func(tag string) bool { return slices.Contains(want, strings.ToLower(tag)) })
	}
	return func(s publicSource) bool {
		return (len(onlyTags) == 0 || hasTag(s, onlyTags)) && !hasTag(s, skipTags) &&
			(len(onlyKinds) == 0 || slices.Contains(onlyKinds, s.Kind)) && !slices.Contains(skipKinds, s.Kind) &&
			(primary == "" || s.Primary)
	}, folders, ""
}

// publicList reads a filter given as repeated or comma-separated values, lowercased so tags match the way the app dedupes them.
func publicList(q url.Values, name string) []string {
	var out []string
	for _, value := range q[name] {
		for part := range strings.SplitSeq(value, ",") {
			if part = strings.ToLower(strings.TrimSpace(part)); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// --- GET/PUT /api/public-following ---

type PublicFollowingSettingReader interface {
	PublicFollowingEnabled(ctx context.Context, did string) (bool, error)
}

type PublicFollowingSettingWriter interface {
	SetPublicFollowing(ctx context.Context, arg db.SetPublicFollowingParams) error
}

type publicFollowingSetting struct {
	Enabled *bool `json:"enabled"`
}

func PublicFollowingSettingHandler(store PublicFollowingSettingReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		enabled, err := store.PublicFollowingEnabled(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/public-following: read failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		writeJSON(w, publicFollowingSetting{Enabled: &enabled})
	})
}

func PublicFollowingSettingUpdateHandler(store PublicFollowingSettingWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body publicFollowingSetting
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Enabled == nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "enabled is required")
			return
		}
		setting := db.SetPublicFollowingParams{Did: sess.Data.AccountDID.String(), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
		if *body.Enabled {
			setting.PublicFollowing = 1
		}
		err := store.SetPublicFollowing(r.Context(), setting)
		if err != nil {
			slog.Warn("/api/public-following: write failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		writeJSON(w, body)
	})
}
