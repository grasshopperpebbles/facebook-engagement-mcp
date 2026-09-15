package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TokenResolver returns the token for one request. Page-scoped reads need a
// different token per Page, which a value fixed at construction cannot give.
type TokenResolver func(ctx context.Context) (string, error)

// TransportOptions configures a Transport.
//
// GraphOrigin is required rather than resolved here, and that is deliberate: the
// caller resolves it ONCE, in main, and both the URL builder and the paging.next
// pin below read that one value. Two independently-resolved values could drift
// apart, which is the one thing the origin rule exists to prevent.
type TransportOptions struct {
	// AccessToken is a fixed token. Ignored when TokenFunc is set.
	AccessToken string
	// TokenFunc resolves a token per request, for Page-scoped reads.
	TokenFunc TokenResolver

	GraphOrigin string

	HTTPClient  *http.Client
	Timeout     time.Duration
	MaxPages    int
	MaxAttempts int
	RetryBase   time.Duration
	// Sleep is injectable so backoff costs no wall-clock time in tests.
	Sleep func(time.Duration)
}

// PageOptions bounds one paginated read.
type PageOptions struct {
	// MaxItems stops after this many items. Zero means every page up to MaxPages.
	MaxItems int
	PageSize int
	Params   map[string]string
}

// Transport is the only place a live access token is sent anywhere.
type Transport struct {
	token       string
	tokenFunc   TokenResolver
	origin      string
	originURL   *url.URL
	client      *http.Client
	timeout     time.Duration
	maxPages    int
	maxAttempts int
	retryBase   time.Duration
	sleep       func(time.Duration)
}

type graphListResponse struct {
	Data   []json.RawMessage `json:"data"`
	Paging *struct {
		Next string `json:"next"`
	} `json:"paging"`
}

// versionPrefix matches the leading /vNN.N/ segment of a Graph URL path.
var versionPrefix = regexp.MustCompile(`^/v\d+\.\d+/`)

// NewTransport validates the options and returns a ready Transport.
func NewTransport(opts TransportOptions) (*Transport, error) {
	if opts.AccessToken == "" && opts.TokenFunc == nil {
		return nil, fmt.Errorf("a Meta access token is required")
	}
	if opts.GraphOrigin == "" {
		return nil, fmt.Errorf("a Graph origin is required; resolve it once with config.ResolveGraphOrigin")
	}
	parsed, err := url.Parse(opts.GraphOrigin)
	if err != nil {
		return nil, fmt.Errorf("graph origin is not a valid URL: %s", opts.GraphOrigin)
	}

	t := &Transport{
		token:       opts.AccessToken,
		tokenFunc:   opts.TokenFunc,
		origin:      opts.GraphOrigin,
		originURL:   parsed,
		client:      opts.HTTPClient,
		timeout:     opts.Timeout,
		maxPages:    opts.MaxPages,
		maxAttempts: opts.MaxAttempts,
		retryBase:   opts.RetryBase,
		sleep:       opts.Sleep,
	}
	if t.client == nil {
		t.client = &http.Client{}
	}
	if t.timeout == 0 {
		t.timeout = DefaultTimeout
	}
	if t.maxPages == 0 {
		t.maxPages = DefaultMaxPages
	}
	if t.maxAttempts == 0 {
		t.maxAttempts = DefaultRetryAttempts
	}
	if t.retryBase == 0 {
		t.retryBase = DefaultRetryBase
	}
	if t.sleep == nil {
		t.sleep = time.Sleep
	}
	return t, nil
}

// buildURL applies the version pin. It is the only place a request path is
// turned into a URL, so the pin cannot be skipped by a caller.
func (t *Transport) buildURL(path string, params map[string]string) string {
	u := *t.originURL
	u.Path = "/" + GraphAPIVersion + path

	if len(params) > 0 {
		query := url.Values{}
		for key, value := range params {
			if value != "" {
				query.Set(key, value)
			}
		}
		u.RawQuery = query.Encode()
	}
	return u.String()
}

