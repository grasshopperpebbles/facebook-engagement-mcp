package outcomes

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

var permissionByOperation = map[string]string{
	"read":     "pages_read_engagement and pages_read_user_content",
	"ad read":  "ads_read",
	"reply":    "pages_manage_engagement",
	"moderate": "pages_manage_engagement",
}

// ErrorContext is what the explainer needs to give advice rather than an echo.
type ErrorContext struct {
	Operation string
	PageID    string
	// Tasks are the Page roles the token holds. nil means UNKNOWN, an empty
	// slice means known to be none — and the MODERATE guard below must act only
	// on the second.
	Tasks *[]string
}

// ExplainGraphError turns a Graph failure into something the caller can act on.
//
// A bare "(#200) Permissions error" is the most common failure this server's
// audience hits and the most expensive to debug: it does not say which
// permission, whether App Review is the blocker, or whether the Page role is
// simply missing.
func ExplainGraphError(err error, ctx ErrorContext) string {
	var metaErr *graph.MetaAPIError
	if !errors.As(err, &metaErr) {
		if err != nil {
			return err.Error()
		}
		return "Unexpected error talking to Meta."
	}

	trace := ""
	if metaErr.FbtraceID != nil && *metaErr.FbtraceID != "" {
		trace = fmt.Sprintf(" (Meta trace %s)", *metaErr.FbtraceID)
	}

	if metaErr.IsAuthError() {
		// Deliberately concrete. "Re-authenticate" reads as an OAuth flow, and
		// this server has none — a person pastes a token into the client's config
		// — so that wording sends a caller looking for a reconnect button that
		// does not exist. Observed happening on 2026-09-08.
		return "The Meta access token has expired or been revoked, so this call was refused " +
			"before it reached the Page. Mint a new user token and set it as META_ACCESS_TOKEN " +
			"wherever this server is configured — in Claude Desktop that is Settings, " +
			"Extensions, this extension, Configure. A token copied from the Graph API Explorer " +
			"lasts about an hour; exchange it for a long-lived one to get about 60 days." + trace
	}

	if metaErr.IsThrottled() {
		return "Meta rate limited this request. Back off before retrying." + trace
	}

	permissionRefused := (metaErr.Code != nil && *metaErr.Code == 200) ||
		(metaErr.Status != nil && *metaErr.Status == 403)

	if permissionRefused {
		// Established live on 2026-09-08, after a day lost to reading this
		// message literally. publish_actions was the permission for publishing AS
		// A USER. It was removed in 2018, so a write carrying a USER token reaches
		// a code path whose permission no longer exists and Graph names that
		// permission. The message is true and useless: the fault is the identity,
		// not the permission, and no amount of App Review can grant a dead scope.
		if strings.Contains(metaErr.Message, "publish_actions") {
			return "This write was sent with a user token instead of a Page token. Meta answers " +
				"it by naming `publish_actions`, which was the permission for publishing as a " +
				"user and was removed in 2018 — so the message describes a dead permission " +
				"rather than the real fault, and App Review cannot grant it. Supply the " +
				"`pageId` of the Page that owns the comment; the server exchanges it for a Page " +
				"token, and the same call then succeeds. Meta's own words: \"" +
				metaErr.Message + "\"" + trace
		}

		// The MODERATE guard acts on a KNOWN-EMPTY role list, never on silence.
		// Refusing on silence produces a message naming a role the identity may
		// well hold.
		if ctx.Operation != "read" && ctx.Tasks != nil && !slices.Contains(*ctx.Tasks, "MODERATE") {
			pageID := ctx.PageID
			if pageID == "" {
				pageID = "(unknown)"
			}
			return fmt.Sprintf(
				"This token has no MODERATE task on Page %s, so it cannot reply to or hide "+
					"comments. Grant that role in Page settings and re-issue the token.%s",
				pageID, trace)
		}

		permission, ok := permissionByOperation[ctx.Operation]
		if !ok {
			permission = "the relevant Page permission"
		}

		// Meta's own message is kept, always. It is often misleading — a refused
		// reply comes back naming publish_actions, deprecated since 2018 and not
		// the permission involved — but it is the only thing that distinguishes
		// one refusal from another, and a caller who cannot see it debugs the
		// wrong thing. Observed happening 2026-09-08.
		return fmt.Sprintf(
			"Meta refused this %s for lack of permission. It requires %s, which requires App "+
				"Review for Pages outside your own development-mode app. Meta's own words, which "+
				"may name a different or deprecated permission: \"%s\"%s",
			ctx.Operation, permission, metaErr.Message, trace)
	}

	return metaErr.Message + trace
}
