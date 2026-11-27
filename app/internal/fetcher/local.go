package fetcher

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func WithLocalFeeds(isLocal func(*url.URL) bool, h http.Handler) Option {
	return func(f *Fetcher) {
		f.localFeeds = &localFeeds{isLocal: isLocal, handler: h}
	}
}

type localFeeds struct {
	isLocal func(*url.URL) bool
	handler http.Handler
	next    http.RoundTripper
}

func (l localFeeds) RoundTrip(req *http.Request) (*http.Response, error) {
	if !l.isLocal(req.URL) {
		return l.next.RoundTrip(req)
	}
	w := &bufferedResponse{header: http.Header{}}
	l.handler.ServeHTTP(w, req)
	return w.response(req), nil
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *bufferedResponse) Header() http.Header { return w.header }

func (w *bufferedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *bufferedResponse) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(p)
}

func (w *bufferedResponse) response(req *http.Request) *http.Response {
	w.WriteHeader(http.StatusOK)
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", w.status, http.StatusText(w.status)),
		StatusCode:    w.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        w.header,
		Body:          io.NopCloser(bytes.NewReader(w.body.Bytes())),
		ContentLength: int64(w.body.Len()),
		Request:       req,
	}
}