// authHeaders puts the token in a Bearer header rather than an access_token
// query parameter. Query strings are recorded by proxies, CDNs and server logs;
// headers are not.
func (t *Transport) authHeaders(ctx context.Context) (map[string]string, error) {
	token := t.token
	if t.tokenFunc != nil {
		resolved, err := t.tokenFunc(ctx)
		if err != nil {
			return nil, err
		}
		token = resolved
	}
	if token == "" {
		return nil, fmt.Errorf("a Meta access token is required")
	}
	return map[string]string{
		"Authorization": "Bearer " + token,
		"Accept":        "application/json",
		"User-Agent":    "gpp-meta-client",
	}, nil
}

// toError maps a non-2xx response to a MetaAPIError, without the token.
func toError(status int, body []byte) *MetaAPIError {
	err := &MetaAPIError{
		Message: fmt.Sprintf("Meta API request failed with status %d", status),
		Status:  &status,
	}

	// A non-JSON body (an HTML error page, an empty 502) must still produce a
	// usable error rather than a parse failure.
	var parsed struct {
		Error *graphErrorBody `json:"error"`
	}
	if jsonErr := json.Unmarshal(body, &parsed); jsonErr != nil || parsed.Error == nil {
		return err
	}

	if parsed.Error.Message != nil {
		err.Message = *parsed.Error.Message
	}
	err.Code = parsed.Error.Code
	err.Subcode = parsed.Error.ErrorSubcode
	err.FbtraceID = parsed.Error.FbtraceID
	err.Type = parsed.Error.Type
	return err
}

// attempt makes exactly one request and returns its raw body.
func (t *Transport) attempt(ctx context.Context, method, rawURL string, body io.Reader, extraHeaders map[string]string) ([]byte, error) {
	headers, err := t.authHeaders(ctx)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, body)
	if err != nil {
		return nil, &MetaAPIError{Message: fmt.Sprintf("Request to Meta failed: %v", err), IsNetworkError: true}
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	res, err := t.client.Do(req)
	if err != nil {
		return nil, &MetaAPIError{Message: fmt.Sprintf("Request to Meta failed: %v", err), IsNetworkError: true}
	}
	defer func() { _ = res.Body.Close() }()

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &MetaAPIError{Message: fmt.Sprintf("Request to Meta failed: %v", err), IsNetworkError: true}
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, toError(res.StatusCode, payload)
	}
	return payload, nil
}

// request retries a READ on throttling and transient upstream faults.
//
// Writes must never be routed through here — a retried reply is a double post.
// Post is a separate code path rather than a flag, so no later edit can route a
// write into this loop by changing an argument.
func (t *Transport) request(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attemptNo := 0; attemptNo < t.maxAttempts; attemptNo++ {
		payload, err := t.attempt(ctx, http.MethodGet, rawURL, nil, nil)
		if err == nil {
			return payload, nil
		}
		lastErr = err

		metaErr, ok := err.(*MetaAPIError)
		if !ok || !metaErr.IsRetryable() || attemptNo == t.maxAttempts-1 {
			return nil, err
		}
		t.sleep(t.retryBase * time.Duration(1<<attemptNo))
	}
	return nil, lastErr
}

// nextPageURL returns the next page's URL, or "" when there is none.
//
// paging.next is a URL taken out of a response body and then followed with the
// access token attached, so whoever that URL names receives the credential. Meta
// is trusted and a comment author cannot influence it, which is why this has
// never been a live hole — but the blast radius if that stopped being true is
// the token, and the check is cheaper than the argument about whether it is
// needed.
//
// It also carries the API version back to the pinned one. Live on 2026-09-14 a
// request sent to v25.0 was answered with a paging.next on v26.0: Meta builds
// that URL and does not build it on the version you asked for. Followed as
// given, the pin stops applying at page 2 and one result set is assembled from
// two API versions — precisely what pinning exists to prevent. Only the version
// segment is rewritten; the cursor is what makes the URL worth following and is
// preserved exactly.
func (t *Transport) nextPageURL(body graphListResponse) (string, error) {
	if body.Paging == nil || body.Paging.Next == "" {
		return "", nil
	}

	next, err := url.Parse(body.Paging.Next)
	if err != nil {
		return "", &MetaAPIError{Message: fmt.Sprintf("paging.next is not a valid URL: %v", err)}
	}

	if next.Scheme+"://"+next.Host != t.originURL.Scheme+"://"+t.originURL.Host {
		return "", &MetaAPIError{
			Message: fmt.Sprintf(
				"Refusing to follow paging.next to a different origin (%s://%s). "+
					"The access token is only ever sent to %s.",
				next.Scheme, next.Host, t.origin),
		}
	}

	next.Path = versionPrefix.ReplaceAllString(next.Path, "/"+GraphAPIVersion+"/")
	return next.String(), nil
}

