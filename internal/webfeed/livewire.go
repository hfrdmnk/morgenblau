package webfeed

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"morgenblau/internal/safehttp"
)

// lazyMarker is the call every Livewire lazy placeholder makes; a page without it is never tokenized.
var lazyMarker = []byte("__lazyLoad(")

var reLazyLoad = regexp.MustCompile(`__lazyLoad\(\s*['"]([^'"]+)['"]\s*\)`)

const (
	maxLazyComponents = 4
	defaultUpdateURI  = "/livewire/update"
)

// lazyPlaceholder is one lazy component's element, as byte offsets into the page.
type lazyPlaceholder struct {
	start, end int
	snapshot   string
	param      string
}

// livewirePage is what the page tells its own script about loading its lazy components.
type livewirePage struct {
	placeholders []lazyPlaceholder
	updateURI    string
	csrf         string
}

// expandLazy loads a listing's Livewire lazy components with the one POST the page's script makes and splices their
// HTML in; any failure returns r untouched and false.
func (s *session) expandLazy(ctx context.Context, r *response, lang string, cache *RobotsCache) (*response, bool) {
	if r.err != nil || r.status < 200 || r.status > 299 || !bytes.Contains(r.body, lazyMarker) {
		return r, false
	}
	lw := scanLivewire(r.body)
	if len(lw.placeholders) == 0 {
		return r, false
	}
	listing, err := url.Parse(r.url)
	if err != nil {
		return r, false
	}
	update, err := listing.Parse(lw.updateURI)
	if err != nil || (update.Scheme != "http" && update.Scheme != "https") || !strings.EqualFold(update.Host, listing.Host) {
		return r, false
	}
	update.Fragment = ""
	if !s.robotsAllow(ctx, update.String(), cache) || s.remaining() <= 0 {
		return r, false
	}
	htmls, ok := s.postLazy(ctx, update.String(), lang, lw, r.header)
	if !ok {
		return r, false
	}
	var b bytes.Buffer
	prev := 0
	for i, ph := range lw.placeholders {
		b.Write(r.body[prev:ph.start])
		b.WriteString(htmls[i])
		prev = ph.end
	}
	b.Write(r.body[prev:])
	return &response{url: r.url, status: r.status, header: r.header, body: b.Bytes()}, true
}

// postLazy sends every placeholder's __lazyLoad call in one request and returns each component's HTML in order.
func (s *session) postLazy(ctx context.Context, updateURL, lang string, lw livewirePage, listingHeader http.Header) ([]string, bool) {
	type call struct {
		Path   string   `json:"path"`
		Method string   `json:"method"`
		Params []string `json:"params"`
	}
	type component struct {
		Snapshot string   `json:"snapshot"`
		Updates  struct{} `json:"updates"`
		Calls    []call   `json:"calls"`
	}
	payload := struct {
		Token      string      `json:"_token"`
		Components []component `json:"components"`
	}{Token: lw.csrf}
	for _, ph := range lw.placeholders {
		payload.Components = append(payload.Components, component{Snapshot: ph.snapshot, Calls: []call{{Method: "__lazyLoad", Params: []string{ph.param}}}})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, updateURL, bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	req.Header.Set("Content-Type", "application/json")
	// Livewire's endpoint answers only requests that carry its header, as the page's script sends them.
	req.Header.Set("X-Livewire", "1")
	if lang := safehttp.LanguageTag(lang); lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	var cookies []string
	for _, c := range (&http.Response{Header: listingHeader}).Cookies() {
		cookies = append(cookies, c.Name+"="+c.Value)
	}
	if len(cookies) > 0 {
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, false
	}
	body, err := s.readBody(resp.Body, maxBody)
	if err != nil {
		return nil, false
	}
	var out struct {
		Components []struct {
			Effects struct {
				HTML string `json:"html"`
			} `json:"effects"`
		} `json:"components"`
	}
	if json.Unmarshal(body, &out) != nil || len(out.Components) != len(lw.placeholders) {
		return nil, false
	}
	htmls := make([]string, len(out.Components))
	for i, c := range out.Components {
		if strings.TrimSpace(c.Effects.HTML) == "" {
			return nil, false
		}
		htmls[i] = c.Effects.HTML
	}
	return htmls, true
}

// scanLivewire finds the first lazy placeholders and the Livewire script's update URI and token.
func scanLivewire(body []byte) livewirePage {
	lw := livewirePage{updateURI: defaultUpdateURI}
	z := html.NewTokenizer(bytes.NewReader(body))
	offset := 0
	// open is the placeholder being read to its end tag; depth counts nested elements of its tag name.
	var open *lazyPlaceholder
	openTag, depth := "", 0
	scriptSeen := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return lw
		}
		start := offset
		offset += len(z.Raw())
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			if open != nil {
				if tag == openTag && tt == html.StartTagToken {
					depth++
				}
				continue
			}
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				if _, dup := attrs[string(k)]; !dup {
					attrs[string(k)] = string(v)
				}
			}
			if tag == "script" && !scriptSeen {
				if uri, ok := attrs["data-update-uri"]; ok {
					scriptSeen = true
					if uri = strings.TrimSpace(uri); uri != "" {
						lw.updateURI = uri
					}
					lw.csrf = attrs["data-csrf"]
				}
			}
			snapshot, ok := attrs["wire:snapshot"]
			if !ok || tt != html.StartTagToken || len(lw.placeholders) >= maxLazyComponents {
				continue
			}
			if param := lazyCallParam(attrs); param != "" {
				open = &lazyPlaceholder{start: start, snapshot: snapshot, param: param}
				openTag, depth = tag, 0
			}
		case html.EndTagToken:
			if open == nil {
				continue
			}
			if name, _ := z.TagName(); string(name) != openTag {
				continue
			}
			if depth > 0 {
				depth--
				continue
			}
			open.end = offset
			lw.placeholders = append(lw.placeholders, *open)
			open = nil
		}
	}
}

// lazyCallParam is the argument of the __lazyLoad call in a placeholder's x-init or x-intersect, with any modifiers.
func lazyCallParam(attrs map[string]string) string {
	for k, v := range attrs {
		if k == "x-init" || k == "x-intersect" || strings.HasPrefix(k, "x-intersect.") {
			if m := reLazyLoad.FindStringSubmatch(v); m != nil {
				return m[1]
			}
		}
	}
	return ""
}
