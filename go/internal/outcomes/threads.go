// Package outcomes turns Graph reads into the answers this server exists to
// give: which conversations are still waiting on the Page, grouped so a person
// can act on them.
package outcomes

import (
	"sort"
	"time"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

// graphTimeLayout matches Graph's own format, 2026-09-02T10:00:00+0000.
//
// Note the offset carries no colon, so this is NOT RFC3339 and time.RFC3339
// will not parse it.
const graphTimeLayout = "2006-01-02T15:04:05-0700"

// ThreadPost is the post a thread hangs off, as it appears in a response.
type ThreadPost struct {
	ID string `json:"id"`
	// IsPublished is false for unpublished, ad-backed posts; true for published
	// ones.
	//
	// ABSENT WHEN IT IS NOT KNOWN. Only a /feed sweep reads the post node, so a
	// post reached by id — a post, ad or campaign target — has no published state
	// to report, and an asserted default would mislabel a boosted published post
	// as ad-backed and an ad post fetched by id as organic. Omission says "not
	// checked"; it never means "published".
	//
	// This is a pointer where graph.PagePost.IsPublished is a plain bool, and the
	// two directions are opposite on purpose.
	IsPublished *bool   `json:"isPublished,omitempty"`
	Message     *string `json:"message,omitempty"`
	Permalink   *string `json:"permalink,omitempty"`
}

// Thread is one top-level comment and everything said under it, flattened.
type Thread struct {
	Comment graph.Comment
	Replies []graph.Comment
	Post    ThreadPost
}

// NewThreadPost builds a ThreadPost from a post that came out of a feed sweep,
// so its published state is known and carried.
func NewThreadPost(post graph.PagePost) ThreadPost {
	published := post.IsPublished
	return ThreadPost{
		ID:          post.ID,
		IsPublished: &published,
		Message:     post.Message,
		Permalink:   post.Permalink,
	}
}

// ThreadPostByID builds a ThreadPost for a post reached by id, whose node was
// never read — so it says nothing about publication rather than guessing.
func ThreadPostByID(postID string) ThreadPost {
	return ThreadPost{ID: postID}
}

// CommentTime returns when a comment was written, and whether that could be
// determined.
//
// It never fails loudly. A malformed timestamp costs one comment its place in
// the ordering, never the whole answer.
func CommentTime(c graph.Comment) (time.Time, bool) {
	if c.CreatedTime == nil || *c.CreatedTime == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(graphTimeLayout, *c.CreatedTime)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// InTimeOrder returns comments in the order they were written.
//
// STABLE, and deliberately so. A comment Graph timed incompletely keeps its
// incoming position rather than sorting to one end: degrading to the order the
// caller supplied is defensible, inventing an order is not.
//
// This exists because a thread is assembled from more than one Graph call and
// the concatenation is not chronological. Each call returns its own replies
// chronologically, but putting every second-level reply ahead of every
// third-level one ignores when any of it was written — so on a two-branch thread
// the Page's older answer ends up last. See LastWord and T-36.
//
// It copies rather than sorting in place: the caller's slice is usually the one
// Graph returned, and is read again.
func InTimeOrder(comments []graph.Comment) []graph.Comment {
	ordered := make([]graph.Comment, len(comments))
	copy(ordered, comments)

	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftOK := CommentTime(ordered[i])
		right, rightOK := CommentTime(ordered[j])
		if !leftOK || !rightOK {
			// Neither moves. SliceStable then leaves both where they were.
			return false
		}
		return left.Before(right)
	})
	return ordered
}

// LastWord returns the most recent thing said in this thread — the comment
// itself when nobody has replied.
//
// BY THE CLOCK, NOT BY ARRAY POSITION, and that distinction is the whole of
// T-36. Triage asks who spoke last, and until 2026-09-14 it answered with
// replies.at(-1). That is the same question only while the array is in time
// order, and the sweep builds it by concatenating every second-level reply with
// every third-level one: give a thread two branches and the Page's older answer
// on the first lands last, behind a visitor's newer comment on the second. The
// thread then reports `answered` with a customer waiting.
//
// That is exactly the inversion T-11 fixed on 2026-09-11, in the same direction,
// inside the code T-11's fix was written into — because a fix inherits the
// invariants of the code it lands in, and this one was never written down. So
// the ordering is no longer assumed anywhere: the sweep sorts what it assembles,
// and this answers from the timestamps regardless, because a caller that builds
// a Thread by hand must not be able to invert the product's only answer by
// listing replies in the wrong order.
func LastWord(thread Thread) graph.Comment {
	ordered := InTimeOrder(thread.Replies)
	if len(ordered) == 0 {
		return thread.Comment
	}
	return ordered[len(ordered)-1]
}

// AssembleThreads pairs each top-level comment with its flattened replies.
func AssembleThreads(
	post ThreadPost,
	comments []graph.Comment,
	repliesByCommentID map[string][]graph.Comment,
) []Thread {
	threads := make([]Thread, 0, len(comments))
	for _, c := range comments {
		replies := repliesByCommentID[c.ID]
		if replies == nil {
			// An empty slice rather than nil: this is rendered into JSON, and an
			// absent array reads differently from an empty one.
			replies = []graph.Comment{}
		}
		threads = append(threads, Thread{Comment: c, Replies: replies, Post: post})
	}
	return threads
}
