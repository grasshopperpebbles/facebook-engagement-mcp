package outcomes

import "sort"

// GroupBy selects how threads are bucketed in the response.
type GroupBy string

const (
	GroupByPost   GroupBy = "post"
	GroupByAuthor GroupBy = "author"
	GroupByStatus GroupBy = "status"
	GroupByDay    GroupBy = "day"
	GroupByNone   GroupBy = "none"
)

// GroupBys is every accepted value, for schema generation and validation.
var GroupBys = []GroupBy{GroupByPost, GroupByAuthor, GroupByStatus, GroupByDay, GroupByNone}

// Counts summarises a set of threads.
type Counts struct {
	Threads int `json:"threads"`
	// Comments is top-level comments plus their replies.
	Comments   int `json:"comments"`
	NeedsReply int `json:"needsReply"`
	Hidden     int `json:"hidden"`
}

// Group is one bucket of threads.
type Group struct {
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Counts  Counts          `json:"counts"`
	Threads []TriagedThread `json:"threads"`
}

// CountThreads counts a batch.
func CountThreads(threads []TriagedThread) Counts {
	counts := Counts{Threads: len(threads)}
	for _, thread := range threads {
		counts.Comments += 1 + len(thread.Replies)
		switch thread.Status {
		case StatusNeedsReply:
			counts.NeedsReply++
		case StatusHidden:
			counts.Hidden++
		}
	}
	return counts
}

// keyFor returns a group key and its human label.
//
// LABELS NEVER CARRY UNTRUSTED TEXT — not comment bodies and not author display
// names, which are free text chosen by whoever wrote the comment and would
// otherwise put attacker-authored strings into a structural field. Only Graph
// identifiers and this server's own words appear here; the display name itself
// is returned per comment, delimited, by the render layer.
func keyFor(thread TriagedThread, by GroupBy) (string, string) {
	switch by {
	case GroupByAuthor:
		if !hasAuthorID(thread.Comment) {
			return "unknown", "Author not returned by Graph"
		}
		return *thread.Comment.Author.ID, "Author " + *thread.Comment.Author.ID

	case GroupByStatus:
		return string(thread.Status), statusLabel(thread.Status)

	case GroupByDay:
		// The date part of Graph's own timestamp. Absent when Graph gave none —
		// no substituting today's date for a comment whose date is unknown.
		if thread.Comment.CreatedTime == nil || len(*thread.Comment.CreatedTime) < 10 {
			return "unknown", "Date not returned by Graph"
		}
		day := (*thread.Comment.CreatedTime)[:10]
		return day, day

	case GroupByNone:
		return "all", "All threads"

	default: // GroupByPost
		// isPublished is absent when the post node was never read; saying nothing
		// beats asserting either state. See ThreadPost.
		suffix := ""
		if thread.Post.IsPublished != nil && !*thread.Post.IsPublished {
			suffix = " (unpublished — ad-backed)"
		}
		return thread.Post.ID, "Post " + thread.Post.ID + suffix
	}
}

func statusLabel(status ThreadStatus) string {
	if status == StatusNeedsReply {
		return "needs reply"
	}
	return string(status)
}

// GroupThreads buckets threads and orders the buckets with the most waiting
// customers first — this response is opened to find work, not to browse.
func GroupThreads(threads []TriagedThread, by GroupBy) []Group {
	// Insertion order is kept alongside the map so the result is deterministic
	// for buckets that tie on both sort keys. Ranging a Go map is deliberately
	// randomised, and a response that reorders itself run to run is a response
	// nobody can diff.
	order := []string{}
	buckets := map[string]*Group{}

	for _, thread := range threads {
		key, label := keyFor(thread, by)
		bucket, seen := buckets[key]
		if !seen {
			bucket = &Group{Key: key, Label: label, Threads: []TriagedThread{}}
			buckets[key] = bucket
			order = append(order, key)
		}
		bucket.Threads = append(bucket.Threads, thread)
	}

	groups := make([]Group, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		bucket.Counts = CountThreads(bucket.Threads)
		groups = append(groups, *bucket)
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Counts.NeedsReply != groups[j].Counts.NeedsReply {
			return groups[i].Counts.NeedsReply > groups[j].Counts.NeedsReply
		}
		return groups[i].Counts.Threads > groups[j].Counts.Threads
	})
	return groups
}
