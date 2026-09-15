package outcomes

import "github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"

// ThreadStatus is what triage decided about one thread.
type ThreadStatus string

const (
	StatusNeedsReply ThreadStatus = "needs_reply"
	StatusAnswered   ThreadStatus = "answered"
	StatusHidden     ThreadStatus = "hidden"
)

// StatusBasis says which question the status actually answers.
//
// BasisAuthorIdentity — no reply was authored by the Page. The real question.
// BasisReplyCount — nobody replied at all. A weaker, different question, used
// when Graph did not return `from`. A degraded answer that looks identical to a
// confident one is the defect this label exists to prevent.
type StatusBasis string

const (
	BasisAuthorIdentity StatusBasis = "author_identity"
	BasisReplyCount     StatusBasis = "reply_count"
)

// Filter selects which threads a caller wants back.
type Filter string

const (
	FilterNeedsReply Filter = "needs_reply"
	FilterUnanswered Filter = "unanswered"
	FilterHidden     Filter = "hidden"
	FilterAll        Filter = "all"
)

// Filters is every accepted value, for schema generation and validation.
var Filters = []Filter{FilterNeedsReply, FilterUnanswered, FilterHidden, FilterAll}

// TriagedThread is a thread and its verdict.
type TriagedThread struct {
	Thread
	Status ThreadStatus
}

// ReplyCountNote is what the response says when the status could only be decided
// on reply counts.
//
// It is a function rather than a constant so a test can assert it, which is the
// point: T-45 found this claim wrong in five places across two repos, two of
// them strings a marketer reads, and it survived because MUTATING THE NOTE TO
// "MUTATION XXXXX" LEFT ALL 244 TESTS GREEN. A user-facing string nothing
// asserts is a string nothing corrects.
//
// What it must NOT say is the cause that was overturned: that Graph returns
// `from` for a Page and withholds it for a person. Every observation behind that
// claim shared one uncontrolled variable — the token — and hours later a
// personally-granted token for the same app returned `from` for the same
// person's same comments (T-42). The cause is not established and nothing here
// may assert one.
func ReplyCountNote() string {
	return "Graph did not return author information for these comments, so it is not possible " +
		"to tell who replied. Nothing here can be shown to have ended with the Page, so every " +
		"thread is reported as needing a reply. This is the CAUTIOUS answer rather than the " +
		"accurate one: some of these may already be handled."
}

// StatusBasisFor reports whether the batch can support the real question.
func StatusBasisFor(threads []Thread) StatusBasis {
	for _, thread := range threads {
		if hasAuthorID(thread.Comment) {
			return BasisAuthorIdentity
		}
		for _, reply := range thread.Replies {
			if hasAuthorID(reply) {
				return BasisAuthorIdentity
			}
		}
	}
	return BasisReplyCount
}

// hasAuthorID is the nil-versus-zero check this whole port turns on. An Author
// that is present with no id is something Graph said that identifies nobody, and
// it must not lift the basis.
func hasAuthorID(c graph.Comment) bool {
	return c.Author != nil && c.Author.ID != nil && *c.Author.ID != ""
}

// TriageThreads decides which threads still need an answer from the Page.
func TriageThreads(threads []Thread, pageID *string) ([]TriagedThread, StatusBasis) {
	// Identity is only usable when we know both the authors and which id is the
	// Page. Missing either means the weaker question, said out loud.
	basis := BasisReplyCount
	if pageID != nil {
		basis = StatusBasisFor(threads)
	}

	triaged := make([]TriagedThread, 0, len(threads))
	for _, thread := range threads {
		// Hidden first, and absent is not hidden: a comment Graph said nothing
		// about must not be filed away, because it would leave the default view.
		if thread.Comment.Hidden != nil && *thread.Comment.Hidden {
			triaged = append(triaged, TriagedThread{Thread: thread, Status: StatusHidden})
			continue
		}

		if basis == BasisAuthorIdentity && pageID != nil {
			// The Page must have the LAST word, not merely a word.
			//
			// This used to be .some(reply => author === pageId), which reads
			// visitor -> Page -> visitor as answered and hides a customer who is
			// waiting. That shape was a hypothetical until 2026-09-11 (T-11), when
			// threads were confirmed against live Graph to go at least three
			// levels deep with correct parent pointers.
			//
			// "Last" is by the clock. It used to be replies.at(-1), resting on the
			// replies arriving chronologically — true of any ONE Graph call and
			// untrue of the thread the sweep assembles from several. See LastWord
			// and T-36.
			spokeLast := LastWord(thread)
			status := StatusNeedsReply
			if hasAuthorID(spokeLast) && *spokeLast.Author.ID == *pageID {
				status = StatusAnswered
			}
			triaged = append(triaged, TriagedThread{Thread: thread, Status: status})
			continue
		}

		if pageID != nil {
			// reply_count WITH a known Page: nobody is answered (T-39).
			//
			// This branch used to read `len(replies) > 0 ? answered : needs_reply`
			// — "somebody replied, so it is handled" — and that inverts the
			// product's answer: nothing here can tell whether the somebody was the
			// Page.
			//
			// The reason first recorded for the fix was overturned within the day
			// and is deliberately not repeated. See ReplyCountNote.
			//
			// What this rests on does not need a cause: with no author anywhere,
			// no thread can be shown to have ended with the Page, so none can be
			// called answered. That is the over-surfacing direction — say plainly
			// that it is the cautious answer, because some of these may be handled.
			//
			// Marking them "unknown" instead stays rejected: ApplyFilter matches an
			// exact status and needs_reply is the default filter, so every waiting
			// customer would drop silently out of the default view.
			triaged = append(triaged, TriagedThread{Thread: thread, Status: StatusNeedsReply})
			continue
		}

		// No Page id at all: nothing here can identify anyone, and "somebody
		// replied" is the only question the data supports.
		status := StatusNeedsReply
		if len(thread.Replies) > 0 {
			status = StatusAnswered
		}
		triaged = append(triaged, TriagedThread{Thread: thread, Status: status})
	}

	return triaged, basis
}

// ApplyFilter narrows a triaged batch to what the caller asked for.
func ApplyFilter(threads []TriagedThread, filter Filter) []TriagedThread {
	switch filter {
	case FilterNeedsReply, FilterUnanswered:
		return matching(threads, StatusNeedsReply)
	case FilterHidden:
		return matching(threads, StatusHidden)
	default:
		return threads
	}
}

func matching(threads []TriagedThread, status ThreadStatus) []TriagedThread {
	out := make([]TriagedThread, 0, len(threads))
	for _, thread := range threads {
		if thread.Status == status {
			out = append(out, thread)
		}
	}
	return out
}
