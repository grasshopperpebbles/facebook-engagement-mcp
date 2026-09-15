package outcomes

import (
	"fmt"
	"testing"
	"time"

	"github.com/grasshopperpebbles/facebook-engagement-mcp/go/internal/graph"
)

func strp(s string) *string { return &s }

func comment(id, at, authorID string) graph.Comment {
	c := graph.Comment{ID: id}
	if at != "" {
		c.CreatedTime = strp(at)
	}
	if authorID != "" {
		c.Author = &graph.Author{ID: strp(authorID)}
	}
	return c
}

// T-36. The sweep concatenates every second-level reply with every third-level
// one, so on a two-branch thread the Page's older answer lands last behind a
// visitor's newer comment on the other branch.
func TestInTimeOrderFixesTheConcatenation(t *testing.T) {
	// Arrival order is [second-level..., third-level...]; chronological is not.
	arrived := []graph.Comment{
		comment("branchA_page", "2026-09-02T11:00:00+0000", "pg1"),
		comment("branchB_page", "2026-09-02T11:30:00+0000", "pg1"),
		comment("branchB_visitor", "2026-09-02T12:00:00+0000", "visitor"),
	}
	got := InTimeOrder(arrived)
	want := []string{"branchA_page", "branchB_page", "branchB_visitor"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d = %s, want %s", i, got[i].ID, id)
		}
	}
}

// The shape that actually bites: a third-level reply written BEFORE a
// second-level one on another branch.
func TestInTimeOrderReordersAcrossBranches(t *testing.T) {
	arrived := []graph.Comment{
		// second level, both branches
		comment("A_page", "2026-09-02T11:00:00+0000", "pg1"),
		comment("B_page", "2026-09-02T11:30:00+0000", "pg1"),
		// third level, written between them
		comment("A_visitor", "2026-09-02T11:15:00+0000", "visitor"),
	}
	got := InTimeOrder(arrived)
	if got[1].ID != "A_visitor" {
		t.Fatalf("order = %s, %s, %s; want A_page, A_visitor, B_page",
			got[0].ID, got[1].ID, got[2].ID)
	}
}

// Last is by the clock, not by array position. Reading replies[len-1] is the
// same question only while the array is already in time order.
func TestLastWordReadsTheClockNotThePosition(t *testing.T) {
	thread := Thread{
		Comment: comment("c1", "2026-09-02T10:00:00+0000", "visitor"),
		Replies: []graph.Comment{
			comment("later_visitor", "2026-09-02T12:00:00+0000", "visitor"),
			comment("earlier_page", "2026-09-02T11:00:00+0000", "pg1"),
		},
	}
	if got := LastWord(thread); got.ID != "later_visitor" {
		t.Fatalf("LastWord = %s, want later_visitor — it read the position, not the clock", got.ID)
	}
}

func TestLastWordIsTheCommentWhenNobodyReplied(t *testing.T) {
	thread := Thread{Comment: comment("c1", "2026-09-02T10:00:00+0000", "visitor")}
	if got := LastWord(thread); got.ID != "c1" {
		t.Fatalf("LastWord = %s, want c1", got.ID)
	}
}

// A malformed timestamp costs one comment its place, never the whole answer.
func TestUnparseableTimesKeepTheirIncomingPosition(t *testing.T) {
	arrived := []graph.Comment{
		comment("first", "not-a-timestamp", ""),
		comment("second", "2026-09-02T10:00:00+0000", ""),
		comment("third", "", ""),
	}
	got := InTimeOrder(arrived)
	if len(got) != 3 {
		t.Fatalf("lost a comment: got %d, want 3", len(got))
	}
	if got[0].ID != "first" || got[1].ID != "second" || got[2].ID != "third" {
		t.Fatalf("unparseable comments were reordered: %s, %s, %s", got[0].ID, got[1].ID, got[2].ID)
	}
}

// Equal timestamps keep their incoming order rather than swapping run to run.
//
// THIS TEST TOOK TWO TRIES TO BECOME ABLE TO DISAGREE, and both failures are
// worth recording because neither is obvious.
//
// The first version used three tied comments. It passed with sort.Slice as
// happily as with sort.SliceStable, because Go's sort.Slice drops to insertion
// sort below about a dozen elements and insertion sort happens to be stable.
//
// Growing it to 64 tied comments did NOT fix it, which is the interesting half.
// When every element ties, the comparator returns false for every pair, so the
// slice is already sorted by any definition — pdqsort detects that and returns
// without moving anything. Size was never the variable.
//
// What it needs is ties AND real reordering work. Groups of tied comments in
// DESCENDING time order force actual partitioning, and stability then shows up
// as the order within each group surviving the move. Mutating SliceStable to
// Slice now fails at position 0.
func TestOrderingIsStableForEqualTimestamps(t *testing.T) {
	const groups, perGroup = 16, 4

	arrived := make([]graph.Comment, 0, groups*perGroup)
	for g := range groups {
		// Group 0 is the latest, so sorting ascending must reverse the groups.
		at := time.Date(2026, 9, 2, 10, 0, groups-g, 0, time.UTC).Format(graphTimeLayout)
		for m := range perGroup {
			arrived = append(arrived, comment(fmt.Sprintf("g%02d_m%d", g, m), at, ""))
		}
	}

	for range 20 {
		got := InTimeOrder(arrived)
		if len(got) != groups*perGroup {
			t.Fatalf("lost comments: %d", len(got))
		}
		for i, c := range got {
			// After sorting, group (groups-1-i/perGroup) comes first, and within
			// each group the members must still read m0, m1, m2, m3.
			wantID := fmt.Sprintf("g%02d_m%d", groups-1-i/perGroup, i%perGroup)
			if c.ID != wantID {
				t.Fatalf("position %d = %s, want %s — ties were reordered", i, c.ID, wantID)
			}
		}
	}
}

