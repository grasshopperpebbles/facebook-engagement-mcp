package outcomes

import (
	"testing"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

func pageID(id string) *string { return &id }

// T-11. `answered` means the Page spoke LAST, not that the Page appears
// somewhere in the thread. The triage read .some(isPage), so visitor -> Page ->
// visitor came back answered while the customer was still waiting.
func TestPageMustHaveTheLastWord(t *testing.T) {
	answered := Thread{
		Comment: comment("answered1", "2026-09-02T10:00:00+0000", "visitor"),
		Replies: []graph.Comment{comment("answered1_r", "2026-09-02T11:00:00+0000", "pg1")},
	}
	waiting := Thread{
		Comment: comment("waiting1", "2026-09-02T10:00:00+0000", "visitor"),
		Replies: []graph.Comment{
			comment("waiting1_r", "2026-09-02T11:00:00+0000", "pg1"),
			comment("waiting1_r_r", "2026-09-02T12:00:00+0000", "visitor"),
		},
	}

	got, basis := TriageThreads([]Thread{answered, waiting}, pageID("pg1"))
	if basis != BasisAuthorIdentity {
		t.Fatalf("basis = %s, want author_identity", basis)
	}
	if got[0].Status != StatusAnswered {
		t.Errorf("answered1 = %s, want answered", got[0].Status)
	}
	if got[1].Status != StatusNeedsReply {
		t.Errorf("waiting1 = %s, want needs_reply — the Page spoke, then the customer did", got[1].Status)
	}
}

// T-36 reaching triage: the same two threads, with the replies handed over in
// the order the sweep's concatenation would produce rather than in time order.
// The verdict must not change.
func TestVerdictSurvivesRepliesArrivingOutOfOrder(t *testing.T) {
	waiting := Thread{
		Comment: comment("waiting1", "2026-09-02T10:00:00+0000", "visitor"),
		Replies: []graph.Comment{
			// The visitor's LATER comment listed first, the Page's earlier one
			// second — which is what flattening two branches produces.
			comment("waiting1_r_r", "2026-09-02T12:00:00+0000", "visitor"),
			comment("waiting1_r", "2026-09-02T11:00:00+0000", "pg1"),
		},
	}

	got, _ := TriageThreads([]Thread{waiting}, pageID("pg1"))
	if got[0].Status != StatusNeedsReply {
		t.Fatalf("status = %s, want needs_reply — triage read the array, not the clock", got[0].Status)
	}
}

// T-39/T-45. With no author anywhere, nothing can be shown to have ended with
// the Page, so nothing may be called answered. This is the CAUTIOUS answer.
func TestNoAuthorAnywhereMeansNeedsReply(t *testing.T) {
	thread := Thread{
		Comment: comment("c1", "2026-09-02T10:00:00+0000", ""),
		Replies: []graph.Comment{comment("r1", "2026-09-02T11:00:00+0000", "")},
	}
	got, basis := TriageThreads([]Thread{thread}, pageID("pg1"))
	if basis != BasisReplyCount {
		t.Fatalf("basis = %s, want reply_count", basis)
	}
	if got[0].Status != StatusNeedsReply {
		t.Fatalf("status = %s, want needs_reply — \"somebody replied\" cannot mean \"the Page replied\"", got[0].Status)
	}
}

// One authored comment anywhere in the batch is enough to ask the real question.
func TestOneAuthorInTheBatchLiftsTheWholeBatch(t *testing.T) {
	authored := Thread{Comment: comment("c1", "2026-09-02T10:00:00+0000", "visitor")}
	anonymous := Thread{Comment: comment("c2", "2026-09-02T10:00:00+0000", "")}

	_, basis := TriageThreads([]Thread{anonymous, authored}, pageID("pg1"))
	if basis != BasisAuthorIdentity {
		t.Fatalf("basis = %s, want author_identity", basis)
	}
}

// An author object with no id does not identify anyone, so it must not lift the
// basis. This is the nil-versus-zero trap arriving at the place it matters.
func TestAnAuthorWithoutAnIDDoesNotLiftTheBasis(t *testing.T) {
	c := graph.Comment{ID: "c1", CreatedTime: strp("2026-09-02T10:00:00+0000")}
	c.Author = &graph.Author{Name: strp("Someone")} // present, but no id

	_, basis := TriageThreads([]Thread{{Comment: c}}, pageID("pg1"))
	if basis != BasisReplyCount {
		t.Fatalf("basis = %s, want reply_count — an author with no id identifies nobody", basis)
	}
}

// Hidden wins over everything, and it is checked before the basis branches.
func TestHiddenIsItsOwnStatus(t *testing.T) {
	hidden := true
	c := comment("c1", "2026-09-02T10:00:00+0000", "visitor")
	c.Hidden = &hidden

	got, _ := TriageThreads([]Thread{{Comment: c}}, pageID("pg1"))
	if got[0].Status != StatusHidden {
		t.Fatalf("status = %s, want hidden", got[0].Status)
	}
}

// Absent is not hidden. A comment Graph said nothing about must not be filed
// away as hidden — it would vanish from the default view.
func TestAbsentHiddenIsNotHidden(t *testing.T) {
	got, _ := TriageThreads([]Thread{{
		Comment: comment("c1", "2026-09-02T10:00:00+0000", "visitor"),
	}}, pageID("pg1"))
	if got[0].Status == StatusHidden {
		t.Fatal("a comment with no is_hidden field was filed as hidden")
	}
}

// With no Page id at all, "somebody replied" is the only question the data
// supports — and it is answered as such rather than refused.
func TestWithoutAPageIDReplyCountIsTheOnlyQuestion(t *testing.T) {
	withReply := Thread{
		Comment: comment("c1", "2026-09-02T10:00:00+0000", "visitor"),
		Replies: []graph.Comment{comment("r1", "2026-09-02T11:00:00+0000", "someone")},
	}
	without := Thread{Comment: comment("c2", "2026-09-02T10:00:00+0000", "visitor")}

	got, basis := TriageThreads([]Thread{withReply, without}, nil)
	if basis != BasisReplyCount {
		t.Fatalf("basis = %s, want reply_count", basis)
	}
	if got[0].Status != StatusAnswered || got[1].Status != StatusNeedsReply {
		t.Fatalf("got %s, %s; want answered, needs_reply", got[0].Status, got[1].Status)
	}
}

func TestApplyFilter(t *testing.T) {
	threads := []TriagedThread{
		{Thread: Thread{Comment: comment("a", "", "")}, Status: StatusNeedsReply},
		{Thread: Thread{Comment: comment("b", "", "")}, Status: StatusAnswered},
		{Thread: Thread{Comment: comment("c", "", "")}, Status: StatusHidden},
	}
	for _, tc := range []struct {
		filter Filter
		want   int
	}{
		{FilterNeedsReply, 1},
		{FilterUnanswered, 1},
		{FilterHidden, 1},
		{FilterAll, 3},
	} {
		if got := ApplyFilter(threads, tc.filter); len(got) != tc.want {
			t.Errorf("filter %s returned %d, want %d", tc.filter, len(got), tc.want)
		}
	}
}

// The overturned cause must not come back. T-42 showed the token was the
// uncontrolled variable, not the author, and T-45 found the claim surviving in
// five places across two repos — two of them strings a marketer reads. A
// user-facing string nothing asserts is a string nothing corrects.
func TestTheOverturnedCauseIsNotRestated(t *testing.T) {
	forbidden := []string{
		"withholds it for a person",
		"withholds it for people",
		"returns author information for comments written by a Page",
		"Facebook returns author information",
	}
	note := ReplyCountNote()
	for _, phrase := range forbidden {
		if containsFold(note, phrase) {
			t.Errorf("the reply_count note restates the overturned cause: %q", phrase)
		}
	}
	// And it must say what it IS: a cautious answer, not an accurate one.
	if !containsFold(note, "cautious") {
		t.Errorf("the reply_count note does not say it is the cautious answer: %q", note)
	}
}

func containsFold(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexFold(haystack, needle) >= 0
}

func indexFold(haystack, needle string) int {
	lower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + 32
		}
		return b
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range len(needle) {
			if lower(haystack[i+j]) != lower(needle[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
