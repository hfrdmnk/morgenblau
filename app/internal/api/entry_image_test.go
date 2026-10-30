package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"morgenblau/internal/database/db"
)

// bandedPNG is green except for a red band across the vertical centre, so a centre crop of a tall image starts red and a top crop starts green.
func bandedPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		c := color.RGBA{0, 200, 0, 255}
		if y >= h/3 && y < h*2/3 {
			c = color.RGBA{220, 0, 0, 255}
		}
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func entryImageUpstream(t *testing.T, routes map[string][]byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func entryWithImage(postURL, image string) db.FeedEntry {
	entry := entryFixture()
	entry.Url = postURL
	meta := `{"image":` + string(must(json.Marshal(image))) + `}`
	entry.Metadata = &meta
	return entry
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func entryImageRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/entries/abc123/image", nil)
	req.SetPathValue("slug", "abc123")
	return withSession(req, "did:plc:alice", "sid-1")
}

func TestEntryImage_CropsToTheLayoutRangeAndScalesDownWithoutUpscaling(t *testing.T) {
	upstream, _ := entryImageUpstream(t, map[string][]byte{
		"/img/portrait.png": bandedPNG(t, 2000, 3000),
		"/img/panorama.png": bandedPNG(t, 3000, 500),
		"/img/small.png":    bandedPNG(t, 400, 300),
	})
	cases := []struct {
		image         string
		width, height int
	}{
		{"/img/portrait.png", 720, 540},
		{upstream.URL + "/img/panorama.png", 720, 360},
		{"../img/small.png", 400, 300},
	}
	for _, tc := range cases {
		t.Run(tc.image, func(t *testing.T) {
			reader := &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", tc.image), subOK: true}
			rr := httptest.NewRecorder()
			EntryImageHandler(reader, upstream.Client()).ServeHTTP(rr, entryImageRequest())
			if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "image/jpeg" {
				t.Fatalf("status = %d, type = %q, body = %s", rr.Code, rr.Header().Get("Content-Type"), rr.Body.String())
			}
			got, err := jpeg.Decode(rr.Body)
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds().Dx() != tc.width || got.Bounds().Dy() != tc.height {
				t.Errorf("size = %v, want %dx%d", got.Bounds().Size(), tc.width, tc.height)
			}
		})
	}
}

func TestEntryImage_TallImagesKeepTheirCentre(t *testing.T) {
	upstream, _ := entryImageUpstream(t, map[string][]byte{"/img/tall.png": bandedPNG(t, 300, 900)})
	reader := &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", "/img/tall.png"), subOK: true}
	rr := httptest.NewRecorder()
	EntryImageHandler(reader, upstream.Client()).ServeHTTP(rr, entryImageRequest())
	got, err := jpeg.Decode(rr.Body)
	if err != nil {
		t.Fatalf("status = %d: %v", rr.Code, err)
	}
	r, g, _, _ := got.At(got.Bounds().Dx()/2, 2).RGBA()
	if r>>8 < 150 || g>>8 > 80 {
		t.Errorf("top of the preview is rgb(%d,%d,…), want the red centre band rather than the green top", r>>8, g>>8)
	}
}

func TestEntryImage_CachesByImageURL(t *testing.T) {
	upstream, hits := entryImageUpstream(t, map[string][]byte{"/img/a.png": bandedPNG(t, 800, 600)})
	reader := &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", "/img/a.png"), subOK: true}
	h := EntryImageHandler(reader, upstream.Client())
	for range 2 {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, entryImageRequest())
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("upstream hits = %d, want 1", hits.Load())
	}
}

func TestEntryImage_RefusesWhatItShouldNotServe(t *testing.T) {
	upstream, hits := entryImageUpstream(t, map[string][]byte{
		"/img/a.png":    bandedPNG(t, 800, 600),
		"/img/page":     []byte("<!doctype html><p>not an image</p>"),
		"/img/huge.png": bandedPNG(t, 5000, 4000),
	})
	plain := entryFixture()
	plain.Metadata = nil
	cases := []struct {
		name      string
		reader    *fakeEntryReader
		status    int
		fetchesAt bool
	}{
		{"feed not followed", &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", "/img/a.png")}, http.StatusNotFound, false},
		{"entry without image", &fakeEntryReader{entry: plain, subOK: true}, http.StatusNotFound, false},
		{"script URL", &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", "javascript:alert(1)"), subOK: true}, http.StatusNotFound, false},
		{"not an image", &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", "/img/page"), subOK: true}, http.StatusBadGateway, true},
		{"over the pixel cap", &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", "/img/huge.png"), subOK: true}, http.StatusBadGateway, true},
		{"upstream 404", &fakeEntryReader{entry: entryWithImage(upstream.URL+"/posts/a", "/img/missing.png"), subOK: true}, http.StatusBadGateway, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := hits.Load()
			rr := httptest.NewRecorder()
			EntryImageHandler(tc.reader, upstream.Client()).ServeHTTP(rr, entryImageRequest())
			if rr.Code != tc.status {
				t.Errorf("status = %d, want %d", rr.Code, tc.status)
			}
			if fetched := hits.Load() > before; fetched != tc.fetchesAt {
				t.Errorf("fetched upstream = %v, want %v", fetched, tc.fetchesAt)
			}
		})
	}
}
