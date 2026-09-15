// Package graph is the Meta Graph API client: transport, errors, and the Pages
// and Marketing surfaces this server reads.
package graph

import "time"

// GraphAPIVersion is pinned deliberately. v25.0 (2026-02-18, expires
// 2028-07-29). v26.0 shipped 2026-07-29 but is too new to trust for a client
// touching ad spend. Moving this is an ADR, not a local edit — the pin serves
// more than one server.
const GraphAPIVersion = "v25.0"

// DefaultPageSize is what Graph is asked for per page.
const DefaultPageSize = 100

// MaxPageSize is what Graph rejects above, on most edges.
const MaxPageSize = 500

// DefaultMaxPages is a hard stop so a broken cursor cannot loop forever.
const DefaultMaxPages = 50

// DefaultTimeout bounds a single request, not a whole sweep.
const DefaultTimeout = 30 * time.Second

// DefaultRetryAttempts counts the first attempt. Writes never retry.
const DefaultRetryAttempts = 3

// DefaultRetryBase is the first backoff; each subsequent attempt doubles it.
const DefaultRetryBase = 500 * time.Millisecond

// throttleErrorCodes are the Graph codes meaning "back off", not "try again
// now".
//
// https://developers.facebook.com/docs/graph-api/overview/rate-limiting
var throttleErrorCodes = map[int]bool{4: true, 17: true, 32: true, 613: true}

// Field selections encode which Graph fields are actually useful per resource.
// They are copied exactly from the TypeScript rather than retyped from memory:
// dropping one silently narrows what the product can see, and nothing else in
// the repository would notice.
var (
	PageFields = []string{"id", "name", "category", "tasks"}

	// PageCredentialFields is the ONLY selection that asks for access_token.
	// pages.List must never request credential material.
	PageCredentialFields = []string{"id", "access_token", "tasks"}

	PostFields = []string{"id", "message", "created_time", "permalink_url", "is_published"}

	// PhotoFields answers "which post does this photo belong to", and nothing
	// else. No media sources: this edge is read to COUNT posts the feed withheld
	// (T-37), never to render anything, and a media selection would pull several
	// kilobytes of CDN URLs per photo for a number.
	PhotoFields = []string{"id", "created_time", "page_story_id"}

	// COMMENT_FIELDS carries two fields whose availability is not guaranteed:
	// `from` is not returned for every author on every token, and `can_hide` is
	// not listed in the v26.0 Comment node reference. Both are optional in the
	// normalized shape, so absence degrades rather than breaks.
	CommentFields = []string{
		"id", "message", "created_time", "from", "like_count", "comment_count",
		"is_hidden", "can_comment", "can_hide", "permalink_url", "parent",
	}

	AccountFields = []string{
		"id", "name", "account_status", "currency", "timezone_name",
		"created_time", "amount_spent", "spend_cap", "business_id",
	}

	CampaignFields = []string{
		"id", "name", "status", "objective", "daily_budget", "lifetime_budget",
		"start_time", "stop_time", "created_time", "updated_time", "effective_status",
	}

	AdSetFields = []string{
		"id", "name", "status", "campaign_id", "daily_budget", "lifetime_budget",
		"optimization_goal", "created_time", "updated_time",
	}

	AdFields = []string{
		"id", "name", "adset_id", "status", "created_time", "updated_time", "effective_status",
	}

	// AdCreativeFields resolves an ad to the Page post behind it.
	// effective_object_story_id resolves to the real post even when the creative
	// was defined inline; object_story_id is the explicit reference.
	AdCreativeFields = []string{"id", "effective_object_story_id", "object_story_id"}
)
