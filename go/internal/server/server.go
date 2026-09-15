// Package server registers this implementation's three MCP tools.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/config"
	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/outcomes"
)

// Version is reported to the client in the initialize handshake.
const Version = "v0.1.0"

// Tool descriptions, copied verbatim from the other implementations. A model
// reads these to choose a tool, so they are part of the contract rather than
// documentation about it.
const (
	commentActivityDescription = "Find and group the comments on a Facebook Page's posts. Use this instead of listing " +
		"posts and then their comments separately: it sweeps the target, assembles reply " +
		"threads, marks which need a reply from the Page, and returns counts per group. " +
		"Comment text is returned as untrusted third-party content. " +
		"For comments on ADS, pass an ad or campaign id rather than a page id: ads usually run " +
		"on unpublished posts, and no Page-level sweep returns those — resolving the ad to the " +
		"post behind it is the only way to reach them. " +
		"When you do not know an id, walk down: omit every target to list Pages and ad accounts, " +
		"pass an adAccount to list its campaigns by name, then pass the campaign you want. The " +
		"first two rungs read no comments and are cheap. "

	replyDescription = "Publish a reply to a Facebook comment as the Page. This is a write: the reply is " +
		"public immediately and deleting it later does not unpublish what people saw. Use " +
		"dryRun first to confirm the target and the text."

	moderateDescription = "Hide or unhide a Facebook comment. Hiding removes it from public view without " +
		"deleting it, and is reversible in one call with the opposite action. This server " +
		"cannot delete comments."
)

// CommentActivityInput is the comment_activity tool's arguments.
//
// Every target is a pointer so the schema marks none of them required — the
// no-target case IS the orientation call.
type CommentActivityInput struct {
	Page       *string `json:"page,omitempty" jsonschema:"Page id to sweep. Omit every target to list what this identity can reach."`
	Post       *string `json:"post,omitempty" jsonschema:"Post id, in the {page-id}_{post-id} form Graph returns."`
	Comment    *string `json:"comment,omitempty" jsonschema:"Comment id, to read one thread in full."`
	Ad         *string `json:"ad,omitempty" jsonschema:"Ad id. Resolves to the Page post behind the ad, which is the only route to comments on unpublished posts."`
	Campaign   *string `json:"campaign,omitempty" jsonschema:"Campaign id. Resolves every ad in it to its Page post."`
	AdAccount  *string `json:"adAccount,omitempty" jsonschema:"Ad account id. Lists its campaigns by name; reads no comments."`
	Filter     *string `json:"filter,omitempty" jsonschema:"needs_reply (default), unanswered, hidden, or all."`
	GroupBy    *string `json:"groupBy,omitempty" jsonschema:"post (default), author, status, day, or none."`
	Since      *string `json:"since,omitempty" jsonschema:"ISO date. Filters the post sweep of a page target only; defaults to 30 days ago."`
	MaxThreads *int    `json:"maxThreads,omitempty" jsonschema:"Most threads to return. Default 100."`
}

// RespondInput is the respond_to_comment tool's arguments.
//
// CommentID, Message and PageID are NOT pointers, so the generated schema marks
// them required. That is one of the two guards on pageId; the tool body checks
// it as well, and either alone satisfies the rule.
type RespondInput struct {
	CommentID string `json:"commentId" jsonschema:"ID of the comment to reply to."`
	Message   string `json:"message" jsonschema:"Reply text. Published publicly as the Page."`
	PageID    string `json:"pageId" jsonschema:"Page that owns the comment. Required: a reply is published as the Page, which needs a Page token, and this is what the server exchanges to get one."`
	DryRun    bool   `json:"dryRun,omitempty" jsonschema:"Report what would be published without publishing it."`
}

// ModerateInput is the moderate_comment tool's arguments.
type ModerateInput struct {
	CommentID string `json:"commentId" jsonschema:"ID of the comment to hide or unhide."`
	Action    string `json:"action" jsonschema:"hide removes the comment from public view; unhide restores it."`
	PageID    string `json:"pageId" jsonschema:"Page that owns the comment. Required: hiding acts as the Page, which needs a Page token, and this is what the server exchanges to get one."`
	DryRun    bool   `json:"dryRun,omitempty" jsonschema:"Report the intended change without making it."`
}