// Get reads one resource and decodes it into out.
func (t *Transport) Get(ctx context.Context, path string, params map[string]string, out any) error {
	payload, err := t.request(ctx, t.buildURL(path, params))
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return &MetaAPIError{Message: fmt.Sprintf("Meta returned a body this client could not parse: %v", err)}
	}
	return nil
}

// GetPage reads up to MaxItems rows and reports whether more exist.
func (t *Transport) GetPage(ctx context.Context, path string, opts PageOptions) ([]json.RawMessage, bool, error) {
	requested := opts.PageSize
	if requested == 0 {
		requested = DefaultPageSize
	}
	// Never ask Graph for more rows than the caller wants, and never exceed what
	// the edge accepts.
	limit := requested
	if opts.MaxItems > 0 && opts.MaxItems < limit {
		limit = opts.MaxItems
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	params := map[string]string{}
	for key, value := range opts.Params {
		params[key] = value
	}
	params["limit"] = strconv.Itoa(limit)

	items := []json.RawMessage{}
	truncated := false
	rawURL := t.buildURL(path, params)

	for page := 0; page < t.maxPages && rawURL != ""; page++ {
		payload, err := t.request(ctx, rawURL)
		if err != nil {
			return nil, false, err
		}

		var body graphListResponse
		if err := json.Unmarshal(payload, &body); err != nil {
			return nil, false, &MetaAPIError{Message: fmt.Sprintf("Meta returned a list body this client could not parse: %v", err)}
		}

		for _, item := range body.Data {
			if opts.MaxItems > 0 && len(items) >= opts.MaxItems {
				return items, true, nil
			}
			items = append(items, item)
		}
		truncated = body.Paging != nil && body.Paging.Next != ""

		// Stop before fetching a page we would immediately discard. Every avoided
		// request is one that does not count against the rate limit.
		if opts.MaxItems > 0 && len(items) >= opts.MaxItems {
			return items, truncated, nil
		}

		rawURL, err = t.nextPageURL(body)
		if err != nil {
			return nil, false, err
		}
	}

	return items, truncated, nil
}

// GetAll returns every matching row, or an error.
//
// Returning a silently short list is the defect this client exists to avoid —
// the Python reference dropped paging.next and turned 200 campaigns into 25 with
// nothing to notice. Callers who can accept a partial result use GetPage, which
// reports truncated instead of failing.
func (t *Transport) GetAll(ctx context.Context, path string, opts PageOptions) ([]json.RawMessage, error) {
	items, truncated, err := t.GetPage(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, &MetaAPIError{
			Message: fmt.Sprintf(
				"More results exist for %s than were retrieved (%d so far). "+
					"Use GetPage to accept a partial result, or raise MaxPages / MaxItems.",
				path, len(items)),
		}
	}
	return items, nil
}

// Post is a form-encoded write. Parameters go in the body, never the query
// string: comment text is user data and must not reach proxy or CDN logs.
//
// It never retries, and it is deliberately a separate code path from request().
func (t *Transport) Post(ctx context.Context, path string, params map[string]string, out any) error {
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}

	payload, err := t.attempt(
		ctx,
		http.MethodPost,
		t.buildURL(path, nil),
		strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return &MetaAPIError{Message: fmt.Sprintf("Meta returned a body this client could not parse: %v", err)}
	}
	return nil
}
