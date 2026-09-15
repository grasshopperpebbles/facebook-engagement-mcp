package outcomes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

// RunCommentActivity reads comment activity for a Page, post, comment, ad or
// campaign.
//
// A Page target reads /feed, so unpublished ad-backed posts are included — that
// is the point of the server. Failures on individual posts degrade the answer to
// partial rather than failing it.
func RunCommentActivity(ctx context.Context, deps Deps, opts RunOptions) (CommentActivity, error) {
	target, err := ResolveTarget(opts)
	if err != nil {
		return CommentActivity{}, err
	}

	filter := opts.Filter
	if filter == "" {
		filter = FilterNeedsReply
	}
	groupBy := opts.GroupBy
	if groupBy == "" {
		groupBy = GroupByPost
	}
	maxThreads := opts.MaxThreads
	if maxThreads <= 0 {
		maxThreads = 100
	}
	today := opts.Today
	if today.IsZero() {
		today = time.Now()
	}
	since := opts.Since
	if since == "" {
		since = isoDaysAgo(defaultSinceDays, today)
	}

	notes := []string{}
	partial := false
	var posts []ThreadPost
	var threads []Thread

	// Ad and campaign targets resolve to their Page posts FIRST, before a token
	// is chosen: the resolved post ids are the only thing that names the Page,
	// and comments read with a user token come back as empty data rather than an
	// error — a silent zero on the exact capability this tool headlines. This
	// runs on the Marketing client, which is user-token scoped by design.
	if target.Kind == "ad" || target.Kind == "campaign" {
		if deps.Marketing == nil {
			return CommentActivity{}, errors.New(
				"ad and campaign targets need Marketing API access, which is not configured")
		}
		adTarget := AdTarget{}
		if target.Kind == "ad" {
			adTarget.Ad = target.ID
		} else {
			adTarget.Campaign = target.ID
		}

		postIDs, resolveNotes, resolveErr := ResolveAdPosts(ctx, deps.Marketing, adTarget)
		if resolveErr != nil {
			return CommentActivity{}, fmt.Errorf("%s", ExplainGraphError(resolveErr, ErrorContext{Operation: "read"}))
		}
		notes = append(notes, resolveNotes...)
		if len(resolveNotes) > 0 {
			partial = true
		}
		for _, id := range postIDs {
			posts = append(posts, ThreadPostByID(id))
		}
	}

	// Comment reads need a Page token: a user token returns empty data, which
	// reads as "no comments" rather than "wrong token". A page target names its
	// Page explicitly; every other target's Page is recovered best-effort from a
	// composite id. Any derivation failure falls back to the user token silently
	// rather than erroring, and usedUserToken tells the caller about it below.
	pageID := ""
	usedUserToken := false
	var accessToken string

	if target.Kind == "page" {
		pageID = *target.ID
		accessToken, err = deps.Tokens.TokenFor(ctx, pageID)
		if err != nil {
			var accessErr *graph.PageAccessError
			if errors.As(err, &accessErr) {
				return CommentActivity{}, errors.New(accessErr.Error())
			}
			return CommentActivity{}, fmt.Errorf("%s",
				ExplainGraphError(err, ErrorContext{Operation: "read", PageID: pageID}))
		}
	} else {
		derived := pageIDForTarget(target, posts)
		derivedToken := ""
		if derived != "" {
			if token, tokenErr := deps.Tokens.TokenFor(ctx, derived); tokenErr == nil {
				derivedToken = token
			}
		}
		if derivedToken != "" {
			accessToken = derivedToken
			pageID = derived
		} else {
			accessToken = deps.Tokens.TokenForUser()
			usedUserToken = true
		}
	}

	client, err := deps.NewPagesClient(accessToken)
	if err != nil {
		return CommentActivity{}, err
	}

	// A campaign can run ads on several Pages, and one request carries one Page
	// token. This is a real limitation, not a degradation to paper over.
	if pageID != "" && (target.Kind == "ad" || target.Kind == "campaign") {
		spanned := map[string]bool{}
		for _, post := range posts {
			if owner := pageIDFromPostID(post.ID); owner != "" && owner != pageID {
				spanned[owner] = true
			}
		}
		if len(spanned) > 0 {
			partial = true
			notes = append(notes, fmt.Sprintf(
				"This %s runs on %d Pages, but comments can be read with only one Page access "+
					"token per call. Only posts belonging to Page %s were read with a Page token; "+
					"posts on the other Pages return no comments rather than an error, so an empty "+
					"result for them is not evidence there are none. Query those Pages separately.",
				target.Kind, len(spanned)+1, pageID))
		}
	}

	switch target.Kind {
	case "page":
		feed, truncated, feedErr := client.Posts(ctx, *target.ID, graph.PostListOptions{
			PageOptions: graph.PageOptions{MaxItems: maxPosts},
			Since:       since,
		})
		if feedErr != nil {
			return CommentActivity{}, fmt.Errorf("%s",
				ExplainGraphError(feedErr, ErrorContext{Operation: "read", PageID: pageID}))
		}
		posts = nil
		for _, post := range feed {
			posts = append(posts, NewThreadPost(post))
		}
		if truncated {
			partial = true
			notes = append(notes, fmt.Sprintf(
				"More than %d posts exist since %s; older posts were not read.", maxPosts, since))
		}

	case "post":
		// No post node is fetched, so its published state is unknown and is
		// omitted rather than asserted. See ThreadPost.
		posts = []ThreadPost{ThreadPostByID(*target.ID)}
	}

	if target.Kind == "comment" {
		root, rootErr := client.Comment(ctx, *target.ID)
		if rootErr != nil {
			return CommentActivity{}, fmt.Errorf("%s",
				ExplainGraphError(rootErr, ErrorContext{Operation: "read", PageID: pageID}))
		}
		replies, _, repliesErr := client.Replies(ctx, *target.ID, graph.PageOptions{MaxItems: maxRepliesForCommentTarget})
		if repliesErr != nil {
			return CommentActivity{}, fmt.Errorf("%s",
				ExplainGraphError(repliesErr, ErrorContext{Operation: "read", PageID: pageID}))
		}
		threads = []Thread{{Comment: root, Replies: replies, Post: ThreadPostByID("unknown")}}
	} else {
		for _, post := range posts {
			postThreads, sweepErr := sweepPost(ctx, client, post)
			if sweepErr != nil {
				// One failing post must not fail the whole answer — it becomes a
				// labelled gap instead.
				partial = true
				notes = append(notes, fmt.Sprintf("Comments for post %s could not be read: %s",
					post.ID, ExplainGraphError(sweepErr, ErrorContext{Operation: "read", PageID: pageID})))
				continue
			}
			threads = append(threads, postThreads...)
		}
	}

	var pageIDPtr *string
	if pageID != "" {
		pageIDPtr = &pageID
	}

	triaged, basis := TriageThreads(threads, pageIDPtr)
	filtered := ApplyFilter(triaged, filter)

	// Sort BEFORE capping. Comments arrive oldest-first (order: chronological)
	// and post by post, so an unsorted slice keeps the oldest threads from the
	// first few posts while the note below claims the newest — dropping exactly
	// the comments a triage tool exists to surface.
	capped := make([]TriagedThread, len(filtered))
	copy(capped, filtered)
	newestFirst(capped)
	if len(capped) > maxThreads {
		capped = capped[:maxThreads]
	}
	if len(capped) < len(filtered) {
		partial = true
		notes = append(notes, fmt.Sprintf(
			"%d threads matched; the %d most recent are shown.", len(filtered), len(capped)))
	}

	notes = append(notes, basisNotes(basis, pageIDPtr, capped)...)

	unreadablePosts := 0
	if target.Kind == "page" {
		var gapNotes []string
		unreadablePosts, gapNotes = checkForWithheldPosts(ctx, client, *target.ID, posts, pageID)
		if unreadablePosts > 0 {
			partial = true
		}
		notes = append(notes, gapNotes...)
	}

	if usedUserToken {
		notes = append(notes,
			"Comments were read with the user access token because the Page could not be "+
				"determined from this target. Meta returns no comments for a user token, so an "+
				"empty result here may mean the wrong token rather than no comments. Pass a page "+
				"target to avoid this.")
	}

	// `since` is a Graph-side filter on the /feed sweep and nothing else. It is
	// echoed on every response, so every response that did not apply it has to
	// say so — otherwise the echoed value reads as a filter that ran.
	if target.Kind != "page" {
		notes = append(notes, fmt.Sprintf(
			"'since' (%s) filters the post sweep of a page target only. It was not applied to "+
				"this %s target, so comments older than that date can appear below.", since, target.Kind))
	}

	identity := describeIdentity(ctx, deps)
	withoutAuthors := countThreadsEndingWithoutAnAuthor(capped)

	// The identity goes on every response; the WARNING goes on only the ones
	// where it plausibly cost something. A caveat that fires every time is read
	// as boilerplate by the third response, and this one needs to be believed on
	// the day it matters.
	if identity != nil && identity.Type != nil && *identity.Type == "SYSTEM_USER" &&
		(unreadablePosts > 0 || withoutAuthors > 0) {
		notes = append(notes,
			"This read used a Business System User token. On 2026-09-15 such a token was shown "+
				"three posts on a Page where a personally-granted token for the same app was shown "+
				"five, and returned no author for comments the other identified — silently, in both "+
				"cases. Some of what is missing or unattributed above may be that rather than the "+
				"Page. A token granted by a person who administers the Page is the check; it "+
				"expires, which is why this one is recommended.")
	}

	renderedPosts, renderedThreads, renderNotes, renderPartial := renderWithinBudget(capped)
	notes = append(notes, renderNotes...)
	if renderPartial {
		partial = true
	}

	groups := GroupThreads(capped, groupBy)
	renderedGroups := make([]RenderedGroup, 0, len(groups))
	for _, group := range groups {
		out := RenderedGroup{Key: group.Key, Label: group.Label, Counts: group.Counts}
		out.Threads = make([]RenderedThread, 0, len(group.Threads))
		for _, thread := range group.Threads {
			out.Threads = append(out.Threads, renderedThreads[thread.Comment.ID])
		}
		renderedGroups = append(renderedGroups, out)
	}

	return CommentActivity{
		Target:      target,
		Identity:    identity,
		Filter:      filter,
		GroupBy:     groupBy,
		Since:       since,
		StatusBasis: basis,
		Totals:      CountThreads(capped),
		Posts:       renderedPosts,
		Groups:      renderedGroups,
		Partial:     partial,
		Notes:       notes,
	}, nil
}

