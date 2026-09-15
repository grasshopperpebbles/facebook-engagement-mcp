// Package config resolves the environment-derived settings this server needs
// before it makes any request.
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// GraphAPIHost is where a live access token goes unless an override is both set
// and explicitly allowed.
const GraphAPIHost = "https://graph.facebook.com"

// GraphOriginEnv points the client at the conformance stub. Meaningless unless
// GraphOriginAllowEnv is exactly "true".
const GraphOriginEnv = "META_GRAPH_ORIGIN"

// GraphOriginAllowEnv is the opt-in. Without it the variable above is an error,
// never a default.
const GraphOriginAllowEnv = "META_ALLOW_GRAPH_ORIGIN_OVERRIDE"

// ResolveGraphOrigin returns the origin every request goes to, and the ONLY
// origin paging.next may point at.
//
// Those are deliberately one value rather than two, which is the whole security
// content of this function. The access token is attached when following
// paging.next, so a next pointing elsewhere is a credential-exfiltration shape
// whose blast radius is the token. Point this at a stub and it still refuses to
// follow next anywhere but that same stub.
//
// Two variables, and the second is not a formality. A value that silently
// applied would be a way to send a live token somewhere unexpected by setting
// one variable. A value silently IGNORED would be worse in the other direction:
// a test that believes it is talking to a stub while it is talking to Graph with
// a real token, with no error and no symptom. So an origin set without the
// opt-in returns an error.
//
// Not loopback-restricted, and deliberately so: inside a container 127.0.0.1 is
// the container's own loopback, so the stub is reached at a service name or
// host.docker.internal. A loopback rule would hold only where it was checked.
//
// env is a lookup function rather than a map so tests touch no process state and
// callers pass os.Getenv.
func ResolveGraphOrigin(env func(string) string) (string, error) {
	origin := env(GraphOriginEnv)
	if origin == "" {
		return GraphAPIHost, nil
	}

	if env(GraphOriginAllowEnv) != "true" {
		return "", fmt.Errorf(
			"%s is set but %s is not \"true\", so it was refused rather than ignored. "+
				"Ignoring it would send this token to the real Graph API while you believed "+
				"it was going to %s. Set %s=true if that is what you meant; this override "+
				"exists for the conformance suite.",
			GraphOriginEnv, GraphOriginAllowEnv, origin, GraphOriginAllowEnv)
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid URL: %s", GraphOriginEnv, origin)
	}

	// Scheme BEFORE host, and the order is load-bearing for the diagnosis rather
	// than the verdict. "file:///etc/passwd" parses perfectly well and is simply
	// the wrong protocol; a host check placed first reports "not a valid URL"
	// for it. Python checked host before scheme once and refused the same input
	// with a different explanation than TypeScript — caught by a unit test, and
	// invisible to the conformance suite, which only sees that both refused.
	if parsed.Scheme == "" {
		// net/url is more permissive than JavaScript's URL or Python's urlparse:
		// it accepts a bare "example.com" with no error and an empty Scheme, so
		// this check is Go-specific and has no counterpart in the other two.
		return "", fmt.Errorf("%s is not a valid URL: %s", GraphOriginEnv, origin)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s must be http or https, got %s:", GraphOriginEnv, scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%s is not a valid URL: %s", GraphOriginEnv, origin)
	}

	// The origin alone. A path would be dropped when building request URLs and
	// kept when comparing paging.next, so the two jobs would stop agreeing —
	// which is the one thing this function exists to prevent.
	return scheme + "://" + parsed.Host, nil
}
