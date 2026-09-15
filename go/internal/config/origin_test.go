package config

import (
	"strings"
	"testing"
)

func env(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func TestResolveGraphOrigin(t *testing.T) {
	const allow = GraphOriginAllowEnv
	const origin = GraphOriginEnv

	tests := []struct {
		name    string
		vars    map[string]string
		want    string
		wantErr string // substring; empty means no error expected
	}{
		{
			name: "unset falls back to the real Graph host",
			vars: map[string]string{},
			want: GraphAPIHost,
		},
		{
			name: "empty string is treated as unset",
			vars: map[string]string{origin: ""},
			want: GraphAPIHost,
		},
		{
			name:    "set without the opt-in is refused, not ignored",
			vars:    map[string]string{origin: "http://127.0.0.1:8080"},
			wantErr: "refused rather than ignored",
		},
		{
			name:    "opt-in must be exactly true",
			vars:    map[string]string{origin: "http://127.0.0.1:8080", allow: "yes"},
			wantErr: "refused rather than ignored",
		},
		{
			name: "http loopback is allowed",
			vars: map[string]string{origin: "http://127.0.0.1:8080", allow: "true"},
			want: "http://127.0.0.1:8080",
		},
		{
			// Deliberately not loopback-restricted: inside a container 127.0.0.1
			// is the container's own loopback, so the conformance stub is reached
			// at host.docker.internal.
			name: "a non-loopback host is allowed, deliberately",
			vars: map[string]string{origin: "http://host.docker.internal:8080", allow: "true"},
			want: "http://host.docker.internal:8080",
		},
		{
			name: "a path component is stripped",
			vars: map[string]string{origin: "http://127.0.0.1:8080/v25.0/x", allow: "true"},
			want: "http://127.0.0.1:8080",
		},
		{
			// Parses perfectly well and is simply the wrong protocol. A host check
			// placed first would call this "not a valid URL".
			name:    "a file URL names the protocol, not the URL",
			vars:    map[string]string{origin: "file:///etc/passwd", allow: "true"},
			wantErr: "must be http or https",
		},
		{
			name:    "ftp names the protocol",
			vars:    map[string]string{origin: "ftp://example.com", allow: "true"},
			wantErr: "must be http or https",
		},
		{
			// net/url accepts this with an empty Scheme and no error, where
			// JavaScript's URL and Python's urlparse both refuse it.
			name:    "a bare host is not a valid URL",
			vars:    map[string]string{origin: "example.com", allow: "true"},
			wantErr: "is not a valid URL",
		},
		{
			name:    "a scheme with no host is not a valid URL",
			vars:    map[string]string{origin: "https://", allow: "true"},
			wantErr: "is not a valid URL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveGraphOrigin(env(tc.vars))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error containing %q, got origin %q", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The refusal must say what would have happened, not merely that it refused:
// ignoring the variable would send a live token to the real Graph API while the
// caller believed it was going to the stub.
func TestRefusalExplainsTheConsequence(t *testing.T) {
	_, err := ResolveGraphOrigin(env(map[string]string{GraphOriginEnv: "http://127.0.0.1:8080"}))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, want := range []string{GraphOriginEnv, GraphOriginAllowEnv, "http://127.0.0.1:8080", "real Graph API"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}
}
