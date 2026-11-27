package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/image/draw"
	"golang.org/x/net/html"

	"morgenblau/internal/safehttp"
)

const (
	widestDigestSlotCSSPx      = 360
	entryImageWidth            = 2 * widestDigestSlotCSSPx
	entryImageFetchTimeout     = 10 * time.Second
	entryImageMaxBytes         = 8 * 1024 * 1024
	entryImageMaxPixels        = 16_000_000
	entryImageCacheCapacity    = 256
	entryImageMaxDecodes       = 2
	entryBodyImageMaxBytes     = 20 * 1024 * 1024
	entryBodyImageFetchTimeout = 30 * time.Second
	entryBodyImageCSP          = "default-src 'none'; style-src 'unsafe-inline'; sandbox"
)

func entryImagePath(slug string, metadata *string, entryURL string) *string {
	if entryImageURL(metadata, entryURL) == "" {
		return nil
	}
	path := "/api/entries/" + url.PathEscape(slug) + "/image"
	return &path
}

func entryImageURL(metadata *string, entryURL string) string {
	if metadata == nil {
		return ""
	}
	var meta struct {
		Image string `json:"image"`
	}
	if json.Unmarshal([]byte(*metadata), &meta) != nil {
		return ""
	}
	return resolveImageURL(meta.Image, entryURL)
}

func resolveImageURL(raw, entryURL string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if base, err := url.Parse(entryURL); err == nil {
		ref = base.ResolveReference(ref)
	}
	if (ref.Scheme != "http" && ref.Scheme != "https") || ref.Host == "" {
		return ""
	}
	return ref.String()
}

func EntryImageHandler(reader EntryReader, client *http.Client) http.Handler {
	cache, _ := lru.New[string, []byte](entryImageCacheCapacity)
	decodes := make(chan struct{}, entryImageMaxDecodes)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry, _, _, ok := loadAndAuthorize(w, r, reader)
		if !ok {
			return
		}
		src := entryImageURL(entry.Metadata, entry.Url)
		if src == "" {
			writeError(w, http.StatusNotFound, codeNotFound, "not found")
			return
		}
		preview, ok := cache.Get(src)
		if !ok {
			var err error
			preview, err = fetchEntryImage(r.Context(), client, src, decodes)
			if err != nil {
				slog.Warn("/api/entries/image: preview failed", "url", src, "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
				return
			}
			cache.Add(src, preview)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		if _, err := w.Write(preview); err != nil {
			slog.Warn("/api/entries/image: write failed", "err", err)
		}
	})
}

func fetchEntryImage(ctx context.Context, client *http.Client, src string, decodes chan struct{}) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, entryImageFetchTimeout)
	defer cancel()
	resp, err := getImage(ctx, client, src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > entryImageMaxBytes {
		return nil, fmt.Errorf("image is %d bytes, over the %d byte limit", resp.ContentLength, entryImageMaxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, entryImageMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > entryImageMaxBytes {
		return nil, fmt.Errorf("image is over the %d byte limit", entryImageMaxBytes)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if config.Width*config.Height > entryImageMaxPixels {
		return nil, fmt.Errorf("image is %dx%d, over the %d pixel limit", config.Width, config.Height, entryImageMaxPixels)
	}
	select {
	case decodes <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-decodes }()
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, previewImage(img), &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func EntryBodyImageHandler(reader EntryReader, client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry, _, _, ok := loadAndAuthorize(w, r, reader)
		if !ok {
			return
		}
		raw := r.URL.Query().Get("src")
		src := resolveImageURL(raw, entry.Url)
		if src == "" || !bodyShowsImage(entry.ContentHtml, raw) && !bodyShowsImage(entry.ExtractedBody, raw) {
			writeError(w, http.StatusNotFound, codeNotFound, "not found")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), entryBodyImageFetchTimeout)
		defer cancel()
		resp, err := getImage(ctx, client, src)
		if err != nil {
			slog.Warn("/api/entries/images: fetch failed", "url", src, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}
		defer resp.Body.Close()
		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(strings.ToLower(contentType), "image/") || resp.ContentLength > entryBodyImageMaxBytes {
			slog.Warn("/api/entries/images: refused", "url", src, "type", contentType, "length", resp.ContentLength)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", entryBodyImageCSP)
		w.Header().Set("Cache-Control", "private, max-age=86400")
		if _, err := io.Copy(w, io.LimitReader(resp.Body, entryBodyImageMaxBytes)); err != nil {
			slog.Warn("/api/entries/images: stream failed", "url", src, "err", err)
		}
	})
}

func bodyShowsImage(body *string, raw string) bool {
	if body == nil {
		return false
	}
	z := html.NewTokenizer(strings.NewReader(*body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			name, more := z.TagName()
			if string(name) != "img" {
				continue
			}
			for more {
				var key, val []byte
				key, val, more = z.TagAttr()
				if string(key) == "src" && string(val) == raw {
					return true
				}
			}
		}
	}
}

func getImage(ctx context.Context, client *http.Client, src string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return resp, nil
}

func previewImage(src image.Image) *image.RGBA {
	crop := src.Bounds()
	w, h := crop.Dx(), crop.Dy()
	if h*4 > w*3 {
		keep := max(w*3/4, 1)
		crop.Min.Y += (h - keep) / 2
		crop.Max.Y = crop.Min.Y + keep
	} else if w > h*2 {
		keep := h * 2
		crop.Min.X += (w - keep) / 2
		crop.Max.X = crop.Min.X + keep
	}
	width := min(crop.Dx(), entryImageWidth)
	height := max(crop.Dy()*width/crop.Dx(), 1)
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.BiLinear.Scale(dst, dst.Bounds(), src, crop, draw.Over, nil)
	return dst
}
