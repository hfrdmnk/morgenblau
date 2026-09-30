package api

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"morgenblau/internal/lexicon"
)

type importSource struct {
	FeedURL string   `json:"feedUrl"`
	Title   string   `json:"title,omitempty"`
	SiteURL string   `json:"siteUrl,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type importPlan struct {
	Sources  []importSource `json:"sources"`
	Warnings []string       `json:"warnings"`
}

type opmlDocument struct {
	XMLName xml.Name  `xml:"opml"`
	Version string    `xml:"version,attr"`
	Title   string    `xml:"head>title"`
	Body    *opmlBody `xml:"body"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr,omitempty"`
	Type     string        `xml:"type,attr,omitempty"`
	XMLURL   string        `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string        `xml:"htmlUrl,attr,omitempty"`
	Outlines []opmlOutline `xml:"outline"`
}

func parseOPML(input string) (importPlan, error) {
	d := xml.NewDecoder(strings.NewReader(input))
	var doc opmlDocument
	if err := d.Decode(&doc); err != nil || doc.Body == nil {
		return importPlan{}, fmt.Errorf("Choose a valid OPML file with a body element")
	}
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return importPlan{}, fmt.Errorf("Invalid OPML document")
		}
		if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) == "" {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return importPlan{}, fmt.Errorf("Unexpected content after the OPML document")
	}
	var sources []importSource
	var walk func([]opmlOutline, []string, int) error
	walk = func(outlines []opmlOutline, parents []string, depth int) error {
		if depth > 64 {
			return fmt.Errorf("OPML folders are nested too deeply")
		}
		for _, o := range outlines {
			name := strings.TrimSpace(o.Text)
			if name == "" {
				name = strings.TrimSpace(o.Title)
			}
			path := append([]string(nil), parents...)
			if o.XMLURL != "" || strings.EqualFold(o.Type, "rss") || strings.EqualFold(o.Type, "atom") {
				sources = append(sources, importSource{FeedURL: o.XMLURL, Title: name, SiteURL: o.HTMLURL, Tags: path})
			} else if name != "" {
				path = append(path, name)
			}
			if err := walk(o.Outlines, path, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc.Body.Outlines, nil, 0); err != nil {
		return importPlan{}, err
	}
	return prepareImportSources(sources), nil
}

func prepareImportSources(sources []importSource) importPlan {
	plan := importPlan{Sources: []importSource{}, Warnings: []string{}}
	positions := map[string]int{}
	var merged []importSource
	for _, source := range sources {
		source.FeedURL = strings.TrimSpace(source.FeedURL)
		source.SiteURL = strings.TrimSpace(source.SiteURL)
		source.Title = strings.TrimSpace(source.Title)
		if i, ok := positions[source.FeedURL]; ok {
			merged[i].Tags = append(merged[i].Tags, source.Tags...)
			if merged[i].Title == "" {
				merged[i].Title = source.Title
			}
			if merged[i].SiteURL == "" {
				merged[i].SiteURL = source.SiteURL
			}
		} else {
			positions[source.FeedURL] = len(merged)
			merged = append(merged, source)
		}
	}
	for _, source := range merged {
		tagList, err := mergeImportTags(nil, source.Tags)
		if err == nil {
			source.Tags = tagList
			err = validateImportSource(source)
		}
		if err != nil {
			label := source.Title
			if label == "" {
				label = source.FeedURL
			}
			if label == "" {
				label = "Unnamed source"
			}
			plan.Warnings = append(plan.Warnings, label+": "+err.Error())
			continue
		}
		plan.Sources = append(plan.Sources, source)
	}
	return plan
}

func mergeImportTags(existing, incoming []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, group := range [][]string{existing, incoming} {
		for _, tag := range group {
			tag = strings.TrimSpace(tag)
			if tag == "" || seen[strings.ToLower(tag)] {
				continue
			}
			seen[strings.ToLower(tag)] = true
			out = append(out, tag)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	// Validate the union rather than normalizeTags, which silently discards overflow.
	if err := lexicon.ValidateRecord(subscriptionCollection, map[string]any{
		"source": sourceUnion("rss", "https://example.com/feed", ""), "createdAt": "2026-01-01T00:00:00Z", "tags": out,
	}); err != nil {
		return nil, fmt.Errorf("Tags must fit within 10 tags of 64 characters each")
	}
	return out, nil
}

func validateImportSource(s importSource) error {
	u, err := url.Parse(s.FeedURL)
	if err != nil || !isValidFeedURL(s.FeedURL) || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("A valid HTTP or HTTPS feed URL is required")
	}
	if s.SiteURL != "" && !isValidFeedURL(s.SiteURL) {
		return fmt.Errorf("Invalid website URL")
	}
	if err := lexicon.ValidateRecord(subscriptionCollection, importRecord(s, "2026-01-01T00:00:00Z")); err != nil {
		return fmt.Errorf("Source metadata exceeds the subscription limits")
	}
	return nil
}

func importRecord(s importSource, createdAt string) map[string]any {
	record := map[string]any{"$type": subscriptionCollection, "source": sourceUnion("rss", s.FeedURL, s.SiteURL), "createdAt": createdAt}
	if s.Title != "" {
		record["title"] = s.Title
	}
	if len(s.Tags) != 0 {
		record["tags"] = s.Tags
	}
	return record
}

func encodeOPML(sources []importSource) (string, error) {
	doc := opmlDocument{Version: "2.0", Title: "Morgenblau sources", Body: &opmlBody{}}
	folders := map[string][]opmlOutline{}
	for _, s := range sources {
		title := s.Title
		if title == "" {
			title = s.FeedURL
		}
		o := opmlOutline{Text: title, Title: title, Type: "rss", XMLURL: s.FeedURL, HTMLURL: s.SiteURL}
		if len(s.Tags) == 0 {
			doc.Body.Outlines = append(doc.Body.Outlines, o)
		}
		for _, tag := range s.Tags {
			folders[tag] = append(folders[tag], o)
		}
	}
	names := make([]string, 0, len(folders))
	for name := range folders {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		doc.Body.Outlines = append(doc.Body.Outlines, opmlOutline{Text: name, Outlines: folders[name]})
	}
	encoded, err := xml.MarshalIndent(doc, "", "  ")
	return xml.Header + string(encoded) + "\n", err
}
