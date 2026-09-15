"""The Python half of the Graph origin contract.

The conformance suite drives the wire; this drives the function directly,
because it is the one place in this package that decides where a live access
token may be sent and the failure modes are cheaper to enumerate here.
"""

from __future__ import annotations

import pytest

from facebook_engagement_mcp.config import (
    GRAPH_API_HOST,
    GRAPH_ORIGIN_ALLOW_ENV,
    GRAPH_ORIGIN_ENV,
    ConfigError,
    resolve_graph_origin,
)


def allowed(origin: str) -> dict[str, str]:
    return {GRAPH_ORIGIN_ENV: origin, GRAPH_ORIGIN_ALLOW_ENV: "true"}


def test_defaults_to_the_real_graph_host():
    assert resolve_graph_origin({}) == GRAPH_API_HOST


def test_empty_override_is_ignored_rather_than_parsed():
    assert resolve_graph_origin({GRAPH_ORIGIN_ENV: ""}) == GRAPH_API_HOST


def test_returns_the_override_when_explicitly_allowed():
    assert resolve_graph_origin(allowed("http://stub.local:8080")) == "http://stub.local:8080"


def test_refuses_rather_than_ignores_without_the_opt_in():
    """The reason the second variable exists, and why this raises.

    Silently ignoring the override leaves a test believing it is talking to a
    stub while it is talking to Graph with a real token — no error, no symptom,
    which is the worst shape a credential mistake can take.
    """
    with pytest.raises(ConfigError, match="refused rather than ignored"):
        resolve_graph_origin({GRAPH_ORIGIN_ENV: "http://stub.local:8080"})


@pytest.mark.parametrize("value", ["1", "yes", "TRUE", "false", ""])
def test_opt_in_must_be_exactly_true(value: str):
    with pytest.raises(ConfigError, match=GRAPH_ORIGIN_ALLOW_ENV):
        resolve_graph_origin({GRAPH_ORIGIN_ENV: "http://stub.local:8080",
                              GRAPH_ORIGIN_ALLOW_ENV: value})


def test_refuses_a_non_url():
    with pytest.raises(ConfigError, match="not a valid URL"):
        resolve_graph_origin(allowed("not a url"))


@pytest.mark.parametrize("origin", ["file:///etc/passwd", "ftp://example.com"])
def test_refuses_a_non_http_protocol(origin: str):
    with pytest.raises(ConfigError, match="http or https"):
        resolve_graph_origin(allowed(origin))


def test_reduces_a_url_with_a_path_to_its_origin():
    """A path would be dropped when building requests and kept when comparing
    `paging.next`, so the two jobs this value serves would stop agreeing."""
    assert resolve_graph_origin(allowed("http://stub.local:8080/graph/v1")) == "http://stub.local:8080"


@pytest.mark.parametrize("origin", ["http://host.docker.internal:8080", "http://graph-stub:8080"])
def test_accepts_non_loopback_because_containers_need_it(origin: str):
    assert resolve_graph_origin(allowed(origin)) == origin
