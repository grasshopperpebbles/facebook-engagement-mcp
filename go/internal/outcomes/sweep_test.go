package outcomes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

// stub serves recorded Graph responses and records every path asked for.
type stub struct {
	*httptest.Server
	mu       sync.Mutex
	paths    []string
	routes   map[string]string
	failing  map[string]bool
	fallback string
}

func newStub(t *testing.T, routes map[string]string) *stub {
	t.Helper()
	s := &stub{routes: routes, failing: map[string]bool{}, fallback: `{"data":[]}`}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /vNN.N prefix so routes read like Graph edges.
		path := r.URL.Path
		if idx := strings.Index(path[1:], "/"); idx >= 0 {
			path = path[idx+1:]
		}

		s.mu.Lock()
		s.paths = append(s.paths, path)
		body, known := s.routes[path]
		failing := s.failing[path]
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream","code":1}}`))
			return
		}
		if !known {
			_, _ = w.Write([]byte(s.fallback))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *stub) requested(suffix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.paths {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

func (s *stub) deps(t *testing.T) Deps {
	t.Helper()
	newTransport := func(token string) (*graph.Transport, error) {
		return graph.NewTransport(graph.TransportOptions{
			AccessToken: token,
			GraphOrigin: s.URL,
			Sleep:       func(time.Duration) {},
			MaxAttempts: 1,
		})
	}
	transport, err := newTransport("user-token")
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	pages := graph.NewPagesClient(transport)

	return Deps{
		Pages:     pages,
		Marketing: graph.NewMarketingClient(transport),
		Tokens:    graph.NewTokenProvider("user-token", pages),
		NewPagesClient: func(token string) (*graph.PagesClient, error) {
			bound, err := newTransport(token)
			if err != nil {
				return nil, err
			}
			return graph.NewPagesClient(bound), nil
		},
	}
}

const accountsWithPage = `{"data":[{"id":"pg1","name":"Test Page","access_token":"page-token","tasks":["MODERATE","MANAGE"]}]}`

// T-11, confirmed live 2026-09-11. A third level is accepted and `parent` points
// at the reply, not at the top-level comment. Stopping after one level meant a
// visitor's answer to the Page's reply was never read — and since triage asks
// who spoke last, an unread last word made a waiting customer look handled.
func TestSweepReadsThirdLevelReplies(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts":     accountsWithPage,
		"/pg1/feed":        `{"data":[{"id":"pg1_p1","message":"post","created_time":"2026-09-01T09:00:00+0000","is_published":true}]}`,
		"/pg1_p1/comments": `{"data":[{"id":"c1","message":"Is this in stock?","created_time":"2026-09-02T10:00:00+0000","comment_count":1,"from":{"id":"visitor","name":"A Customer"}}]}`,
		"/c1/comments":     `{"data":[{"id":"c1_r","message":"Yes","created_time":"2026-09-02T11:00:00+0000","comment_count":1,"from":{"id":"pg1","name":"Test Page"}}]}`,
		"/c1_r/comments":   `{"data":[{"id":"c1_r_r","message":"How do I order?","created_time":"2026-09-02T12:00:00+0000","comment_count":0,"from":{"id":"visitor","name":"A Customer"}}]}`,
	})

	got, err := RunCommentActivity(context.Background(), s.deps(t), RunOptions{
		Page: "pg1", Filter: FilterAll, GroupBy: GroupByPost,
	})
	if err != nil {
		t.Fatalf("RunCommentActivity: %v", err)
	}

	if !s.requested("/c1_r/comments") {
		t.Fatal("the third level was never requested")
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Threads) != 1 {
		t.Fatalf("groups = %+v", got.Groups)
	}
	thread := got.Groups[0].Threads[0]
	if len(thread.Replies) != 2 {
		t.Fatalf("replies = %d, want 2 (second and third level flattened)", len(thread.Replies))
	}
	// The visitor spoke last, so the Page is still on the hook.
	if thread.Status != string(StatusNeedsReply) {
		t.Fatalf("status = %s, want needs_reply", thread.Status)
	}
}

