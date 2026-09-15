package outcomes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

// Limits. Pagination is internal; no cursor is exposed to the caller.
const (
	maxAdAccounts = 50
	// MaxCampaigns is listed for one account. Overflow is reported, never
	// silently cut.
	MaxCampaigns = 100

	defaultSinceDays = 30
	maxPosts         = 50

	// maxPhotosForGapCheck reads photos to detect posts the feed withheld
	// (T-37). One extra request per Page sweep, and it buys the difference
	// between a short answer and a short answer that says it is short.
	maxPhotosForGapCheck = 100

	// maxGapProbes bounds candidate posts actually probed before being called
	// unreadable. Each costs a request, and the count is normally zero or one.
	// The cap is here so a Page whose photos name fifty unswept stories cannot
	// turn one sweep into fifty-one requests.
	maxGapProbes = 10

	// maxCommentsPerPost is comments read per post before triage.
	//
	// Deliberately NOT maxThreads: maxThreads is a returned-volume cap, not a
	// page size. Using it as one would mean maxThreads=5 fetched five comments
	// from each of fifty posts and then triaged that arbitrary slice, which
	// answers a different question from "the five newest threads that need a
	// reply".
	maxCommentsPerPost = 200

	maxRepliesPerComment       = 25
	maxRepliesForCommentTarget = 100

	// postTextBudget is the share of the response text budget post bodies may
	// take, so a wall of ad copy cannot starve the comments, which are the
	// actual answer.
	postTextBudget = MaxResponseTextChars / 4
)

// Target names what was asked for.
type Target struct {
	Kind string  `json:"kind"`
	ID   *string `json:"id,omitempty"`
}

// AdAccountSummary is an ad account this identity can reach. Orientation only —
// no spend, no budgets.
type AdAccountSummary struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

// CampaignSummary is a campaign, named so a person can choose one without
// knowing its id.
type CampaignSummary struct {
	ID     string  `json:"id"`
	Name   *string `json:"name,omitempty"`
	Status *string `json:"status,omitempty"`
}

// Identity says which token read this, because it changes what came back.
//
// A Business System User token was shown three posts on a Page where a
// personally-granted token for the same app was shown five, and returned no
// author for comments the other identified (2026-09-15). Both differences are
// silent, so an answer that cannot name its own identity cannot explain itself.
//
// Reported on every response rather than only when something looks wrong: the
// degradation is not always detectable, and a field that appears only on bad
// days teaches a caller to read its absence as good news.
type Identity struct {
	Type    *string `json:"type,omitempty"`
	AppName *string `json:"appName,omitempty"`
	// NeverExpires is true when Graph reported expires_at: 0.
	NeverExpires *bool `json:"neverExpires,omitempty"`
}

// RenderedPost is a ThreadPost with its free text moved behind the untrusted
// marker — a post's own message is still public, third-party-reachable content,
// so it gets the same treatment. Nothing in a response may carry a raw message.
type RenderedPost struct {
	ID          string         `json:"id"`
	IsPublished *bool          `json:"isPublished,omitempty"`
	Permalink   *string        `json:"permalink,omitempty"`
	Text        *UntrustedText `json:"text,omitempty"`
}

// RenderedThread is one conversation as the caller receives it.
type RenderedThread struct {
	Comment RenderedComment   `json:"comment"`
	Replies []RenderedComment `json:"replies"`
	// PostID looks the body up in the response's top-level posts.
	PostID string `json:"postId"`
	Status string `json:"status"`
	// Abbreviated is set when the response ran out of text budget before this
	// thread: structure is returned, free text is not, and notes says how many.
	Abbreviated bool `json:"abbreviated,omitempty"`
}

// RenderedGroup is a group whose threads have been rendered.
type RenderedGroup struct {
	Key     string           `json:"key"`
	Label   string           `json:"label"`
	Counts  Counts           `json:"counts"`
	Threads []RenderedThread `json:"threads"`
}

