# The conformance runner, CI and self-hosting. NOT how anyone installs this —
# that is `go install`. See README.md.
#
# The build stage compiles a static binary and the runtime stage runs it. That
# is the point of the two stages and it is not cosmetic: this project shipped a
# `bin` entrypoint bug that did not exist in the source at all — it existed in
# the packaging, and twelve reviews of the source approved the line. An image
# that runs `go run` over the source reproduces exactly the review that missed
# it, and the conformance suite would be testing code no user ever receives.
FROM golang:1.27 AS build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/facebook-engagement-mcp ./cmd/facebook-engagement-mcp

# Distroless static rather than scratch: the binary makes HTTPS calls to
# graph.facebook.com and needs a CA bundle. `nonroot` because this process reads
# a live access token from its environment.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/facebook-engagement-mcp /facebook-engagement-mcp

# No shell wrapper: MCP is JSON-RPC on stdin and stdout, and anything that
# writes to stdout corrupts the framing.
ENTRYPOINT ["/facebook-engagement-mcp"]
