package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
)

const avatarCID = "bafkreiexampleavatar"

// avatarUpstream serves a 400x200 PNG whose outer quarters are green, so a squashed thumbnail shows green and a centre crop shows only red then blue.
func avatarUpstream(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := range 200 {
		for x := range 400 {
			c := color.RGBA{0, 200, 0, 255}
			if x >= 100 && x < 200 {
				c = color.RGBA{220, 0, 0, 255}
			} else if x >= 200 && x < 300 {
				c = color.RGBA{0, 0, 220, 255}
			}
			img.Set(x, y, c)
		}
	}
	var body bytes.Buffer
	if err := png.Encode(&body, img); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func avatarProfiles(upstream string) *fakeProfileSource {
	did := syntax.DID("did:plc:alice")
	u := upstream + "/xrpc/com.atproto.sync.getBlob?" + url.Values{"did": {did.String()}, "cid": {avatarCID}}.Encode()
	return &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{did: {DID: did.String(), Handle: "alice.example", Avatar: &u}}}
}

func avatarRequest(cid string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/avatars/did:plc:alice/"+cid, nil)
	req.SetPathValue("did", "did:plc:alice")
	req.SetPathValue("cid", cid)
	return withSession(req, "did:plc:bob", "sid-1")
}

func TestAvatar_ServesCentreCroppedThumbnailOnceThenFromCache(t *testing.T) {
	upstream, hits := avatarUpstream(t)
	h := AvatarHandler(avatarProfiles(upstream.URL), upstream.Client())

	for range 2 {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, avatarRequest(avatarCID))
		if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("status = %d, type = %q, body = %s", rr.Code, rr.Header().Get("Content-Type"), rr.Body.String())
		}
		img, err := jpeg.Decode(rr.Body)
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != avatarSize || b.Dy() != avatarSize {
			t.Fatalf("size = %v, want %dx%d", b, avatarSize, avatarSize)
		}
		left, _, _, _ := img.At(4, avatarSize/2).RGBA()
		_, _, right, _ := img.At(avatarSize-5, avatarSize/2).RGBA()
		_, edgeGreen, _, _ := img.At(1, avatarSize/2).RGBA()
		if left < 0xa000 || right < 0xa000 || edgeGreen > 0x4000 {
			t.Errorf("thumbnail is not a centre crop: left red %x, right blue %x, edge green %x", left, right, edgeGreen)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("upstream fetched %d times, want 1", hits.Load())
	}
}

func TestAvatar_RefusesCIDThatIsNotTheProfilesAvatar(t *testing.T) {
	upstream, hits := avatarUpstream(t)
	rr := httptest.NewRecorder()
	AvatarHandler(avatarProfiles(upstream.URL), upstream.Client()).ServeHTTP(rr, avatarRequest("bafkreisomethingelse"))
	if rr.Code != http.StatusNotFound || hits.Load() != 0 {
		t.Fatalf("status = %d, upstream hits = %d", rr.Code, hits.Load())
	}
}

func TestMeProfile_PointsAvatarAtThumbnail(t *testing.T) {
	rr := httptest.NewRecorder()
	MeProfileHandler(avatarProfiles("https://pds.example.com"), nil, nil, nil).ServeHTTP(rr,
		withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil), "did:plc:alice", "sid-1"))
	var got meResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Avatar == nil || *got.Avatar != "/api/avatars/did:plc:alice/"+avatarCID {
		t.Fatalf("avatar = %v", got.Avatar)
	}
}

func TestAvatar_AcceptsWebP(t *testing.T) {
	// An 8x4 solid red lossless WebP.
	body, _ := base64.StdEncoding.DecodeString("UklGRhwAAABXRUJQVlA4TA8AAAAvB8AAAAcQ/Y/+ByKi/wEA")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(body)
	}))
	t.Cleanup(upstream.Close)
	rr := httptest.NewRecorder()
	AvatarHandler(avatarProfiles(upstream.URL), upstream.Client()).ServeHTTP(rr, avatarRequest(avatarCID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	img, err := jpeg.Decode(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	if r, g, _, _ := img.At(avatarSize/2, avatarSize/2).RGBA(); r < 0xd000 || g > 0x3000 {
		t.Errorf("centre pixel is not red: r %x g %x", r, g)
	}
}

func TestAvatar_RefusesImagesTooLargeToDecodeSafely(t *testing.T) {
	// Only the header matters: decoding is refused from the claimed dimensions before any pixels are allocated.
	var body bytes.Buffer
	if err := png.Encode(&body, image.NewGray(image.Rect(0, 0, avatarMaxSide+1, 1))); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body.Bytes())
	}))
	t.Cleanup(upstream.Close)
	rr := httptest.NewRecorder()
	AvatarHandler(avatarProfiles(upstream.URL), upstream.Client()).ServeHTTP(rr, avatarRequest(avatarCID))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}
