"""Console-script entry point.

The suite drives the INSTALLED package through this, never the source tree. The
`bin` entrypoint bug this project shipped did not exist in the source — it
existed in the packaging, and twelve reviews of the source approved it.
"""

from __future__ import annotations

import os
import sys

from .server import build


def main() -> None:
    access_token = os.environ.get("META_ACCESS_TOKEN")
    if not access_token:
        print("META_ACCESS_TOKEN is required. See the README for the scopes it needs.", file=sys.stderr)
        raise SystemExit(1)

    print(
        "Write tools are not implemented in the Python build; this server is read-only.",
        file=sys.stderr,
    )
    build(access_token).run(transport="stdio")


if __name__ == "__main__":
    main()
