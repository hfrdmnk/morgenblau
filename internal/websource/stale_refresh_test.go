package websource

import (
	"context"
	"errors"
	"testing"
	"time"
)

// staleUnsureSource stores an unsure source for the site and lets its verdict go stale.
func staleUnsureSource(t *testing.T, env *testEnv) Preview {
	t.Helper()
	env.gen.generated.Health = HealthNeedsAttention
	first, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	env.advance(staleSetup)
	return first
}

func TestCreateFallsBackToTheExistingSourceWhenARefreshFails(t *testing.T) {
	for name, setupErr := range map[string]error{
		"generation":  errors.New("listing parse failed"),
		"unreachable": ErrUnreachable,
	} {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			first := staleUnsureSource(t, env)
			env.gen.setupErr = setupErr

			got, err := env.service.Create(context.Background(), "did:plc:visitor", "https://news.example.com/", "")
			if err != nil {
				t.Fatalf("refresh failure reached the reader: %v", err)
			}
			if got.Source.ID != first.Source.ID || got.ItemCount != first.ItemCount {
				t.Errorf("preview = %+v, want the existing source", got.Source)
			}
			if n := env.gen.setupCount(); n != 2 {
				t.Fatalf("setups = %d, want the refresh attempted once", n)
			}

			env.advance(time.Minute)
			if _, err := env.service.Create(context.Background(), "did:plc:another", "https://news.example.com/", ""); err != nil {
				t.Fatal(err)
			}
			if n := env.gen.setupCount(); n != 2 {
				t.Errorf("setups = %d, want the failed refresh to space the next one by staleSetup", n)
			}
			env.advance(staleSetup)
			if _, err := env.service.Create(context.Background(), "did:plc:another", "https://news.example.com/", ""); err != nil {
				t.Fatal(err)
			}
			if n := env.gen.setupCount(); n != 3 {
				t.Errorf("setups = %d, want a retry once staleSetup passed", n)
			}
		})
	}
}

func TestCreateSkipsARefreshTheReaderCannotRunNow(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		env := newTestEnv(t)
		first := staleUnsureSource(t, env)
		env.service.limiter = &fakeLimiter{limit: 0, used: map[string]int{}}
		got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
		if err != nil || got.Source.ID != first.Source.ID {
			t.Fatalf("preview = %+v, err = %v; want the existing source", got.Source, err)
		}
		if n := env.gen.setupCount(); n != 1 {
			t.Errorf("setups = %d, want no refresh", n)
		}
	})
	t.Run("setup in flight", func(t *testing.T) {
		env := newTestEnv(t)
		first := staleUnsureSource(t, env)
		release, err := env.service.claimReader(testDID)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
		if err != nil || got.Source.ID != first.Source.ID {
			t.Fatalf("preview = %+v, err = %v; want the existing source", got.Source, err)
		}
	})
	t.Run("busy", func(t *testing.T) {
		env := newTestEnv(t)
		first := staleUnsureSource(t, env)
		for range setupSlots {
			env.service.slots <- struct{}{}
		}
		env.service.setupTimeout = 10 * time.Millisecond
		got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
		if err != nil || got.Source.ID != first.Source.ID {
			t.Fatalf("preview = %+v, err = %v; want the existing source", got.Source, err)
		}
	})
}

func TestCreateStillFailsAnAnchoredRequestWhoseSetupFails(t *testing.T) {
	env := newTestEnv(t)
	staleUnsureSource(t, env)
	env.gen.setupErr = ErrUnreachable
	_, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/posts/mine")
	if !errors.Is(err, ErrUnreachable) {
		t.Errorf("err = %v, want the anchored reader told the site did not answer", err)
	}
}
