package graph

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestTransport(t *testing.T, origin string) *Transport {
	t.Helper()
	tr, err := NewTransport(TransportOptions{
		AccessToken: "test-token",
		GraphOrigin: origin,
		Sleep:       func(time.Duration) {}, // backoff costs no wall-clock
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return tr
}

// The token goes in a header, never a query string: query strings are recorded
// by proxies, CDNs and server logs, and headers are not.
func TestTokenTravelsInTheAuthorizationHeaderOnly(t *testing.T) {
	var gotAuth, gotQuery, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	var out map[string]any
	if err := tr.Get(context.Background(), "/me/accounts", nil, &out); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if strings.Contains(gotQuery, "test-token") || strings.Contains(gotQuery, "access_token") {
		t.Errorf("token leaked into the query string: %q", gotQuery)
	}
	if want := "/" + GraphAPIVersion + "/me/accounts"; gotPath != want {
		t.Errorf("path = %q, want %q — the version pin is applied by the URL builder", gotPath, want)
	}
}

// paging.next is a URL taken out of a response body and then followed WITH the
// access token attached, so whoever that URL names receives the credential.
func TestPagingNextToAnotherOriginIsRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"1"}],"paging":{"next":"https://evil.example.com/v25.0/x?after=abc"}}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	_, _, err := tr.GetPage(context.Background(), "/pg1/feed", PageOptions{})
	if err == nil {
		t.Fatal("expected a refusal, got none — the token would have gone to evil.example.com")
	}
	if !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("error does not name the cause: %v", err)
	}
}

// Live on 2026-09-14 a request sent to v25.0 was answered with a paging.next on
// v26.0. Followed as given, the pin stops applying at page 2 and one result set
// is assembled from two API versions.
func TestPagingNextIsRewrittenBackToThePinnedVersion(t *testing.T) {
	var secondPath string
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page++
		if page == 1 {
			next := "http://" + r.Host + "/v26.0/pg1/feed?after=cursor-abc&limit=100"
			_, _ = w.Write([]byte(`{"data":[{"id":"1"}],"paging":{"next":"` + next + `"}}`))
			return
		}
		secondPath = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[{"id":"2"}]}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	items, _, err := tr.GetPage(context.Background(), "/pg1/feed", PageOptions{})
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if !strings.HasPrefix(secondPath, "/"+GraphAPIVersion+"/") {
		t.Errorf("second page went to %q; the version pin stopped applying at page 2", secondPath)
	}
	// The cursor is what makes the URL worth following and must survive exactly.
	if !strings.Contains(secondPath, "after=cursor-abc") {
		t.Errorf("cursor was lost rewriting the version: %q", secondPath)
	}
}

// A retried reply is a double post.
func TestWritesAreNeverRetried(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream","code":1}}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	var out map[string]any
	err := tr.Post(context.Background(), "/c1/comments", map[string]string{"message": "hi"}, &out)
	if err == nil {
		t.Fatal("expected an error")
	}
	if attempts != 1 {
		t.Fatalf("POST was attempted %d times; writes must never retry", attempts)
	}
}

// Comment text is user data and must not reach proxy or CDN logs, so POST
// parameters go in the body and never the query string.
func TestPostSendsParametersInTheBody(t *testing.T) {
	var gotQuery, gotBody, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotContentType = r.Header.Get("Content-Type")
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1_r1"}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	var out map[string]any
	if err := tr.Post(context.Background(), "/c1/comments", map[string]string{"message": "secret text"}, &out); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if strings.Contains(gotQuery, "secret") {
		t.Errorf("comment text leaked into the query string: %q", gotQuery)
	}
	if !strings.Contains(gotBody, "secret+text") && !strings.Contains(gotBody, "secret%20text") {
		t.Errorf("body did not carry the message: %q", gotBody)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
}

// A read retries a transient fault, and stops at MaxAttempts.
func TestReadsRetryTransientFaults(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"1"}]}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	items, _, err := tr.GetPage(context.Background(), "/pg1/feed", PageOptions{})
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
}

// An auth error is not retryable: trying again with the same bad token wastes
// the rate limit and delays the real diagnosis.
func TestAuthErrorsAreNotRetried(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth access token","code":190}}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	if _, _, err := tr.GetPage(context.Background(), "/pg1/feed", PageOptions{}); err == nil {
		t.Fatal("expected an error")
	}
	if attempts != 1 {
		t.Fatalf("an auth error was retried %d times", attempts)
	}
}

// A non-JSON body (an HTML error page, an empty 502) must still produce a usable
// error rather than a parse failure.
func TestNonJSONErrorBodyStillYieldsAUsableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`<html><body>Bad Gateway</body></html>`))
	}))
	defer server.Close()

	tr, err := NewTransport(TransportOptions{
		AccessToken: "t", GraphOrigin: server.URL, MaxAttempts: 1, Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}

	var out map[string]any
	err = tr.Get(context.Background(), "/me", nil, &out)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("error does not carry the status: %v", err)
	}
}

// GetAll errors rather than returning a silently short list — the defect this
// client exists to avoid, where 200 campaigns became 25 with nothing to notice.
func TestGetAllRefusesToReturnASilentlyShortList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next := "http://" + r.Host + "/v25.0/acct/campaigns?after=more"
		_, _ = w.Write([]byte(`{"data":[{"id":"1"}],"paging":{"next":"` + next + `"}}`))
	}))
	defer server.Close()

	tr, err := NewTransport(TransportOptions{
		AccessToken: "t", GraphOrigin: server.URL, MaxPages: 1, Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if _, err := tr.GetAll(context.Background(), "/acct/campaigns", PageOptions{}); err == nil {
		t.Fatal("expected GetAll to refuse a truncated result")
	}
}

// Never ask Graph for more rows than the caller wants: every avoided request is
// one that does not count against the rate limit.
func TestGetPageNeverAsksForMoreThanMaxItems(t *testing.T) {
	var gotLimit string
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		next := "http://" + r.Host + "/v25.0/pg1/feed?after=more"
		_, _ = w.Write([]byte(`{"data":[{"id":"1"},{"id":"2"},{"id":"3"}],"paging":{"next":"` + next + `"}}`))
	}))
	defer server.Close()

	tr := newTestTransport(t, server.URL)
	items, truncated, err := tr.GetPage(context.Background(), "/pg1/feed", PageOptions{MaxItems: 2})
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if gotLimit != "2" {
		t.Errorf("limit = %q, want 2", gotLimit)
	}
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}
	if !truncated {
		t.Error("truncated = false; more results existed")
	}
	if requests != 1 {
		t.Errorf("made %d requests; it should stop before fetching a page it would discard", requests)
	}
}

// A token is required, and the origin must be resolved by the caller so the URL
// builder and the paging.next pin cannot disagree.
func TestNewTransportRefusesIncompleteOptions(t *testing.T) {
	if _, err := NewTransport(TransportOptions{GraphOrigin: "https://graph.facebook.com"}); err == nil {
		t.Error("expected an error with no token")
	}
	if _, err := NewTransport(TransportOptions{AccessToken: "t"}); err == nil {
		t.Error("expected an error with no origin")
	}
}
