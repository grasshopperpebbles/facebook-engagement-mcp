// Command facebook-engagement-mcp is the MCP server's entry point.
//
// The conformance suite drives the INSTALLED binary through this, never the
// source tree. The `bin` entrypoint bug this project shipped did not exist in
// the source — it existed in the packaging, and twelve reviews of the source
// approved the line.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/server"
)

func main() {
	// NOTHING MAY WRITE TO STDOUT. MCP is JSON-RPC on stdin and stdout, and a
	// stray print corrupts the framing — the failure looks like a client that
	// cannot see the server rather than like a log line in the wrong place.
	log.SetOutput(os.Stderr)

	accessToken := os.Getenv("META_ACCESS_TOKEN")
	if accessToken == "" {
		fmt.Fprintln(os.Stderr,
			"META_ACCESS_TOKEN is required. See the README for the scopes it needs.")
		os.Exit(1)
	}

	enableWrites := os.Getenv("FACEBOOK_ENGAGEMENT_ENABLE_WRITES") == "true"
	if !enableWrites {
		fmt.Fprintln(os.Stderr,
			"Write tools are disabled. Set FACEBOOK_ENGAGEMENT_ENABLE_WRITES=true to enable "+
				"replying and hiding.")
	}

	built, err := server.Build(accessToken, enableWrites)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if err := built.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
