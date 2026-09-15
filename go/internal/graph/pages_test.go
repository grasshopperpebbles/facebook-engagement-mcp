package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// recordingServer captures every request path and query the client makes.
type recordingServer struct {
	*httptest.Server
	requests []*url.URL
}

func newRecordingServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *recordingServer {
	t.Helper()
	rec := &recordingServer{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured := *r.URL
		rec.requests = append(rec.requests, &captured)
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(rec.Close)
	return rec
}

func (r *recordingServer) queryFor(t *testing.T, pathSuffix string) url.Values {
	t.Helper()
	for _, u := range r.requests {
		if strings.HasSuffix(u.Path, pathSuffix) {
			return u.Query()
		}
	}
	t.Fatalf("no request to *%s; made %d requests", pathSuffix, len(r.requests))
	return nil
}

// pages.List must never request credential material. The field constant is
// right today; this asserts the call site rather than trusting it to stay right.
func TestListNeverAsksForAccessTokens(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"pg1","name":"Test Page"}]}`))
	})

	client := NewPagesClient(newTestTransport(t, server.URL))
	pages, _, err := client.List(context.Background(), PageOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pages) != 1 || pages[0].ID != "pg1" {
		t.Fatalf("pages = %+v", pages)
	}

	fields := server.queryFor(t, "/me/accounts").Get("fields")
	if strings.Contains(fields, "access_token") {
		t.Fatalf("List requested credential material: fields=%q", fields)
	}
}

// A Page can appear in /me/accounts and still carry no token. Folding those rows
// away made them indistinguishable from Pages the listing never mentioned.
func TestCredentialsKeepUsableAndTokenlessApart(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"pg1","access_token":"tok1","tasks":["MODERATE","MANAGE"]},
			{"id":"pg2","tasks":["ANALYZE"]},
			{"id":"pg3","access_token":"tok3"}
		]}`))
	})

	client := NewPagesClient(newTestTransport(t, server.URL))
	got, err := client.Credentials(context.Background())
	if err != nil {
		t.Fatalf("Credentials: %v", err)
	}

	if len(got.Usable) != 2 {
		t.Fatalf("Usable = %+v, want 2", got.Usable)
	}
	if len(got.Tokenless) != 1 || got.Tokenless[0] != "pg2" {
		t.Fatalf("Tokenless = %v, want [pg2]", got.Tokenless)
	}

	// Absent tasks and returned tasks are different answers, and pg3 has none.
	if got.Usable[1].PageID != "pg3" || got.Usable[1].Tasks != nil {
		t.Fatalf("pg3 Tasks = %v, want nil — Graph did not return the field", got.Usable[1].Tasks)
	}
	if got.Usable[0].Tasks == nil || len(*got.Usable[0].Tasks) != 2 {
		t.Fatalf("pg1 Tasks = %v, want two roles", got.Usable[0].Tasks)
	}
	if got.Usable[0].AccessToken != "tok1" {
		t.Fatalf("pg1 token not carried")
	}
}

// Each individual Graph call returns its replies chronologically. That is what
// makes the CONCATENATION in the sweep the thing that destroys ordering, rather
// than the calls themselves — so the parameter has to actually be sent.
func TestCommentReadsAskForChronologicalOrder(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	client := NewPagesClient(newTestTransport(t, server.URL))
	ctx := context.Background()
	if _, _, err := client.CommentsForPost(ctx, "pg1_p1", PageOptions{}); err != nil {
		t.Fatalf("CommentsForPost: %v", err)
	}
	if _, _, err := client.Replies(ctx, "c1", PageOptions{}); err != nil {
		t.Fatalf("Replies: %v", err)
	}

	forPost := server.queryFor(t, "/pg1_p1/comments")
	if forPost.Get("order") != "chronological" {
		t.Errorf("forPost order = %q, want chronological", forPost.Get("order"))
	}
	// Top-level only: replies are fetched deliberately, per comment, so the
	// sweep can tell which level it is reading.
	if forPost.Get("filter") != "toplevel" {
		t.Errorf("forPost filter = %q, want toplevel", forPost.Get("filter"))
	}

	replies := server.queryFor(t, "/c1/comments")
	if replies.Get("order") != "chronological" {
		t.Errorf("replies order = %q, want chronological", replies.Get("order"))
	}
	// The replies edge must NOT filter to toplevel — that is the whole point.
	if replies.Get("filter") == "toplevel" {
		t.Error("replies edge filtered to toplevel; it would return nothing")
	}
}

// A photo somebody else posted names somebody else's story, which would be
// counted as a post this token cannot see and would be right for the wrong
// reason.
func TestPhotosAskForUploadedOnly(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"ph1","page_story_id":"pg1_p1"}]}`))
	})

	client := NewPagesClient(newTestTransport(t, server.URL))
	photos, _, err := client.Photos(context.Background(), "pg1", PageOptions{})
	if err != nil {
		t.Fatalf("Photos: %v", err)
	}
	if len(photos) != 1 || photos[0].PostID == nil || *photos[0].PostID != "pg1_p1" {
		t.Fatalf("photos = %+v", photos)
	}
	if got := server.queryFor(t, "/pg1/photos").Get("type"); got != "uploaded" {
		t.Errorf("type = %q, want uploaded", got)
	}
}

// expires_at: 0 means NEVER, and 0 is the value this whole project turns on.
func TestIdentityKeepsAZeroExpiry(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"type":"SYSTEM_USER","app_id":"123","application":"Test App","expires_at":0,"is_valid":true}}`))
	})

	client := NewPagesClient(newTestTransport(t, server.URL))
	got, err := client.Identity(context.Background())
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if got.ExpiresAt == nil {
		t.Fatal("ExpiresAt = nil; Graph returned 0, which means never")
	}
	if *got.ExpiresAt != 0 {
		t.Fatalf("ExpiresAt = %d, want 0", *got.ExpiresAt)
	}
	if !got.Valid || got.Type == nil || *got.Type != "SYSTEM_USER" {
		t.Fatalf("identity = %+v", got)
	}

	// /debug_token inspects a token passed as a VALUE — the one deliberate
	// exception to this client's tokens-in-headers rule.
	if server.queryFor(t, "/debug_token").Get("input_token") != "test-token" {
		t.Error("debug_token was not given the token to inspect")
	}
}

// A token Graph will not even debug is not a crash, it is a `false`.
func TestIdentityReturnsInvalidRatherThanFailing(t *testing.T) {
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth access token","code":190}}`))
	})

	client := NewPagesClient(newTestTransport(t, server.URL))
	got, err := client.Identity(context.Background())
	if err != nil {
		t.Fatalf("Identity returned an error for an unusable token: %v", err)
	}
	if got.Valid {
		t.Fatal("Valid = true for a token Graph refused")
	}
}

// Hide and unhide are one endpoint and one boolean.
func TestSetHiddenSendsTheBoolean(t *testing.T) {
	var body string
	server := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	client := NewPagesClient(newTestTransport(t, server.URL))
	if err := client.SetHidden(context.Background(), "c1", true); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}
	if !strings.Contains(body, "is_hidden=true") {
		t.Fatalf("body = %q, want is_hidden=true", body)
	}
}
