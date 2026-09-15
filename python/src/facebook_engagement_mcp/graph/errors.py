"""Graph API errors, kept whole.

The methodological finding behind this module (T-26, 2026-09-08): we once kept
Meta's ``message`` and discarded ``error_subcode``, ``error_user_msg`` and
``fbtrace_id``, and a record that thin can only be re-read. Re-reading it is
exactly how "this needs App Review" got asserted about an error that named a
permission removed in 2018 — a message literally true and entirely misleading,
because the fault was the identity, not the permission.
"""

from __future__ import annotations

from typing import Any


class MetaApiError(RuntimeError):
    def __init__(
        self,
        message: str,
        *,
        status: int = 0,
        code: int | None = None,
        subcode: int | None = None,
        user_message: str | None = None,
        trace_id: str | None = None,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.code = code
        self.subcode = subcode
        self.user_message = user_message
        self.trace_id = trace_id

    @property
    def is_auth_error(self) -> bool:
        """An expired, revoked or malformed token, as opposed to any other refusal."""
        return self.code in (102, 190) or self.status in (401,)

    @classmethod
    def from_body(cls, status: int, body: Any) -> MetaApiError:
        error = (body or {}).get("error", {}) if isinstance(body, dict) else {}
        return cls(
            error.get("message") or f"Graph API request failed with status {status}",
            status=status,
            code=error.get("code"),
            subcode=error.get("error_subcode"),
            user_message=error.get("error_user_msg"),
            trace_id=error.get("fbtrace_id"),
        )
