package newsletter

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"morgenblau/internal/database/db"
)

func TestRandomLocalPartHasThreeMemorableWords(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`)
	seen := make(map[string]bool)
	for range 64 {
		localPart, err := randomLocalPart()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(localPart) || len(localPart) > 64 {
			t.Fatalf("local part %q must be three lowercase words within SMTP's 64-byte limit", localPart)
		}
		for i, word := range strings.Split(localPart, "-") {
			pool := [][]string{addressAdjectives, addressAnimals, addressPlaces}[i]
			found := false
			for _, candidate := range pool {
				found = found || candidate == word
			}
			if !found {
				t.Fatalf("%q is in the wrong word category at position %d", localPart, i)
			}
		}
		seen[localPart] = true
	}
	if len(seen) < 60 {
		t.Fatalf("only %d distinct addresses from 64 generations", len(seen))
	}
}

func TestAddressWordPoolsAreDistinctAndEmailSafe(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z]+$`)
	namespace, longest := 1, 2
	for i, pool := range [][]string{addressAdjectives, addressAnimals, addressPlaces} {
		seen := make(map[string]bool)
		maxLength := 0
		for _, word := range pool {
			if seen[word] || !pattern.MatchString(word) {
				t.Errorf("pool %d has a duplicate or unsafe word: %q", i, word)
			}
			seen[word] = true
			maxLength = max(maxLength, len(word))
		}
		if len(seen) < 256 {
			t.Errorf("pool %d has only %d distinct words", i, len(seen))
		}
		namespace *= len(seen)
		longest += maxLength
		t.Logf("pool %d: %d distinct words", i, len(seen))
	}
	if longest > 64 {
		t.Fatalf("longest local part is %d bytes, SMTP allows 64", longest)
	}
	t.Logf("%d possible addresses; longest local part is %d bytes", namespace, longest)
}

func TestCreateAddressRetriesTakenWords(t *testing.T) {
	service, _, writer := newTestService(t)
	ctx := context.Background()
	for did, localPart := range map[string]string{
		"did:plc:first": "quiet-goose-meadow",
		"did:plc:other": "calm-otter-harbor",
	} {
		if err := db.New(writer).CreateNewsletterAddress(ctx, db.CreateNewsletterAddressParams{
			Did: did, LocalPart: localPart, CreatedAt: formatTime(testNow),
		}); err != nil {
			t.Fatal(err)
		}
	}
	candidates := []string{"quiet-goose-meadow", "calm-otter-harbor", "bright-wren-valley"}
	calls := 0
	service.newLocalPart = func() (string, error) {
		if calls >= len(candidates) {
			t.Fatal("generated another address after creation")
		}
		value := candidates[calls]
		calls++
		return value, nil
	}
	address, err := service.CreateAddress(ctx, "did:plc:new")
	if err != nil || address != "bright-wren-valley@news.example" || calls != 3 {
		t.Fatalf("address=%q calls=%d error=%v", address, calls, err)
	}
	address, err = service.CreateAddress(ctx, "did:plc:new")
	if err != nil || address != "bright-wren-valley@news.example" || calls != 3 {
		t.Fatalf("reused address=%q calls=%d error=%v", address, calls, err)
	}
	first, err := service.Address(ctx, "did:plc:first")
	if err != nil || first != "quiet-goose-meadow@news.example" {
		t.Fatalf("collision changed the existing owner: %q, %v", first, err)
	}
}

func TestConcurrentAddressCollisionKeepsOwnersDistinct(t *testing.T) {
	service, _, _ := newTestService(t)
	var calls atomic.Int32
	service.newLocalPart = func() (string, error) {
		if calls.Add(1) <= 2 {
			return "quiet-goose-meadow", nil
		}
		return "bright-wren-valley", nil
	}
	type result struct {
		did, address string
		err          error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, did := range []string{"did:plc:first", "did:plc:other"} {
		go func() {
			<-start
			address, err := service.CreateAddress(context.Background(), did)
			results <- result{did: did, address: address, err: err}
		}()
	}
	close(start)
	seen := make(map[string]bool)
	for range 2 {
		got := <-results
		if got.err != nil || seen[got.address] {
			t.Fatalf("address=%q error=%v duplicates=%v", got.address, got.err, seen)
		}
		seen[got.address] = true
		stored, err := service.Address(context.Background(), got.did)
		if err != nil || stored != got.address {
			t.Fatalf("owner %s got %q but stored %q: %v", got.did, got.address, stored, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("generator calls=%d, want 3", calls.Load())
	}
}

func TestCreateAddressReturnsGenerationFailureWithoutRetry(t *testing.T) {
	service, _, _ := newTestService(t)
	want := errors.New("entropy unavailable")
	calls := 0
	service.newLocalPart = func() (string, error) {
		calls++
		return "", want
	}
	_, err := service.CreateAddress(context.Background(), "did:plc:new")
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}
