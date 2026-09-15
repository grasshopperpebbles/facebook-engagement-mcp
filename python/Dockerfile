# The conformance runner, CI and self-hosting. NOT how anyone installs this —
# that is `pip install facebook-engagement-mcp`. See README.md.
#
# The build stage produces a wheel and the runtime stage installs it. That is
# the point of the two stages, and it is not cosmetic: this project shipped a
# `bin` entrypoint bug that did not exist in the source at all — it existed in
# the packaging, and twelve reviews of the source approved the line. A container
# that does `COPY . /app` and runs the module in place reproduces exactly the
# review that missed it, and the conformance suite would be testing code no user
# ever receives.
FROM python:3.12-slim AS build
WORKDIR /build
RUN pip install --no-cache-dir hatchling
COPY pyproject.toml README.md ./
COPY src ./src
RUN pip wheel --no-cache-dir --no-deps -w /wheels .

FROM python:3.12-slim
# Not root: this process reads a live access token from its environment.
RUN useradd --create-home --uid 10001 app
COPY --from=build /wheels /wheels
RUN pip install --no-cache-dir /wheels/*.whl && rm -rf /wheels
USER app

# No ENTRYPOINT shell wrapper: MCP is JSON-RPC on stdin and stdout, and anything
# that writes to stdout corrupts the framing. The console script is invoked
# directly, by the name the wheel installed it under — so the image exercises
# the same entrypoint resolution a `pip install` gives a user.
ENTRYPOINT ["facebook-engagement-mcp"]
