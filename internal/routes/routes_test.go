package routes

import (
	"strings"
	"testing"
)

func TestLoad_ParsesEmbeddedJSON(t *testing.T) {
	rs, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	want := []Route{
		{Path: "/", Auth: AuthPublic, AuthedRedirect: "/consume"},
		{Path: "/login", Auth: AuthPublic, AuthedRedirect: "/consume"},
		{Path: "/about", Auth: AuthPublic},
		{Path: "/consume", Auth: AuthAuthed},
		{Path: "/sources", Auth: AuthAuthed},
		{Path: "/discover", Auth: AuthAuthed},
		{Path: "/create", Auth: AuthAuthed},
		{Path: "/entry", Auth: AuthAuthed},
	}

	if len(rs) != len(want) {
		t.Fatalf("len(routes) = %d, want %d (got: %+v)", len(rs), len(want), rs)
	}
	for i, w := range want {
		if rs[i] != w {
			t.Errorf("route[%d] = %+v, want %+v", i, rs[i], w)
		}
	}
}

func TestParse_RejectsUnknownAuthValue(t *testing.T) {
	bad := []byte(`[{"path":"/x","auth":"bogus"}]`)
	if _, err := Parse(bad); err == nil {
		t.Fatal("Parse() expected error for unknown auth value, got nil")
	} else if !strings.Contains(err.Error(), "auth") {
		t.Errorf("error = %v, want it to mention auth", err)
	}
}

func TestParse_RejectsAuthedRedirectOnAuthedRoute(t *testing.T) {
	bad := []byte(`[{"path":"/x","auth":"authed","authedRedirect":"/y"}]`)
	if _, err := Parse(bad); err == nil {
		t.Fatal("Parse() expected error for authedRedirect on authed route, got nil")
	} else if !strings.Contains(err.Error(), "authedRedirect") {
		t.Errorf("error = %v, want it to mention authedRedirect", err)
	}
}

func TestParse_RejectsEmptyPath(t *testing.T) {
	bad := []byte(`[{"path":"","auth":"public"}]`)
	if _, err := Parse(bad); err == nil {
		t.Fatal("Parse() expected error for empty path, got nil")
	}
}

func TestParse_RejectsDuplicatePath(t *testing.T) {
	bad := []byte(`[{"path":"/x","auth":"public"},{"path":"/x","auth":"authed"}]`)
	if _, err := Parse(bad); err == nil {
		t.Fatal("Parse() expected error for duplicate path, got nil")
	} else if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %v, want it to mention duplicate", err)
	}
}