// CommentActivity is the answer to the headline question.
type CommentActivity struct {
	Target   Target    `json:"target"`
	Identity *Identity `json:"identity,omitempty"`
	Filter   Filter    `json:"filter"`
	GroupBy  GroupBy   `json:"groupBy"`
	Since    string    `json:"since"`
	// StatusBasis says which question needs_reply answered. See triage.go.
	StatusBasis StatusBasis `json:"statusBasis"`
	Totals      Counts      `json:"totals"`
	// Posts is every post referenced by a returned thread, once each. Post
	// bodies live here rather than on each thread: repeating a 1,000-character
	// ad body onto every one of its comment threads is the same string a hundred
	// times over.
	Posts  []RenderedPost  `json:"posts"`
	Groups []RenderedGroup `json:"groups"`
	// Partial is true when some part of the answer could not be retrieved or was
	// abbreviated.
	Partial bool     `json:"partial"`
	Notes   []string `json:"notes"`
}

// Orientation is the answer when no target was given.
type Orientation struct {
	Pages      []graph.Page       `json:"pages"`
	Identity   *Identity          `json:"identity,omitempty"`
	AdAccounts []AdAccountSummary `json:"adAccounts,omitempty"`
	Notes      []string           `json:"notes,omitempty"`
}

// Campaigns is the answer for an adAccount target.
type Campaigns struct {
	Campaigns []CampaignSummary `json:"campaigns"`
	Truncated bool              `json:"truncated"`
}

// RunOptions is everything the tool accepts.
type RunOptions struct {
	Page       string
	Post       string
	Comment    string
	Ad         string
	Campaign   string
	AdAccount  string
	Filter     Filter
	GroupBy    GroupBy
	Since      string
	MaxThreads int
	// Today is injectable so the default `since` window is testable.
	Today time.Time
}

// Deps is what the runners need to reach Graph.
type Deps struct {
	Pages     *graph.PagesClient
	Marketing *graph.MarketingClient
	Tokens    *graph.TokenProvider
	// NewPagesClient builds a Pages client bound to a different token, for the
	// per-Page reads. Injectable so a test can supply one.
	NewPagesClient func(accessToken string) (*graph.PagesClient, error)
}

// RunOrientation answers "what can this identity reach at all".
//
// Two cheap list calls, NO POSTS AND NO COMMENTS — this is the call a model
// makes to find its feet before spending anything, and the ad accounts belong in
// it because ads are the only route to comments on unpublished posts.
func RunOrientation(ctx context.Context, deps Deps) (Orientation, error) {
	pages, _, err := deps.Pages.List(ctx, graph.PageOptions{MaxItems: 100})
	if err != nil {
		return Orientation{}, fmt.Errorf("%s", ExplainGraphError(err, ErrorContext{Operation: "read"}))
	}

	result := Orientation{Pages: pages}

	// Most tokens carry no ads_read — it is needed only for ad and campaign
	// targets — and orientation is the call a model makes before it knows what it
	// needs. Failing the whole answer would hide the Pages the token CAN reach
	// behind a permission it may never use, so a refusal is reported alongside
	// the Pages rather than instead of them.
	if deps.Marketing != nil {
		accounts, _, adErr := deps.Marketing.AdAccounts(ctx, graph.PageOptions{MaxItems: maxAdAccounts})
		if adErr != nil {
			result.Notes = append(result.Notes,
				"Ad accounts unavailable: "+ExplainGraphError(adErr, ErrorContext{Operation: "ad read"}))
		} else {
			summaries := make([]AdAccountSummary, 0, len(accounts))
			for _, account := range accounts {
				summaries = append(summaries, AdAccountSummary{ID: account.ID, Name: account.Name})
			}
			result.AdAccounts = summaries
		}
	}

	// Orientation is the call a model makes before it knows what it needs, so it
	// is the right place to learn what it is holding.
	result.Identity = describeIdentity(ctx, deps)
	return result, nil
}

// RunCampaigns names an ad account's campaigns, so a person can pick one by name
// rather than pasting an id out of Ads Manager.
//
// Deliberately reads no comments: it is the rung between "what can I reach" and
// the sweep, and it is only worth walking speculatively while it stays cheap.
func RunCampaigns(ctx context.Context, deps Deps, adAccountID string) (Campaigns, error) {
	if deps.Marketing == nil {
		return Campaigns{}, errors.New("ad and campaign targets need Marketing API access, which is not configured")
	}

	campaigns, truncated, err := deps.Marketing.Campaigns(ctx, adAccountID, graph.PageOptions{MaxItems: MaxCampaigns})
	if err != nil {
		return Campaigns{}, fmt.Errorf("%s", ExplainGraphError(err, ErrorContext{Operation: "ad read"}))
	}

	summaries := make([]CampaignSummary, 0, len(campaigns))
	for _, campaign := range campaigns {
		// effective_status in preference to status: a campaign set ACTIVE inside
		// a paused ad set or a disabled account is not running, and only the
		// effective value says so.
		//
		// Every campaign is listed regardless of status: a finished campaign's
		// comments are still comments.
		status := campaign.EffectiveStatus
		if status == nil {
			status = campaign.Status
		}
		summaries = append(summaries, CampaignSummary{ID: campaign.ID, Name: campaign.Name, Status: status})
	}
	return Campaigns{Campaigns: summaries, Truncated: truncated}, nil
}

