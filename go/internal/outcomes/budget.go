package outcomes

import (
	"context"
	"fmt"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

// renderWithinBudget renders posts and threads inside a shared character budget.
//
// Post bodies are hoisted and de-duplicated here; threads reference them by id.
// Repeating a 1,000-character ad body onto every one of its comment threads is
// the same string a hundred times over.
//
// Returns the rendered posts, the rendered threads keyed by their comment id,
// any notes, and whether anything was abbreviated.
func renderWithinBudget(capped []TriagedThread) ([]RenderedPost, map[string]RenderedThread, []string, bool) {
	budget := NewTextBudget(MaxResponseTextChars)
	notes := []string{}
	partial := false

	// Insertion order again, for the same reason grouping keeps one: ranging a
	// Go map is randomised and the posts array would reshuffle every run.
	postOrder := []string{}
	uniquePosts := map[string]ThreadPost{}
	for _, thread := range capped {
		if _, seen := uniquePosts[thread.Post.ID]; !seen {
			uniquePosts[thread.Post.ID] = thread.Post
			postOrder = append(postOrder, thread.Post.ID)
		}
	}

	postSpend := 0
	abbreviatedPosts := 0
	renderedPosts := make([]RenderedPost, 0, len(postOrder))
	for _, id := range postOrder {
		post := uniquePosts[id]
		cost := 0
		if post.Message != nil {
			cost = min(len([]rune(*post.Message)), MaxCommentChars)
		}

		if cost == 0 || (postSpend+cost <= postTextBudget && budget.Take(cost)) {
			postSpend += cost
			renderedPosts = append(renderedPosts, renderPost(post))
		} else {
			abbreviatedPosts++
			renderedPosts = append(renderedPosts, abbreviatePost(post))
		}
	}

	// Threads are already newest-first, so the budget buys the most recent ones.
	// Once one thread does not fit, every later thread is abbreviated too: a
	// response that cherry-picked short threads out of the middle would be harder
	// to reason about than a clean prefix.
	exhausted := false
	abbreviatedThreads := 0
	rendered := make(map[string]RenderedThread, len(capped))
	for _, thread := range capped {
		cost := CommentTextCost(thread.Comment)
		for _, reply := range thread.Replies {
			cost += CommentTextCost(reply)
		}

		if !exhausted && budget.Take(cost) {
			rendered[thread.Comment.ID] = renderThread(thread)
		} else {
			exhausted = true
			abbreviatedThreads++
			rendered[thread.Comment.ID] = abbreviateThread(thread)
		}
	}

	if abbreviatedThreads > 0 {
		partial = true
		notes = append(notes, fmt.Sprintf(
			"%d of the %d threads below are listed without their text: the response reached its "+
				"%d-character text budget. They are marked `abbreviated`. Lower maxThreads, narrow "+
				"the filter, or read one with a comment target to see them.",
			abbreviatedThreads, len(capped), MaxResponseTextChars))
	}
	if abbreviatedPosts > 0 {
		partial = true
		notes = append(notes, fmt.Sprintf(
			"%d post bodies were omitted to stay within the response text budget.", abbreviatedPosts))
	}

	return renderedPosts, rendered, notes, partial
}

func renderPost(post ThreadPost) RenderedPost {
	out := RenderedPost{ID: post.ID, IsPublished: post.IsPublished, Permalink: post.Permalink}
	if post.Message != nil {
		text := Untrusted(*post.Message, MaxCommentChars)
		out.Text = &text
	}
	return out
}

func abbreviatePost(post ThreadPost) RenderedPost {
	return RenderedPost{ID: post.ID, IsPublished: post.IsPublished, Permalink: post.Permalink}
}

func renderThread(thread TriagedThread) RenderedThread {
	replies := make([]RenderedComment, 0, len(thread.Replies))
	for _, reply := range thread.Replies {
		replies = append(replies, RenderComment(reply))
	}
	return RenderedThread{
		Comment: RenderComment(thread.Comment),
		Replies: replies,
		PostID:  thread.Post.ID,
		Status:  string(thread.Status),
	}
}

// abbreviateThread is structure without free text. Enough to count, group and
// act on the thread — or to come back for it with a comment target — without
// spending budget the response no longer has.
func abbreviateThread(thread TriagedThread) RenderedThread {
	return RenderedThread{
		Comment:     AbbreviateComment(thread.Comment),
		Replies:     []RenderedComment{},
		PostID:      thread.Post.ID,
		Status:      string(thread.Status),
		Abbreviated: true,
	}
}

// checkForWithheldPosts says when the Page holds posts these credentials cannot
// read (T-37).
//
// Some Page tokens are shown fewer posts than others on the same Page. Observed
// 2026-09-15: a Page token exchanged from a Business System User returned three
// posts where one exchanged from a personally-granted user token, FOR THE SAME
// APP, returned five — and read the missing two, and their comments, without
// complaint.
//
// The cause is not established, and this comment has already been wrong once
// about it. The first version said "a post is not returned to an app other than
// the one that published it", on a five-for-five correlation between the missing
// posts and another tool's publishing records. Both differences were present at
// once — different app AND different identity type — and only the identity
// survived being tested separately.
//
// What is not in doubt is the consequence: there is no error and no gap in the
// response, the list is simply shorter, and that is the worst way for a triage
// tool to be wrong.
//
// It is detectable because the withheld post's PHOTOS stay reachable, and every
// photo names its post in page_story_id. So any story id the photos edge knows
// and the sweep never saw is a post this token cannot read. That is how the
// 11 September post was found at all.
//
// What this does NOT catch: a text post, which leaves no media behind. So the
// count is a floor, and the note says so rather than implying it is complete.
func checkForWithheldPosts(
	ctx context.Context,
	client *graph.PagesClient,
	pageTargetID string,
	posts []ThreadPost,
	pageID string,
) (int, []string) {
	photos, _, err := client.Photos(ctx, pageTargetID, graph.PageOptions{MaxItems: maxPhotosForGapCheck})
	if err != nil {
		// Never fail the answer over the check on the answer. A token without
		// access to the photos edge still gets its comments; it just does not get
		// told what it is missing.
		return 0, []string{
			"Could not check whether this Page holds posts these credentials cannot read: " +
				ExplainGraphError(err, ErrorContext{Operation: "read", PageID: pageTargetID}),
		}
	}

	swept := map[string]bool{}
	for _, post := range posts {
		swept[post.ID] = true
	}

	candidates := []string{}
	seen := map[string]bool{}
	for _, photo := range photos {
		if photo.PostID == nil || swept[*photo.PostID] || seen[*photo.PostID] {
			continue
		}
		seen[*photo.PostID] = true
		candidates = append(candidates, *photo.PostID)
	}

	// Absent from the sweep is NOT the same as unreadable, and assuming it was
	// made this check wrong on the first Page it ran against: a Page's cover photo
	// names a page_story_id that /feed does not list the post under, so it looked
	// like a withheld post and read back with a plain 200. The count came out
	// right for the wrong reason, which is worse than coming out wrong.
	//
	// So each candidate is asked the question that actually matters — can this
	// token read the post's comments — and only a refusal counts. A readable post
	// with no comments answers with an empty list, which is a success and is not
	// counted.
	unreadable := 0
	for i, postID := range candidates {
		if i >= maxGapProbes {
			break
		}
		if _, _, probeErr := client.CommentsForPost(ctx, postID, graph.PageOptions{MaxItems: 1}); probeErr != nil {
			unreadable++
		}
	}

	if unreadable == 0 {
		return 0, nil
	}

	return unreadable, []string{fmt.Sprintf(
		"At least %d post(s) on this Page could not be read with these credentials and are "+
			"missing from this answer, along with any comments on them. Facebook shows some Page "+
			"access tokens fewer posts than others on the same Page: a token exchanged from a "+
			"Business System User has been observed returning fewer than one exchanged from a "+
			"personally-granted user token for the same app. If posts are missing, try a token "+
			"granted by a person who administers the Page. This count is a minimum: these were "+
			"found through the photos those posts contain, so a text-only post cannot be detected "+
			"at all.", unreadable)}
}
