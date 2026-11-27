package api

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	lru "github.com/hashicorp/golang-lru/v2"
	_ "golang.org/x/image/webp"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/safehttp"
)

const (
	avatarDisplaySize   = 32
	avatarSize          = 3 * avatarDisplaySize
	avatarFetchTimeout  = 10 * time.Second
	avatarMaxBytes      = 4 * 1024 * 1024
	avatarCacheCapacity = 2048
	avatarMaxSide       = 4096
)

func avatarPath(p profiles.Profile) *string {
	cid := avatarCIDOf(p)
	if cid == "" {
		return nil
	}
	path := "/api/avatars/" + p.DID + "/" + cid
	return &path
}

func avatarCIDOf(p profiles.Profile) string {
	if p.Avatar == nil {
		return ""
	}
	u, err := url.Parse(*p.Avatar)
	if err != nil {
		return ""
	}
	return u.Query().Get("cid")
}

func AvatarHandler(src ProfileSource, client *http.Client) http.Handler {
	thumbnailsByCID, _ := lru.New[string, []byte](avatarCacheCapacity)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSession(w, r); !ok {
			return
		}
		did, err := syntax.ParseDID(r.PathValue("did"))
		cid := r.PathValue("cid")
		if err != nil || cid == "" {
			writeError(w, http.StatusNotFound, codeNotFound, "not found")
			return
		}
		thumb, ok := thumbnailsByCID.Get(cid)
		if !ok {
			profile, err := src.Get(r.Context(), did)
			if err != nil || avatarCIDOf(profile) != cid {
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			thumb, err = fetchAvatarThumbnail(r.Context(), client, *profile.Avatar)
			if err != nil {
				slog.Warn("/api/avatars: thumbnail failed", "did", did, "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
				return
			}
			thumbnailsByCID.Add(cid, thumb)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		if _, err := w.Write(thumb); err != nil {
			slog.Warn("/api/avatars: write failed", "err", err)
		}
	})
}

func fetchAvatarThumbnail(ctx context.Context, client *http.Client, blobURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, avatarFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, avatarMaxBytes))
	if err != nil {
		return nil, err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if config.Width > avatarMaxSide || config.Height > avatarMaxSide {
		return nil, fmt.Errorf("avatar is %dx%d, over the %d px limit", config.Width, config.Height, avatarMaxSide)
	}
	src, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, squareThumbnailOnWhite(src, avatarSize), &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func squareThumbnailOnWhite(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		sy0 := y0 + y*side/size
		sy1 := max(y0+(y+1)*side/size, sy0+1)
		for x := range size {
			sx0 := x0 + x*side/size
			sx1 := max(x0+(x+1)*side/size, sx0+1)
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			white := 0xffff - a/n
			dst.Set(x, y, color.RGBA64{uint16(r/n + white), uint16(g/n + white), uint16(bl/n + white), 0xffff})
		}
	}
	return dst
}