// One failing post must not fail the whole answer — it becomes a labelled gap.
func TestOneFailingPostBecomesANoteNotAFailure(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts": accountsWithPage,
		"/pg1/feed": `{"data":[
			{"id":"pg1_p1","created_time":"2026-09-01T09:00:00+0000","is_published":true},
			{"id":"pg1_p2","created_time":"2026-09-01T09:00:00+0000","is_published":true}
		]}`,
		"/pg1_p1/comments": `{"data":[{"id":"c1","message":"hi","created_time":"2026-09-02T10:00:00+0000","comment_count":0,"from":{"id":"visitor"}}]}`,
	})
	s.failing["/pg1_p2/comments"] = true

	got, err := RunCommentActivity(context.Background(), s.deps(t), RunOptions{
		Page: "pg1", Filter: FilterAll,
	})
	if err != nil {
		t.Fatalf("RunCommentActivity failed instead of degrading: %v", err)
	}
	if !got.Partial {
		t.Error("partial = false after a post could not be read")
	}

	named := false
	for _, note := range got.Notes {
		if strings.Contains(note, "pg1_p2") {
			named = true
		}
	}
	if !named {
		t.Errorf("no note names the failing post: %v", got.Notes)
	}
	if got.Totals.Threads != 1 {
		t.Errorf("threads = %d; the readable post's comments were lost too", got.Totals.Threads)
	}
}

// Orientation is the call a model makes before it knows what it needs. An
// implementation that swept a Page to answer it has the same output and the
// wrong cost.
func TestOrientationReadsNoComments(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts":   `{"data":[{"id":"pg1","name":"Test Page","tasks":["MODERATE"]}]}`,
		"/me/adaccounts": `{"data":[{"id":"act_1","name":"Main"}]}`,
		"/debug_token":   `{"data":{"type":"USER","is_valid":true,"expires_at":0}}`,
	})

	got, err := RunOrientation(context.Background(), s.deps(t))
	if err != nil {
		t.Fatalf("RunOrientation: %v", err)
	}
	if len(got.Pages) != 1 || len(got.AdAccounts) != 1 {
		t.Fatalf("orientation = %+v", got)
	}
	if s.requested("/comments") {
		t.Error("orientation read comments")
	}
	if s.requested("/feed") {
		t.Error("orientation swept a Page")
	}
}

// A token without ads_read still orients: the missing scope becomes a note, not
// an error. Failing the whole answer would hide the Pages the token CAN reach
// behind a permission it may never use.
func TestATokenWithoutAdsReadStillOrients(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts": `{"data":[{"id":"pg1","name":"Test Page"}]}`,
		"/debug_token": `{"data":{"type":"USER","is_valid":true}}`,
	})
	s.failing["/me/adaccounts"] = true

	got, err := RunOrientation(context.Background(), s.deps(t))
	if err != nil {
		t.Fatalf("RunOrientation failed over a missing scope: %v", err)
	}
	if len(got.Pages) != 1 {
		t.Fatalf("pages = %+v", got.Pages)
	}
	if len(got.Notes) == 0 {
		t.Error("no note explains the missing ad accounts")
	}
}

// The ad route must NOT also sweep the Page. An implementation that did both
// would pass every output assertion while having missed the point, which is that
// no Page-level sweep reaches an unpublished post.
func TestAnAdResolvesToThePostBehindItWithoutSweeping(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts":       accountsWithPage,
		"/ad1":               `{"creative":{"id":"cr1","effective_object_story_id":"pg1_dark"}}`,
		"/pg1_dark/comments": `{"data":[{"id":"c1","message":"Interested","created_time":"2026-09-02T10:00:00+0000","comment_count":0,"from":{"id":"visitor"}}]}`,
	})

	got, err := RunCommentActivity(context.Background(), s.deps(t), RunOptions{
		Ad: "ad1", Filter: FilterAll,
	})
	if err != nil {
		t.Fatalf("RunCommentActivity: %v", err)
	}
	if !s.requested("/pg1_dark/comments") {
		t.Fatal("the resolved post's comments were never read")
	}
	if s.requested("/pg1/feed") {
		t.Fatal("the ad route also swept the Page; no Page sweep reaches an unpublished post")
	}
	if got.Totals.Threads != 1 {
		t.Fatalf("threads = %d, want 1", got.Totals.Threads)
	}
}

