package graph

import (
	"errors"
	"testing"
)

func TestMetaAPIErrorClassification(t *testing.T) {
	tests := []struct {
		name          string
		err           *MetaAPIError
		wantThrottled bool
		wantAuth      bool
		wantRetryable bool
	}{
		{
			name:          "code 4 is throttling and therefore retryable",
			err:           &MetaAPIError{Message: "limit", Code: intp(4)},
			wantThrottled: true,
			wantRetryable: true,
		},
		{"code 17 is throttling", &MetaAPIError{Message: "limit", Code: intp(17)}, true, false, true},
		{"code 32 is throttling", &MetaAPIError{Message: "limit", Code: intp(32)}, true, false, true},
		{"code 613 is throttling", &MetaAPIError{Message: "limit", Code: intp(613)}, true, false, true},
		{
			name:          "code 190 is an auth error and is NOT retryable",
			err:           &MetaAPIError{Message: "expired", Code: intp(190)},
			wantAuth:      true,
			wantRetryable: false,
		},
		{
			name:          "status 401 is an auth error",
			err:           &MetaAPIError{Message: "unauthorized", Status: intp(401)},
			wantAuth:      true,
			wantRetryable: false,
		},
		{
			name:          "a 500 is retryable",
			err:           &MetaAPIError{Message: "upstream", Status: intp(500)},
			wantRetryable: true,
		},
		{
			name:          "a 400 is not retryable",
			err:           &MetaAPIError{Message: "bad request", Status: intp(400)},
			wantRetryable: false,
		},
		{
			name:          "a network error is retryable",
			err:           &MetaAPIError{Message: "dial failed", IsNetworkError: true},
			wantRetryable: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.IsThrottled(); got != tc.wantThrottled {
				t.Errorf("IsThrottled = %v, want %v", got, tc.wantThrottled)
			}
			if got := tc.err.IsAuthError(); got != tc.wantAuth {
				t.Errorf("IsAuthError = %v, want %v", got, tc.wantAuth)
			}
			if got := tc.err.IsRetryable(); got != tc.wantRetryable {
				t.Errorf("IsRetryable = %v, want %v", got, tc.wantRetryable)
			}
		})
	}
}

// Absent and zero are different facts. A Code of 0 is not a throttle code, and
// "Graph said nothing" must not be read as "Graph said zero".
func TestAbsentCodeIsNotZero(t *testing.T) {
	absent := &MetaAPIError{Message: "something failed"}
	if absent.IsThrottled() {
		t.Error("an error with no code was classified as throttled")
	}
	if absent.IsAuthError() {
		t.Error("an error with no code or status was classified as an auth error")
	}
	if absent.IsRetryable() {
		t.Error("an error with no code, status or network flag was classified as retryable")
	}
}

// The whole reason this is a typed error rather than a string: callers ask what
// KIND of failure this was without matching on message text.
func TestMetaAPIErrorIsReachableByErrorsAs(t *testing.T) {
	var wrapped error = &MetaAPIError{Message: "expired", Code: intp(190)}
	wrapped = errors.Join(errors.New("while reading comments"), wrapped)

	var target *MetaAPIError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As could not reach the MetaAPIError through a wrap")
	}
	if !target.IsAuthError() {
		t.Fatal("recovered error lost its classification")
	}
}

// The token must never reach a message, because errors reach logs and model
// context routinely.
func TestErrorMessageCarriesNoCredential(t *testing.T) {
	err := &MetaAPIError{Message: "Invalid OAuth access token", Code: intp(190)}
	if got := err.Error(); got != "Invalid OAuth access token (code 190)" {
		t.Fatalf("Error() = %q", got)
	}
}

func intp(v int) *int { return &v }
