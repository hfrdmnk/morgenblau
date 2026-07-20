package personsearch

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const (
	didAlice = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	didBob   = "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"
	didCarol = "did:plc:cccccccccccccccccccccccc"
	didDave  = "did:plc:dddddddddddddddddddddddd"
)

type fakeTypeahead struct {
	actors []Actor
	err    error
	gotQ   string
	gotLim int
}

func (f *fakeTypeahead) SearchActorsTypeahead(_ context.Context, q string, limit int) ([]Actor, error) {
	f.gotQ, f.gotLim = q, limit
	return f.actors, f.err
}

type fakePresence struct {
	present   map[string]bool
	presErr   error
	hints     map[string][]string
	hintErr   error
	hintCalls []string
}

func (f *fakePresence) Presence(_ context.Context, _ []string) (map[string]bool, error) {
	if f.presErr != nil {
		return nil, f.presErr
	}
	return f.present, nil
}

func (f *fakePresence) TasteHints(_ context.Context, did string, _ int) ([]string, error) {
	f.hintCalls = append(f.hintCalls, did)
	if f.hintErr != nil {
		return nil, f.hintErr
	}
	return f.hints[did], nil
}

func resultDIDs(results []Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.DID
	}
	return out
}

func TestSearch_PartitionsPresentFirstPreservingAppViewOrder(t *testing.T) {
	ta := &fakeTypeahead{actors: []Actor{
		{DID: didAlice, Handle: "alice.example"},
		{DID: didBob, Handle: "bob.example"},
		{DID: didCarol, Handle: "carol.example"},
		{DID: didDave, Handle: "dave.example"},
	}}
	pr := &fakePresence{present: map[string]bool{didBob: true, didCarol: true}}

	got, err := NewSearcher(ta, pr).Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := []string{didBob, didCarol, didAlice, didDave}
	if names := resultDIDs(got); !reflect.DeepEqual(names, want) {
		t.Errorf("order = %v, want present-first with AppView order preserved %v", names, want)
	}
	for _, r := range got {
		wantPresent := r.DID == didBob || r.DID == didCarol
		if r.InReaderNetwork != wantPresent {
			t.Errorf("%s InReaderNetwork = %v, want %v", r.DID, r.InReaderNetwork, wantPresent)
		}
	}
}

func TestSearch_PassesTypeaheadLimit(t *testing.T) {
	ta := &fakeTypeahead{}
	if _, err := NewSearcher(ta, &fakePresence{}).Search(context.Background(), "hi"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if ta.gotQ != "hi" {
		t.Errorf("q = %q, want %q", ta.gotQ, "hi")
	}
	if ta.gotLim != typeaheadLimit {
		t.Errorf("limit = %d, want %d", ta.gotLim, typeaheadLimit)
	}
}

func TestSearch_FoldsTasteHintsToTwoForPresentOnly(t *testing.T) {
	ta := &fakeTypeahead{actors: []Actor{
		{DID: didAlice, Handle: "alice.example"},
		{DID: didBob, Handle: "bob.example"},
	}}
	pr := &fakePresence{
		present: map[string]bool{didAlice: true},
		hints:   map[string][]string{didAlice: {"Example Weekly", "Example Review", "Example Times"}},
	}

	got, err := NewSearcher(ta, pr).Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	alice := got[0]
	if alice.DID != didAlice {
		t.Fatalf("expected alice first, got %s", alice.DID)
	}
	if len(alice.TasteHint) != maxTasteHints {
		t.Errorf("alice TasteHint = %v, want folded to %d", alice.TasteHint, maxTasteHints)
	}
	if want := []string{"Example Weekly", "Example Review"}; !reflect.DeepEqual(alice.TasteHint, want) {
		t.Errorf("alice TasteHint = %v, want the first two %v", alice.TasteHint, want)
	}
	if len(pr.hintCalls) != 1 || pr.hintCalls[0] != didAlice {
		t.Errorf("hint calls = %v, want only the present didAlice", pr.hintCalls)
	}
}

func TestSearch_AbsentResultFollowableNoHint(t *testing.T) {
	ta := &fakeTypeahead{actors: []Actor{{DID: didAlice, Handle: "alice.example"}}}
	pr := &fakePresence{present: map[string]bool{}}

	got, err := NewSearcher(ta, pr).Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].InReaderNetwork {
		t.Errorf("absent result InReaderNetwork = true, want false")
	}
	if got[0].TasteHint != nil {
		t.Errorf("absent result TasteHint = %v, want nil", got[0].TasteHint)
	}
	if got[0].Handle != "alice.example" {
		t.Errorf("handle = %q, want passed through so the result stays followable", got[0].Handle)
	}
	if len(pr.hintCalls) != 0 {
		t.Errorf("hint calls = %v, want none for an absent person", pr.hintCalls)
	}
}

func TestSearch_PresenceErrorDegradesToAllAbsent(t *testing.T) {
	ta := &fakeTypeahead{actors: []Actor{
		{DID: didAlice, Handle: "alice.example"},
		{DID: didBob, Handle: "bob.example"},
	}}
	pr := &fakePresence{presErr: errors.New("db down")}

	got, err := NewSearcher(ta, pr).Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("Search should degrade, not fail: %v", err)
	}
	if names := resultDIDs(got); !reflect.DeepEqual(names, []string{didAlice, didBob}) {
		t.Errorf("order = %v, want AppView order preserved when all absent", names)
	}
	for _, r := range got {
		if r.InReaderNetwork {
			t.Errorf("%s InReaderNetwork = true, want all absent on presence failure", r.DID)
		}
		if r.TasteHint != nil {
			t.Errorf("%s TasteHint = %v, want none when presence failed", r.DID, r.TasteHint)
		}
	}
	if len(pr.hintCalls) != 0 {
		t.Errorf("hint calls = %v, want none when presence failed", pr.hintCalls)
	}
}

func TestSearch_TypeaheadErrorPropagates(t *testing.T) {
	wantErr := errors.New("appview 502")
	ta := &fakeTypeahead{err: wantErr}

	_, err := NewSearcher(ta, &fakePresence{}).Search(context.Background(), "x")
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want the typeahead error propagated", err)
	}
}

func TestSearch_TasteHintErrorDegradesToNoHint(t *testing.T) {
	ta := &fakeTypeahead{actors: []Actor{{DID: didAlice, Handle: "alice.example"}}}
	pr := &fakePresence{present: map[string]bool{didAlice: true}, hintErr: errors.New("hint read failed")}

	got, err := NewSearcher(ta, pr).Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("Search should not fail on a taste-hint error: %v", err)
	}
	if len(got) != 1 || !got[0].InReaderNetwork {
		t.Fatalf("got %+v, want alice still badged present", got)
	}
	if got[0].TasteHint != nil {
		t.Errorf("TasteHint = %v, want none when the hint read failed", got[0].TasteHint)
	}
}