// sweepPost reads one post's comments, THREE LEVELS DEEP.
//
// Both this server and the comment console were built on a doc comment asserting
// the opposite as settled fact — "Facebook flattens a reply-to-a-reply under the
// top-level comment". Nobody had watched it. Confirmed false against live Graph
// on 2026-09-11 (T-11): a third level is accepted and `parent` points at the
// reply, not at the top-level comment. Stopping after one level meant a
// visitor's answer to the Page's reply was never read — and since triage asks
// who spoke last, an unread last word made a waiting customer look handled.
func sweepPost(ctx context.Context, client *graph.PagesClient, post ThreadPost) ([]Thread, error) {
	comments, _, err := client.CommentsForPost(ctx, post.ID, graph.PageOptions{MaxItems: maxCommentsPerPost})
	if err != nil {
		return nil, err
	}

	repliesByCommentID := map[string][]graph.Comment{}
	for _, comment := range comments {
		// replyCount is what keeps this from costing a call per reply on the
		// ordinary case where nobody has replied.
		if comment.ReplyCount == nil || *comment.ReplyCount == 0 {
			continue
		}

		second, _, repliesErr := client.Replies(ctx, comment.ID, graph.PageOptions{MaxItems: maxRepliesPerComment})
		if repliesErr != nil {
			return nil, repliesErr
		}

		// A reply can carry replies of its own. Flattened into the same thread
		// deliberately: the unit a person acts on is the conversation, not the
		// nesting.
		deeper := []graph.Comment{}
		for _, reply := range second {
			if reply.ReplyCount == nil || *reply.ReplyCount == 0 {
				continue
			}
			nested, _, nestedErr := client.Replies(ctx, reply.ID, graph.PageOptions{MaxItems: maxRepliesPerComment})
			if nestedErr != nil {
				return nil, nestedErr
			}
			deeper = append(deeper, nested...)
		}

		// SORTED, because flattening destroys the order (T-36). Each Graph call
		// returns its own replies chronologically, but concatenating them puts
		// every second-level reply ahead of every third-level one regardless of
		// when any of it was written — so on a two-branch thread the Page's older
		// answer ends up last. Triage no longer reads the end of this array, and
		// it is sorted anyway: the array is what a person reads in the response,
		// and a conversation rendered out of order is wrong on its own terms.
		repliesByCommentID[comment.ID] = InTimeOrder(append(second, deeper...))
	}

	return AssembleThreads(post, comments, repliesByCommentID), nil
}

