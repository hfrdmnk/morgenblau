package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"morgenblau/internal/webfeed"
)

const maxCachedBody = 10 << 20

// cachingDoer replays earlier responses from disk so repeated tuning runs do not hammer the sites.
type cachingDoer struct {
	dir  string
	next webfeed.Doer
}

type cachedResponse struct {
	Status   int         `json:"status"`
	Header   http.Header `json:"header"`
	FinalURL string      `json:"final_url"`
	Body     []byte      `json:"body"`
}

func (c cachingDoer) Do(req *http.Request) (*http.Response, error) {
	sum := sha256.Sum256([]byte(req.Method + " " + req.URL.String() + " " + req.Header.Get("Accept-Language")))
	path := filepath.Join(c.dir, hex.EncodeToString(sum[:]))
	if raw, err := os.ReadFile(path); err == nil {
		var cr cachedResponse
		if json.Unmarshal(raw, &cr) == nil {
			return cr.response(req), nil
		}
	}
	resp, err := c.next.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCachedBody))
	if err != nil {
		return nil, err
	}
	cr := cachedResponse{Status: resp.StatusCode, Header: resp.Header, FinalURL: req.URL.String(), Body: body}
	if resp.Request != nil && resp.Request.URL != nil {
		cr.FinalURL = resp.Request.URL.String()
	}
	// Refusals and server errors are often transient; replaying them would pin a bad day into every later run.
	if cacheable := cr.Status < 400 || cr.Status == http.StatusNotFound || cr.Status == http.StatusGone; !cacheable {
		return cr.response(req), nil
	}
	if raw, err := json.Marshal(cr); err == nil {
		_ = os.WriteFile(path, raw, 0o600)
	}
	return cr.response(req), nil
}

func (cr cachedResponse) response(req *http.Request) *http.Response {
	final := req
	if u, err := url.Parse(cr.FinalURL); err == nil {
		final = req.Clone(req.Context())
		final.URL = u
	}
	return &http.Response{
		StatusCode: cr.Status,
		Header:     cr.Header,
		Body:       io.NopCloser(bytes.NewReader(cr.Body)),
		Request:    final,
	}
}