// Sort before capping. Comments arrive oldest-first and post by post, so an
// unsorted slice keeps the oldest threads while the note claims the newest —
// dropping exactly the comments a triage tool exists to surface.
func TestCappingKeepsTheNewestThreads(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts": accountsWithPage,
		"/pg1/feed": `{"data":[
			{"id":"pg1_old","created_time":"2026-09-01T09:00:00+0000","is_published":true},
			{"id":"pg1_new","created_time":"2026-09-01T09:00:00+0000","is_published":true}
		]}`,
		"/pg1_old/comments": `{"data":[{"id":"old1","created_time":"2026-09-01T10:00:00+0000","comment_count":0,"from":{"id":"visitor"}}]}`,
		"/pg1_new/comments": `{"data":[{"id":"new1","created_time":"2026-09-05T10:00:00+0000","comment_count":0,"from":{"id":"visitor"}}]}`,
	})

	got, err := RunCommentActivity(context.Background(), s.deps(t), RunOptions{
		Page: "pg1", Filter: FilterAll, GroupBy: GroupByNone, MaxThreads: 1,
	})
	if err != nil {
		t.Fatalf("RunCommentActivity: %v", err)
	}
	if got.Totals.Threads != 1 {
		t.Fatalf("threads = %d, want 1", got.Totals.Threads)
	}
	kept := got.Groups[0].Threads[0].Comment.ID
	if kept != "new1" {
		t.Fatalf("kept %s, want new1 — it capped before sorting", kept)
	}
	if !got.Partial {
		t.Error("partial = false after threads were dropped")
	}
}

// A post reached by id says nothing about publication, and `since` is echoed
// with a note saying it did not apply.
func TestAPostTargetSaysSinceDidNotApply(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts":     accountsWithPage,
		"/pg1_p1/comments": `{"data":[{"id":"c1","created_time":"2026-09-02T10:00:00+0000","comment_count":0,"from":{"id":"visitor"}}]}`,
	})

	got, err := RunCommentActivity(context.Background(), s.deps(t), RunOptions{
		Post: "pg1_p1", Filter: FilterAll,
	})
	if err != nil {
		t.Fatalf("RunCommentActivity: %v", err)
	}
	if len(got.Posts) != 1 || got.Posts[0].IsPublished != nil {
		t.Fatalf("posts = %+v; a post reached by id has no published state to report", got.Posts)
	}

	said := false
	for _, note := range got.Notes {
		if strings.Contains(note, "'since'") {
			said = true
		}
	}
	if !said {
		t.Errorf("no note says `since` did not apply: %v", got.Notes)
	}
}

// Two targets is a refusal, not a guess about which was meant.
func TestTwoTargetsIsRefused(t *testing.T) {
	s := newStub(t, nil)
	_, err := RunCommentActivity(context.Background(), s.deps(t), RunOptions{Page: "pg1", Post: "pg1_p1"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "one target") {
		t.Fatalf("error does not explain: %v", err)
	}
}

// T-37. A story id the photos edge knows and the sweep never saw is a candidate
// post this token cannot read — but only a REFUSAL counts, because a cover photo
// names a page_story_id that /feed does not list and reads back with a plain 200.
func TestWithheldPostsAreCountedOnlyWhenTheyRefuse(t *testing.T) {
	s := newStub(t, map[string]string{
		"/me/accounts":     accountsWithPage,
		"/pg1/feed":        `{"data":[{"id":"pg1_p1","created_time":"2026-09-01T09:00:00+0000","is_published":true}]}`,
		"/pg1_p1/comments": `{"data":[]}`,
		"/pg1/photos": `{"data":[
			{"id":"ph1","page_story_id":"pg1_cover"},
			{"id":"ph2","page_story_id":"pg1_withheld"}
		]}`,
		// The cover photo's story reads back fine. It must not be counted.
		"/pg1_cover/comments": `{"data":[]}`,
	})
	s.failing["/pg1_withheld/comments"] = true

	got, err := RunCommentActivity(context.Background(), s.deps(t), RunOptions{
		Page: "pg1", Filter: FilterAll,
	})
	if err != nil {
		t.Fatalf("RunCommentActivity: %v", err)
	}

	found := ""
	for _, note := range got.Notes {
		if strings.Contains(note, "could not be read with these credentials") {
			found = note
		}
	}
	if found == "" {
		t.Fatalf("no note reports the withheld post: %v", got.Notes)
	}
	if !strings.Contains(found, "At least 1 post") {
		t.Errorf("note counted the readable cover photo's story too: %q", found)
	}
	// The count is a floor and the note must say so — a text post leaves no media.
	if !strings.Contains(found, "minimum") {
		t.Errorf("note does not say the count is a floor: %q", found)
	}
}