// countThreadsEndingWithoutAnAuthor counts on the LAST WORD, because that is
// what triage actually read.
//
// This used to require the comment AND every reply to lack an author, which made
// it silent on the commonest real shape: a thread carrying one authored comment
// and no note, and a thread whose LAST word has no author — the thread that is
// waiting — not counted either, because something earlier in it had an id.
func countThreadsEndingWithoutAnAuthor(threads []TriagedThread) int {
	count := 0
	for _, thread := range threads {
		if !hasAuthorID(LastWord(thread.Thread)) {
			count++
		}
	}
	return count
}

// basisNotes says out loud which question the statuses answered.
func basisNotes(basis StatusBasis, pageID *string, capped []TriagedThread) []string {
	notes := []string{}

	if basis == BasisReplyCount && pageID != nil {
		// T-39, corrected by T-42 (T-45). The note must not claim a cause: the
		// author-versus-Page explanation was recorded as fact from three
		// observations that shared one uncontrolled variable, the token, and was
		// overturned hours later. What survives is the behaviour, for a reason
		// that does not depend on the cause.
		notes = append(notes,
			"No comment in this batch carried an author, so the Page cannot be identified in any "+
				"thread and none can be shown as answered. Every thread is listed as needing a "+
				"reply, which may over-report: some may already be handled. Author information is "+
				"missing for reasons that depend on the credential rather than on who wrote the "+
				"comment — see the identity on this response.")
	} else if basis == BasisReplyCount {
		notes = append(notes,
			"Comment authors were not returned by Graph, so 'needs reply' means nobody replied at "+
				"all, not that the Page has not replied.")
	}

	// basis is computed once for the whole batch: one comment anywhere carrying
	// an author id is enough to call it author_identity. An individual thread
	// whose own comments carry no author id is still evaluated by the identity
	// path and, finding no Page-authored reply, comes out needs_reply — safe in
	// that it over-surfaces rather than hides a waiting customer, but the
	// batch-wide confidence claim does not hold for it. Say so.
	if withoutAuthors := countThreadsEndingWithoutAnAuthor(capped); basis == BasisAuthorIdentity && withoutAuthors > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d of these threads end with a comment carrying no author, so they are listed as "+
				"needing a reply because the last word could not be shown to be the Page's. Some "+
				"comments come back without author information and some do not, and which happens "+
				"depends on the credential rather than on who wrote them; a thread here may be "+
				"waiting or may already be handled.", withoutAuthors))
	}

	return notes
}
