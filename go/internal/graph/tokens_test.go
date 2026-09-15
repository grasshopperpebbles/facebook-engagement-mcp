package graph

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Graph can list a Page and withhold its access_token, and that is not the same
// fault as Graph never listing the Page. Reporting the second for both sends a
// reader to check administration and pages_show_list for a case that may be
// neither — which is how a Business Manager System User arrives here when the
// asset assignment is wrong (T-20).
func TestTokenForDistinguishesNotListedFromNoToken(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"pg1","access_token":"page-token","tasks":["MANAGE","MODERATE"]},
			{"id":"pg2","tasks":["ANALYZE"]}
		]}`))
	})

	provider := NewTokenProvider("user-token", NewPagesClient(newTestTransport(t, server.URL)))
	ctx := context.Background()

	got, err := provider.TokenFor(ctx, "pg1")
	if err != nil || got != "page-token" {
		t.Fatalf("TokenFor(pg1) = %q, %v", got, err)
	}

	var accessErr *PageAccessError
	if _, err := provider.TokenFor(ctx, "pg2"); !errors.As(err, &accessErr) || accessErr.Reason != ReasonNoToken {
		t.Fatalf("TokenFor(pg2) should fail with no_token, got %v", err)
	}
	// The two messages must actually differ, or the distinction is decorative.
	noTokenMessage := accessErr.Error()
	if !strings.Contains(noTokenMessage, "listed") {
		t.Errorf("no_token message does not say Graph listed the Page: %q", noTokenMessage)
	}

	if _, err := provider.TokenFor(ctx, "pg9"); !errors.As(err, &accessErr) || accessErr.Reason != ReasonNotListed {
		t.Fatalf("TokenFor(pg9) should fail with not_listed, got %v", err)
	}
	if accessErr.Error() == noTokenMessage {
		t.Error("both causes produced the same message")
	}
}

// Without the cache a sweep asks for the same Page token once per post.
func TestTokenProviderFetchesAccountsOnce(t *testing.T) {
	calls := 0
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":[{"id":"pg1","access_token":"t"}]}`))
	})

	provider := NewTokenProvider("user-token", NewPagesClient(newTestTransport(t, server.URL)))
	for range 3 {
		if _, err := provider.TokenFor(context.Background(), "pg1"); err != nil {
			t.Fatalf("TokenFor: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("/me/accounts was read %d times; want 1", calls)
	}
}

// Concurrent callers share one exchange rather than racing several.
func TestConcurrentCallersShareOneExchange(t *testing.T) {
	calls := 0
	var mu sync.Mutex
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":[{"id":"pg1","access_token":"t"}]}`))
	})

	provider := NewTokenProvider("user-token", NewPagesClient(newTestTransport(t, server.URL)))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = provider.TokenFor(context.Background(), "pg1")
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("/me/accounts was read %d times under concurrency; want 1", calls)
	}
}

// A failed exchange must not poison the cache — the next call retries.
func TestAFailedExchangeIsNotCached(t *testing.T) {
	attempt := 0
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"transient","code":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"pg1","access_token":"t"}]}`))
	})

	provider := NewTokenProvider("user-token", NewPagesClient(newTestTransport(t, server.URL)))
	ctx := context.Background()

	if _, err := provider.TokenFor(ctx, "pg1"); err == nil {
		t.Fatal("expected the first exchange to fail")
	}
	if got, err := provider.TokenFor(ctx, "pg1"); err != nil || got != "t" {
		t.Fatalf("second TokenFor = %q, %v — a failed exchange poisoned the cache", got, err)
	}
}

// Absent and empty are different answers, and a caller refusing a write on a
// missing role must act on the empty one.
func TestTasksForKeepsAbsentDistinctFromEmpty(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"pg1","access_token":"t1","tasks":["MODERATE"]},
			{"id":"pg2","access_token":"t2","tasks":[]},
			{"id":"pg3","access_token":"t3"}
		]}`))
	})

	provider := NewTokenProvider("user-token", NewPagesClient(newTestTransport(t, server.URL)))
	ctx := context.Background()

	withRole, err := provider.TasksFor(ctx, "pg1")
	if err != nil || withRole == nil || len(*withRole) != 1 {
		t.Fatalf("pg1 tasks = %v, %v", withRole, err)
	}

	known, err := provider.TasksFor(ctx, "pg2")
	if err != nil {
		t.Fatalf("pg2: %v", err)
	}
	if known == nil {
		t.Fatal("pg2 tasks = nil; Graph returned the field empty, which is an answer")
	}
	if len(*known) != 0 {
		t.Fatalf("pg2 tasks = %v, want empty", *known)
	}

	unknown, err := provider.TasksFor(ctx, "pg3")
	if err != nil {
		t.Fatalf("pg3: %v", err)
	}
	if unknown != nil {
		t.Fatalf("pg3 tasks = %v, want nil — Graph did not say", *unknown)
	}
}

// The cached credential is shared, so a caller that received the live slice
// could push a role into it and change what every later caller sees.
func TestTasksForReturnsACopy(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"pg1","access_token":"t","tasks":["ANALYZE"]}]}`))
	})

	provider := NewTokenProvider("user-token", NewPagesClient(newTestTransport(t, server.URL)))
	ctx := context.Background()

	first, err := provider.TasksFor(ctx, "pg1")
	if err != nil {
		t.Fatalf("TasksFor: %v", err)
	}
	*first = append(*first, "MODERATE")

	second, err := provider.TasksFor(ctx, "pg1")
	if err != nil {
		t.Fatalf("TasksFor: %v", err)
	}
	if len(*second) != 1 {
		t.Fatalf("a caller mutated the cached roles: %v", *second)
	}
}

func TestTokenForUserReturnsTheConfiguredToken(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	provider := NewTokenProvider("user-token", NewPagesClient(newTestTransport(t, server.URL)))
	if got := provider.TokenForUser(); got != "user-token" {
		t.Fatalf("TokenForUser = %q", got)
	}
}