// Build wires the Graph clients and registers the tools.
//
// enableWrites gates the two write tools entirely: a server built without it
// does not advertise them, so a model cannot call something it will be refused.
func Build(accessToken string, enableWrites bool) (*mcp.Server, error) {
	origin, err := config.ResolveGraphOrigin(os.Getenv)
	if err != nil {
		return nil, err
	}

	// Resolved ONCE, here, and handed to every transport. That is what stops the
	// URL builder and the paging.next pin from ever disagreeing.
	newTransport := func(token string) (*graph.Transport, error) {
		return graph.NewTransport(graph.TransportOptions{AccessToken: token, GraphOrigin: origin})
	}

	userTransport, err := newTransport(accessToken)
	if err != nil {
		return nil, err
	}
	pages := graph.NewPagesClient(userTransport)

	deps := outcomes.Deps{
		Pages:     pages,
		Marketing: graph.NewMarketingClient(userTransport),
		Tokens:    graph.NewTokenProvider(accessToken, pages),
		NewPagesClient: func(token string) (*graph.PagesClient, error) {
			bound, err := newTransport(token)
			if err != nil {
				return nil, err
			}
			return graph.NewPagesClient(bound), nil
		},
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "facebook-engagement-mcp",
		Version: Version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "comment_activity",
		Description: commentActivityDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CommentActivityInput) (*mcp.CallToolResult, any, error) {
		return jsonResult(runCommentActivity(ctx, deps, in))
	})

	if enableWrites {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "respond_to_comment",
			Description: replyDescription,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in RespondInput) (*mcp.CallToolResult, any, error) {
			return jsonResult(outcomes.RunRespondToComment(ctx, deps, outcomes.RespondOptions{
				CommentID: in.CommentID,
				Message:   in.Message,
				PageID:    in.PageID,
				DryRun:    in.DryRun,
			}))
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "moderate_comment",
			Description: moderateDescription,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in ModerateInput) (*mcp.CallToolResult, any, error) {
			return jsonResult(outcomes.RunModerateComment(ctx, deps, outcomes.ModerateOptions{
				CommentID: in.CommentID,
				Action:    outcomes.ModerationAction(in.Action),
				PageID:    in.PageID,
				DryRun:    in.DryRun,
			}))
		})
	}

	return server, nil
}

// runCommentActivity picks the rung the arguments asked for.
func runCommentActivity(ctx context.Context, deps outcomes.Deps, in CommentActivityInput) (any, error) {
	opts := outcomes.RunOptions{
		Page:      deref(in.Page),
		Post:      deref(in.Post),
		Comment:   deref(in.Comment),
		Ad:        deref(in.Ad),
		Campaign:  deref(in.Campaign),
		AdAccount: deref(in.AdAccount),
		Since:     deref(in.Since),
		Filter:    outcomes.Filter(deref(in.Filter)),
		GroupBy:   outcomes.GroupBy(deref(in.GroupBy)),
	}
	if in.MaxThreads != nil {
		opts.MaxThreads = *in.MaxThreads
	}

	target, err := outcomes.ResolveTarget(opts)
	if err != nil {
		return nil, err
	}

	// The two discovery rungs read no comments, so they run on the user token
	// directly and never pay for a Page exchange.
	switch target.Kind {
	case "pages":
		return outcomes.RunOrientation(ctx, deps)
	case "adAccount":
		return outcomes.RunCampaigns(ctx, deps, *target.ID)
	default:
		return outcomes.RunCommentActivity(ctx, deps, opts)
	}
}

// jsonResult encodes an outcome as the single TextContent the callers expect.
//
// The content is built here rather than left to the SDK's typed-output form.
//
// MEASURED, NOT ASSUMED, and the measurement went against the worry. The design
// treated "does the SDK also fill Content[0] from the output value" as an
// unverified assumption about someone else's system. It was tested by returning
// (nil, value, nil) instead: go-sdk v1.8.0 DOES fill it, with the same JSON, and
// the test below passed either way. So the explicit Content is not load-bearing
// today.
//
// It is kept anyway for a reason that does not depend on that: the conformance
// runner reads result.content[0].text and parses it, the TypeScript and Python
// handlers both return a JSON string, and this makes the Go handler say the same
// thing in its own source rather than inheriting it from a dependency's default.
// One fewer thing that changes when the SDK does.
//
// The test is deliberately NOT written to tell the two apart. It asserts the
// behaviour a caller can observe — content[0] is text and it parses — which both
// mechanisms satisfy. Pinning which one produced it would reject a legitimate
// implementation that chose the other, the same shape as the conformance suite's
// own a-reply-without-a-page-id-is-refused case.
//
// An error becomes a JSON object with an `error` key rather than a protocol
// error, matching the other implementations: a refusal is an answer, and the
// caller needs to read it.
func jsonResult(value any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		value = map[string]any{"error": err.Error()}
	}

	encoded, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		return nil, nil, fmt.Errorf("could not encode the answer: %w", marshalErr)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
	}, nil, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
