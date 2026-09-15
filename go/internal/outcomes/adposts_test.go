package outcomes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

func marketingAgainst(t *testing.T, handler http.HandlerFunc) (*graph.MarketingClient, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	transport, err := graph.NewTransport(graph.TransportOptions{
		AccessToken: "test-token",
		GraphOrigin: server.URL,
		Sleep:       func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return graph.NewMarketingClient(transport), server.Close
}

// An ad whose creative has no Page post produces a NOTE, not a silent omission.
// An empty answer would read as "no comments" when it means "not visible here".
func TestAnAdWithNoPagePostSaysSo(t *testing.T) {
	marketing, done := marketingAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"creative":{"id":"cr1"}}`))
	})
	defer done()

	adID := "ad1"
	postIDs, notes, err := ResolveAdPosts(context.Background(), marketing, AdTarget{Ad: &adID})
	if err != nil {
		t.Fatalf("ResolveAdPosts: %v", err)
	}
	if len(postIDs) != 0 {
		t.Fatalf("postIDs = %v, want none", postIDs)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want one", notes)
	}
	if !strings.Contains(notes[0], "ad1") {
		t.Errorf("note does not name the ad: %q", notes[0])
	}
	if !strings.Contains(notes[0], "cannot be read from the Page") {
		t.Errorf("note does not say what the caller is missing: %q", notes[0])
	}
}

// effective_object_story_id wins: it resolves to the real post even when the
// creative was defined inline.
func TestEffectiveStoryIDWinsOverObjectStoryID(t *testing.T) {
	marketing, done := marketingAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"creative":{"id":"cr1","effective_object_story_id":"pg1_effective","object_story_id":"pg1_explicit"}}`))
	})
	defer done()

	adID := "ad1"
	postIDs, _, err := ResolveAdPosts(context.Background(), marketing, AdTarget{Ad: &adID})
	if err != nil {
		t.Fatalf("ResolveAdPosts: %v", err)
	}
	if len(postIDs) != 1 || postIDs[0] != "pg1_effective" {
		t.Fatalf("postIDs = %v, want [pg1_effective]", postIDs)
	}
}

// And object_story_id is the fallback, not dead code.
func TestObjectStoryIDIsUsedWhenEffectiveIsAbsent(t *testing.T) {
	marketing, done := marketingAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"creative":{"id":"cr1","object_story_id":"pg1_explicit"}}`))
	})
	defer done()

	adID := "ad1"
	postIDs, _, err := ResolveAdPosts(context.Background(), marketing, AdTarget{Ad: &adID})
	if err != nil {
		t.Fatalf("ResolveAdPosts: %v", err)
	}
	if len(postIDs) != 1 || postIDs[0] != "pg1_explicit" {
		t.Fatalf("postIDs = %v, want [pg1_explicit]", postIDs)
	}
}

// A campaign fans out through ad sets to ads, and several ads commonly share one
// post — which must come back once, in a deterministic order.
func TestACampaignDeduplicatesSharedPostsDeterministically(t *testing.T) {
	marketing, done := marketingAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/adsets"):
			_, _ = w.Write([]byte(`{"data":[{"id":"as1"},{"id":"as2"}]}`))
		case strings.Contains(r.URL.Path, "/ads"):
			if strings.Contains(r.URL.Path, "as1") {
				_, _ = w.Write([]byte(`{"data":[{"id":"ad1"},{"id":"ad2"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"ad3"}]}`))
		case strings.Contains(r.URL.Path, "ad3"):
			_, _ = w.Write([]byte(`{"creative":{"id":"cr3","effective_object_story_id":"pg1_second"}}`))
		default:
			// ad1 and ad2 both point at the same post.
			_, _ = w.Write([]byte(`{"creative":{"id":"cr1","effective_object_story_id":"pg1_shared"}}`))
		}
	})
	defer done()

	campaignID := "c1"
	for range 10 {
		postIDs, _, err := ResolveAdPosts(context.Background(), marketing, AdTarget{Campaign: &campaignID})
		if err != nil {
			t.Fatalf("ResolveAdPosts: %v", err)
		}
		if len(postIDs) != 2 {
			t.Fatalf("postIDs = %v, want two", postIDs)
		}
		if postIDs[0] != "pg1_shared" || postIDs[1] != "pg1_second" {
			t.Fatalf("postIDs = %v, want [pg1_shared pg1_second] every run", postIDs)
		}
	}
}

func TestResolvingNothingIsAnError(t *testing.T) {
	marketing, done := marketingAgainst(t, func(w http.ResponseWriter, r *http.Request) {})
	defer done()

	if _, _, err := ResolveAdPosts(context.Background(), marketing, AdTarget{}); err == nil {
		t.Fatal("expected an error with neither an ad nor a campaign")
	}
}