// describeIdentity reports the identity behind the token, or nil when Graph
// would not say.
//
// Never fails the answer: this is context on a result, not a precondition for
// producing one.
func describeIdentity(ctx context.Context, deps Deps) *Identity {
	id, err := deps.Pages.Identity(ctx)
	if err != nil {
		return nil
	}
	if !id.Valid && id.Type == nil {
		return nil
	}

	identity := &Identity{Type: id.Type, AppName: id.AppName}
	if id.ExpiresAt != nil {
		never := *id.ExpiresAt == 0
		identity.NeverExpires = &never
	}
	return identity
}

// ResolveTarget decides which single target was asked for.
func ResolveTarget(opts RunOptions) (Target, error) {
	given := []string{}
	add := func(name, value string) {
		if value != "" {
			given = append(given, name)
		}
	}
	add("page", opts.Page)
	add("post", opts.Post)
	add("comment", opts.Comment)
	add("ad", opts.Ad)
	add("campaign", opts.Campaign)
	add("adAccount", opts.AdAccount)

	if len(given) > 1 {
		return Target{}, fmt.Errorf("give one target only; received %s", strings.Join(given, " and "))
	}

	switch {
	case opts.AdAccount != "":
		return Target{Kind: "adAccount", ID: &opts.AdAccount}, nil
	case opts.Page != "":
		return Target{Kind: "page", ID: &opts.Page}, nil
	case opts.Post != "":
		return Target{Kind: "post", ID: &opts.Post}, nil
	case opts.Comment != "":
		return Target{Kind: "comment", ID: &opts.Comment}, nil
	case opts.Ad != "":
		return Target{Kind: "ad", ID: &opts.Ad}, nil
	case opts.Campaign != "":
		return Target{Kind: "campaign", ID: &opts.Campaign}, nil
	default:
		return Target{Kind: "pages"}, nil
	}
}

// pageIDFromPostID recovers the Page from a {page-id}_{post-id} composite.
//
// Best effort: the format is not guaranteed, and callers may pass an id of
// another shape. Real Facebook Page ids are numeric, but this repository's own
// fixtures use readable stand-ins (pg1_p2), so this only checks for a non-empty
// prefix before the first underscore rather than requiring digits — a stricter
// check would silently defeat itself against this repo's own test data.
func pageIDFromPostID(postID string) string {
	prefix, rest, found := strings.Cut(postID, "_")
	if !found || prefix == "" || rest == "" {
		return ""
	}
	return prefix
}

// pageIDForTarget names the Page whose access token should read this target.
//
// ad and campaign targets are resolved to their posts BEFORE this runs, because
// a resolved effective_object_story_id is exactly the {page-id}_{post-id}
// composite the derivation understands. Deriving it afterwards, as an earlier
// version did, left ad and campaign reads on the user token — which Graph
// answers with empty data, not an error.
func pageIDForTarget(target Target, posts []ThreadPost) string {
	switch target.Kind {
	case "page":
		return *target.ID
	case "post":
		return pageIDFromPostID(*target.ID)
	case "ad", "campaign":
		if len(posts) == 0 {
			return ""
		}
		return pageIDFromPostID(posts[0].ID)
	default:
		return ""
	}
}

// newestFirst orders threads newest first; threads with no usable timestamp last.
func newestFirst(threads []TriagedThread) {
	sort.SliceStable(threads, func(i, j int) bool {
		left, leftOK := CommentTime(threads[i].Comment)
		right, rightOK := CommentTime(threads[j].Comment)
		if !leftOK {
			return false
		}
		if !rightOK {
			return true
		}
		return left.After(right)
	})
}

func isoDaysAgo(days int, today time.Time) string {
	return today.AddDate(0, 0, -days).UTC().Format("2006-01-02")
}