// The same size lesson applied to the degraded case: a comment Graph timed
// incompletely keeps its incoming position, and that has to hold at a size where
// the sort would otherwise move it.
func TestUnparseableTimesKeepTheirPositionAtScale(t *testing.T) {
	const size = 64
	arrived := make([]graph.Comment, 0, size)
	want := make([]string, 0, size)
	for i := range size {
		id := string(rune('a'+i%26)) + string(rune('0'+i/26))
		// Every third comment is untimed; the rest share one timestamp.
		at := "2026-09-02T10:00:00+0000"
		if i%3 == 0 {
			at = "not-a-timestamp"
		}
		arrived = append(arrived, comment(id, at, ""))
		want = append(want, id)
	}

	got := InTimeOrder(arrived)
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d = %s, want %s — an untimed comment was moved", i, got[i].ID, id)
		}
	}
}

// Graph's format is 2026-09-02T10:00:00+0000 — no colon in the offset, which is
// not RFC3339 and needs its own layout.
func TestCommentTimeParsesGraphsFormat(t *testing.T) {
	at, ok := CommentTime(comment("c1", "2026-09-02T10:00:00+0000", ""))
	if !ok {
		t.Fatal("CommentTime could not parse Graph's own timestamp format")
	}
	if at.Year() != 2026 || at.Month() != 9 || at.Day() != 2 {
		t.Fatalf("parsed = %v", at)
	}

	if _, ok := CommentTime(comment("c2", "", "")); ok {
		t.Error("a missing timestamp reported as parsed")
	}
	if _, ok := CommentTime(comment("c3", "not-a-timestamp", "")); ok {
		t.Error("a malformed timestamp reported as parsed")
	}
}

// InTimeOrder must not reorder its input in place — the caller's slice is often
// the one Graph returned and is read again.
func TestInTimeOrderDoesNotMutateItsInput(t *testing.T) {
	arrived := []graph.Comment{
		comment("later", "2026-09-02T12:00:00+0000", ""),
		comment("earlier", "2026-09-02T11:00:00+0000", ""),
	}
	_ = InTimeOrder(arrived)
	if arrived[0].ID != "later" {
		t.Fatal("InTimeOrder sorted the caller's slice in place")
	}
}

// ThreadPost.IsPublished is present when the post came from a feed sweep.
func TestThreadPostFromASweptPostCarriesPublishedState(t *testing.T) {
	got := NewThreadPost(graph.PagePost{ID: "p1", IsPublished: false})
	if got.IsPublished == nil || *got.IsPublished != false {
		t.Fatalf("IsPublished = %v, want a present false", got.IsPublished)
	}

	published := NewThreadPost(graph.PagePost{ID: "p2", IsPublished: true})
	if published.IsPublished == nil || *published.IsPublished != true {
		t.Fatalf("IsPublished = %v, want a present true", published.IsPublished)
	}
}

// And absent when the post node was never read — a post reached by id has no
// published state to report, and an asserted default would mislabel it.
func TestThreadPostForAPostReachedByIDSaysNothing(t *testing.T) {
	got := ThreadPostByID("p1")
	if got.IsPublished != nil {
		t.Fatalf("IsPublished = %v, want nil — the post node was never read", *got.IsPublished)
	}
	if got.ID != "p1" {
		t.Fatalf("ID = %s", got.ID)
	}
}

func TestAssembleThreadsPairsCommentsWithTheirReplies(t *testing.T) {
	post := ThreadPostByID("p1")
	comments := []graph.Comment{comment("c1", "2026-09-02T10:00:00+0000", "v"), comment("c2", "2026-09-02T10:05:00+0000", "v")}
	replies := map[string][]graph.Comment{
		"c1": {comment("c1_r", "2026-09-02T11:00:00+0000", "pg1")},
	}

	got := AssembleThreads(post, comments, replies)
	if len(got) != 2 {
		t.Fatalf("got %d threads", len(got))
	}
	if len(got[0].Replies) != 1 || got[0].Replies[0].ID != "c1_r" {
		t.Fatalf("c1 replies = %+v", got[0].Replies)
	}
	// A comment with no replies gets an empty slice, not a nil one — it is
	// rendered into JSON and an absent array reads differently from an empty one.
	if got[1].Replies == nil {
		t.Fatal("c2 replies = nil, want an empty slice")
	}
	if len(got[1].Replies) != 0 {
		t.Fatalf("c2 replies = %+v", got[1].Replies)
	}
}
