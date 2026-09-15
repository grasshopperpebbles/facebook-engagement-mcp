"""Environment-derived configuration.

The Graph origin resolver here is a deliberate, line-for-line port of the
TypeScript one in ``@gpp/meta-client``. It is the one function in this package
that decides where a live access token may be sent, so the conformance suite
tests it directly and a port that quietly relaxes it fails rather than passes.
"""

from __future__ import annotations

import os
from urllib.parse import urlparse

GRAPH_API_VERSION = "v25.0"
GRAPH_API_HOST = "https://graph.facebook.com"

#: Set to the stub's origin. Meaningless unless the variable below is ``"true"``.
GRAPH_ORIGIN_ENV = "META_GRAPH_ORIGIN"
#: The opt-in. Without it the variable above is an error, never a default.
GRAPH_ORIGIN_ALLOW_ENV = "META_ALLOW_GRAPH_ORIGIN_OVERRIDE"


class ConfigError(RuntimeError):
    """Raised before any request is made, so a misconfiguration is loud."""


def resolve_graph_origin(env: dict[str, str] | None = None) -> str:
    """The origin every request goes to, and the ONLY origin ``paging.next`` may
    point at.

    Those are deliberately one value rather than two, which is the whole
    security content of this function. The access token is attached when
    following ``paging.next``, so a ``next`` pointing elsewhere is a
    credential-exfiltration shape whose blast radius is the token. Point this at
    a stub and it still refuses to follow ``next`` anywhere but that same stub.

    Two variables, and the second is not a formality. A value that silently
    applied would be a way to send a live token somewhere unexpected by setting
    one variable. A value silently *ignored* would be worse in the other
    direction: a test that believes it is talking to a stub while it is talking
    to Graph with a real token, with no error and no symptom. So an origin set
    without the opt-in raises.

    Not loopback-restricted, and deliberately so: inside a container
    ``127.0.0.1`` is the container's own loopback, so the stub is reached at a
    service name or ``host.docker.internal``. A loopback rule would hold only
    where it happened to be checked.
    """
    environ = os.environ if env is None else env
    origin = environ.get(GRAPH_ORIGIN_ENV)
    if not origin:
        return GRAPH_API_HOST

    if environ.get(GRAPH_ORIGIN_ALLOW_ENV) != "true":
        raise ConfigError(
            f"{GRAPH_ORIGIN_ENV} is set but {GRAPH_ORIGIN_ALLOW_ENV} is not \"true\", so it "
            "was refused rather than ignored. Ignoring it would send this token to the real "
            f"Graph API while you believed it was going to {origin}. Set "
            f"{GRAPH_ORIGIN_ALLOW_ENV}=true if that is what you meant; this override exists "
            "for the conformance suite."
        )

    parsed = urlparse(origin)
    # Scheme first, THEN host. Ordering matters for the diagnosis rather than
    # the verdict: `urlparse("file:///etc/passwd")` yields a scheme and an empty
    # netloc, so a host check placed first reports "not a valid URL" for a URL
    # that parses perfectly well and is simply the wrong protocol. TypeScript's
    # `new URL` accepts it and reaches the protocol check, so checking host
    # first made the two implementations refuse the same input with different
    # explanations. Caught by a unit test, not by the conformance suite, which
    # only sees that both refused.
    if not parsed.scheme:
        raise ConfigError(f"{GRAPH_ORIGIN_ENV} is not a valid URL: {origin}")
    if parsed.scheme not in ("http", "https"):
        raise ConfigError(f"{GRAPH_ORIGIN_ENV} must be http or https, got {parsed.scheme}:")
    if not parsed.netloc:
        raise ConfigError(f"{GRAPH_ORIGIN_ENV} is not a valid URL: {origin}")

    # The origin alone. A path would be dropped when building request URLs and
    # kept when comparing `paging.next`, so the two jobs would stop agreeing —
    # which is the one thing this function exists to prevent.
    return f"{parsed.scheme}://{parsed.netloc}"
