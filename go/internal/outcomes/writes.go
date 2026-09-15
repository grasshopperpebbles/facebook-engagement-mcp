package outcomes

import (
	"context"
	"errors"
	"strings"
)

// ModerationAction is what moderate_comment does to a comment.
type ModerationAction string

const (
	ActionHide   ModerationAction = "hide"
	ActionUnhide ModerationAction = "unhide"
)

// Actions is every accepted value, for schema generation and validation.
var Actions = []ModerationAction{ActionHide, ActionUnhide}

// RespondOptions is the reply tool's input.
type RespondOptions struct {
	CommentID string
	Message   string
	PageID    string
	DryRun    bool
}

// ModerateOptions is the moderation tool's input.
type ModerateOptions struct {
	CommentID string
	Action    ModerationAction
	PageID    string
	DryRun    bool
}

// missingPageIDMessage is the refusal both write tools give.
//
// Without a pageId the call would fall through to the startup client, which
// holds the USER token — and Meta refuses a user-token comment write by naming
// `publish_actions`, dead since 2018. Refusing here costs one round trip and
// says the true thing; letting it through costs a day. Observed 2026-09-08.
func missingPageIDMessage() string {
	return "A reply is published as the Page, so it needs a Page token. Supply `pageId` for the " +
		"Page that owns this comment. Without it the call would go out as the user, which Meta " +
		"refuses with a message about `publish_actions` — a permission removed in 2018."
}

// RunRespondToComment publishes a reply to a comment, as the Page.
func RunRespondToComment(ctx context.Context, deps Deps, opts RespondOptions) (map[string]any, error) {
	message := strings.TrimSpace(opts.Message)
	if message == "" {
		return nil, errors.New("a reply needs a non-empty message")
	}

	// The pageId guard runs BEFORE the dry run, so a dry run can never report
	// that a call would work when it could not. The tool schema also marks
	// pageId required; either layer satisfies the rule, and both are kept.
	if opts.PageID == "" {
		return nil, errors.New(missingPageIDMessage())
	}

	if opts.DryRun {
		// Report the SHAPE of the effect, never the text itself — the caller wrote
		// it and echoing it back costs context for nothing.
		return map[string]any{
			"dryRun": true,
			"intended": map[string]any{
				"commentId":     opts.CommentID,
				"messageLength": len([]rune(message)),
				"visibility":    "public",
			},
		}, nil
	}

	tasks, _ := deps.Tokens.TasksFor(ctx, opts.PageID)
	token, err := deps.Tokens.TokenFor(ctx, opts.PageID)
	if err != nil {
		return nil, err
	}
	client, err := deps.NewPagesClient(token)
	if err != nil {
		return nil, err
	}

	// No retry. A retried reply is a double post — the transport's Post path
	// enforces that, and nothing here may route around it.
	replyID, err := client.Reply(ctx, opts.CommentID, message)
	if err != nil {
		return nil, errors.New(ExplainGraphError(err, ErrorContext{
			Operation: "reply", PageID: opts.PageID, Tasks: tasks,
		}))
	}

	return map[string]any{"ok": true, "replyId": replyID, "commentId": opts.CommentID}, nil
}

// RunModerateComment hides or unhides a comment.
func RunModerateComment(ctx context.Context, deps Deps, opts ModerateOptions) (map[string]any, error) {
	if opts.Action != ActionHide && opts.Action != ActionUnhide {
		return nil, errors.New("action must be hide or unhide")
	}

	// Same reason as the reply tool, and checked before the dry run for the same
	// reason: a dry run must not report that a call would work when it could not.
	if opts.PageID == "" {
		return nil, errors.New("Hiding a comment acts as the Page, so it needs a Page token. " +
			"Supply `pageId` for the Page that owns this comment. Without it the call would go " +
			"out as the user, which Meta refuses with a message about `publish_actions` — a " +
			"permission removed in 2018.")
	}

	if opts.DryRun {
		return map[string]any{
			"dryRun": true,
			"intended": map[string]any{
				"commentId": opts.CommentID,
				"action":    string(opts.Action),
			},
		}, nil
	}

	hidden := opts.Action == ActionHide

	tasks, _ := deps.Tokens.TasksFor(ctx, opts.PageID)
	token, err := deps.Tokens.TokenFor(ctx, opts.PageID)
	if err != nil {
		return nil, err
	}
	client, err := deps.NewPagesClient(token)
	if err != nil {
		return nil, err
	}

	if err := client.SetHidden(ctx, opts.CommentID, hidden); err != nil {
		return nil, errors.New(ExplainGraphError(err, ErrorContext{
			Operation: "moderate", PageID: opts.PageID, Tasks: tasks,
		}))
	}

	return map[string]any{
		"ok":        true,
		"commentId": opts.CommentID,
		"action":    string(opts.Action),
		"hidden":    hidden,
	}, nil
}
