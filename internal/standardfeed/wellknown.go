package standardfeed

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

const wellKnownPath = "/.well-known/site.standard.publication"

// FetchWellKnown probes a site for its Standardfeed publication: GET
// https://<host>/.well-known/site.standard.publication, whose body is the
// publication's at-uri as plain text. A miss (non-200, unparsable body,
// wrong collection) returns ("", nil) — the probe is best-effort and callers
// treat miss and transport error alike.
func (c *Client) FetchWellKnown(ctx context.Context, siteURL string) (string, error) {
	base, err := url.Parse(siteURL)
	if err != nil || base.Host == "" {
		return "", nil
	}
	scheme := base.Scheme
	if scheme == "" {
		scheme = "https"
	}
	probe := url.URL{Scheme: scheme, Host: base.Host, Path: wellKnownPath}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.String(), nil)
	if err != nil {
		return "", nil
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	uri, err := syntax.ParseATURI(strings.TrimSpace(string(body)))
	if err != nil {
		return "", nil
	}
	if uri.Collection().String() != CollectionPublication {
		return "", nil
	}
	return uri.String(), nil
}
